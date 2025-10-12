package tunnel

import (
	"io"
	"net"
)

var _ Stream = (*streamImpl)(nil)

type Stream interface {
	io.ReadWriteCloser
	Source() string
	Target() string
}

func NewStream(conn net.Conn, source, target string) Stream {
	return &streamImpl{conn, source, target}
}

type streamImpl struct {
	net.Conn
	source string
	target string
}

func (s *streamImpl) Source() string {
	return s.source
}

func (s *streamImpl) Target() string {
	return s.target
}
