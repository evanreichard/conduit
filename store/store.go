package store

import (
	"bytes"
	"errors"
	"io"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	defaultQueueSize = 100
	maxQueueSize     = 100
	maxBodyCapture   = 1024 * 1024
)

var ErrRecordNotFound = errors.New("record not found")

type OnEntryHandler func(record *TunnelRecord)

type TunnelStore interface {
	Get(before time.Time, count int) (results []*TunnelRecord, more bool)
	Subscribe() <-chan *TunnelRecord
	RecordTCP()
	RecordRequest(req *http.Request, sourceAddress string)
	RecordResponse(resp *http.Response) error
}

func NewTunnelStore(queueSize int) TunnelStore {
	if queueSize <= 0 {
		queueSize = defaultQueueSize
	} else if queueSize > maxQueueSize {
		queueSize = maxQueueSize
	}

	return &tunnelStoreImpl{queueSize: queueSize}
}

type tunnelStoreImpl struct {
	orderedRecords []*TunnelRecord
	queueSize      int
	subs           []chan *TunnelRecord
	mu             sync.Mutex
}

func (s *tunnelStoreImpl) Subscribe() <-chan *TunnelRecord {
	s.mu.Lock()
	defer s.mu.Unlock()

	ch := make(chan *TunnelRecord, 100)

	// Flush Existing & Subscribe
	for _, r := range s.orderedRecords {
		ch <- r
	}
	s.subs = append(s.subs, ch)

	return ch
}

func (s *tunnelStoreImpl) Get(before time.Time, count int) ([]*TunnelRecord, bool) {
	// Find First
	start := -1
	for i, r := range s.orderedRecords {
		if r.Time.Before(before) {
			start = i
			break
		}
	}

	// Not Found
	if start == -1 {
		return nil, false
	}

	// Subslice Records
	end := min(start+count, len(s.orderedRecords))
	results := s.orderedRecords[start:end]
	more := end < len(s.orderedRecords)

	return results, more
}

func (s *tunnelStoreImpl) RecordRequest(req *http.Request, sourceAddress string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	url := *req.URL
	rec := &TunnelRecord{
		ID:              uuid.New(),
		Time:            time.Now(),
		URL:             &url,
		Method:          req.Method,
		SourceAddr:      sourceAddress,
		RequestHeaders:  req.Header,
		RequestBodyType: req.Header.Get("Content-Type"),
		RequestBodySize: req.ContentLength,
	}

	bodyData, meta := captureBody(&req.Body, req.Header.Get("Content-Type"), req.ContentLength, false)
	rec.RequestBody = bodyData
	rec.RequestBodySize = meta.size
	rec.RequestBodyCaptured = meta.captured
	rec.RequestBodyTruncated = meta.truncated
	rec.RequestBodySkipped = meta.skipped

	// Add Record & Truncate
	s.orderedRecords = append(s.orderedRecords, rec)
	if len(s.orderedRecords) > s.queueSize {
		s.orderedRecords = s.orderedRecords[len(s.orderedRecords)-s.queueSize:]
	}

	*req = *req.WithContext(withRecord(req.Context(), rec))
}

func (s *tunnelStoreImpl) RecordResponse(resp *http.Response) error {
	rec, found := getRecord(resp.Request.Context())
	if !found {
		return ErrRecordNotFound
	}

	rec.Status = resp.StatusCode
	rec.ResponseHeaders = resp.Header
	rec.ResponseBodyType = resp.Header.Get("Content-Type")
	rec.ResponseBodySize = resp.ContentLength

	bodyData, meta := captureBody(&resp.Body, resp.Header.Get("Content-Type"), resp.ContentLength, true)
	rec.ResponseBody = bodyData
	rec.ResponseBodySize = meta.size
	rec.ResponseBodyCaptured = meta.captured
	rec.ResponseBodyTruncated = meta.truncated
	rec.ResponseBodySkipped = meta.skipped

	s.broadcast(rec)

	return nil
}

func (s *tunnelStoreImpl) RecordTCP() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// TODO
}

func (s *tunnelStoreImpl) broadcast(record *TunnelRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Send to Subscribers
	active := s.subs[:0]
	for _, ch := range s.subs {
		select {
		case ch <- record:
			active = append(active, ch)
		default:
			close(ch)
		}
	}
	s.subs = active
}

type bodyCaptureMeta struct {
	size      int64
	captured  bool
	truncated bool
	skipped   string
}

func captureBody(body *io.ReadCloser, contentType string, contentLength int64, allowImages bool) ([]byte, bodyCaptureMeta) {
	meta := bodyCaptureMeta{size: contentLength}
	if contentLength == 0 || *body == nil || *body == http.NoBody {
		return nil, meta
	}

	previewable := isTextContentType(contentType) || (allowImages && isImageContentType(contentType))
	if !previewable {
		meta.skipped = "body content type is not previewable"
		return nil, meta
	}

	if isImageContentType(contentType) && contentLength > maxBodyCapture {
		meta.skipped = "image body is too large to preview"
		return nil, meta
	}

	// Capture Bounded Prefix
	originalBody := *body
	limit := int64(maxBodyCapture + 1)
	captured, err := io.ReadAll(io.LimitReader(originalBody, limit))
	if err != nil {
		meta.skipped = "failed to read body"
		*body = originalBody
		return nil, meta
	}

	if meta.size < 0 && len(captured) <= maxBodyCapture {
		meta.size = int64(len(captured))
	}

	// Restore Body
	*body = &replayReadCloser{
		Reader: io.MultiReader(bytes.NewReader(captured), originalBody),
		closer: originalBody,
	}

	if len(captured) > maxBodyCapture {
		meta.truncated = true
		captured = captured[:maxBodyCapture]
	}

	if isImageContentType(contentType) && meta.truncated {
		meta.skipped = "image body is too large to preview"
		return nil, meta
	}

	meta.captured = len(captured) > 0
	return captured, meta
}

type replayReadCloser struct {
	io.Reader
	closer io.Closer
}

func (r *replayReadCloser) Close() error {
	return r.closer.Close()
}

func isTextContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	if strings.HasPrefix(mediaType, "text/") {
		return true
	}

	if strings.HasSuffix(mediaType, "+json") || strings.HasSuffix(mediaType, "+xml") {
		return true
	}

	switch mediaType {
	case "application/json":
		return true
	case "application/xml":
		return true
	case "application/javascript":
		return true
	case "application/x-javascript":
		return true
	case "application/x-www-form-urlencoded":
		return true
	default:
		return false
	}
}

func isImageContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	switch mediaType {
	case "image/png":
		return true
	case "image/jpeg":
		return true
	case "image/gif":
		return true
	case "image/webp":
		return true
	case "image/svg+xml":
		return true
	default:
		return false
	}
}
