package tunnel

import (
	"io"
	"net"
)

var _ Stream = (*streamImpl)(nil)

type Stream interface {
	io.ReadWriteCloser
	Source() string
}

func NewStream(conn net.Conn, source string) Stream {
	return &streamImpl{conn, source}
}

type streamImpl struct {
	net.Conn
	source string
}

func (s *streamImpl) Source() string {
	return s.source
}
