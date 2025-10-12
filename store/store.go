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
	}

	if bodyData, err := getRequestBody(req); err == nil {
		rec.RequestBody = bodyData
	}

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

	if bodyData, err := getResponseBody(resp); err == nil {
		rec.ResponseBody = bodyData
	}

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

func getRequestBody(req *http.Request) ([]byte, error) {
	if req.ContentLength == 0 || req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}

	if !isTextContentType(req.Header.Get("Content-Type")) {
		return nil, nil
	}

	// Read Body
	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, err
	}

	// Restore Body
	req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func getResponseBody(resp *http.Response) ([]byte, error) {
	if resp.ContentLength == 0 || resp.Body == nil || resp.Body == http.NoBody {
		return nil, nil
	}

	if !isTextContentType(resp.Header.Get("Content-Type")) {
		return nil, nil
	}

	// Read Body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Restore Body
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	return bodyBytes, nil
}

func isTextContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}

	if strings.HasPrefix(mediaType, "text/") {
		return true
	}

	switch mediaType {
	case "application/json":
		return true
	case "application/xml":
		return true
	case "application/x-www-form-urlencoded":
		return true
	default:
		return false
	}
}
