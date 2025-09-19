package server

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
)

var _ http.ResponseWriter = (*connResponseWriter)(nil)

type connResponseWriter struct {
	conn   net.Conn
	header http.Header
}

func (f *connResponseWriter) Header() http.Header {
	if f.header == nil {
		f.header = make(http.Header)
	}
	return f.header
}

func (f *connResponseWriter) Write(data []byte) (int, error) {
	return f.conn.Write(data)
}

func (f *connResponseWriter) WriteHeader(statusCode int) {
	// Write Status
	status := fmt.Sprintf("HTTP/1.1 %d %s\r\n", statusCode, http.StatusText(statusCode))
	_, _ = f.conn.Write([]byte(status))

	// Write Headers
	for key, values := range f.header {
		for _, value := range values {
			_, _ = fmt.Fprintf(f.conn, "%s: %s\r\n", key, value)
		}
	}

	// End Headers
	_, _ = f.conn.Write([]byte("\r\n"))
}

func (f *connResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	// Return Raw Connection & ReadWriter
	rw := bufio.NewReadWriter(bufio.NewReader(f.conn), bufio.NewWriter(f.conn))
	return f.conn, rw, nil
}
