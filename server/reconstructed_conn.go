package server

import (
	"bytes"
	"io"
	"net"
)

var _ io.ReadWriteCloser = (*reconstructedConn)(nil)

// reconstructedConn wraps a net.Conn and overrides Read to handle captured data.
type reconstructedConn struct {
	net.Conn
	reader io.Reader
}

// Read reads from the reconstructed reader (captured data + original conn).
func (rc *reconstructedConn) Read(p []byte) (n int, err error) {
	return rc.reader.Read(p)
}

// newReconstructedConn creates a reconstructed connection that replays captured data
// before reading from the original connection.
func newReconstructedConn(conn net.Conn, capturedData *bytes.Buffer) net.Conn {
	allReader := io.MultiReader(capturedData, conn)
	return &reconstructedConn{
		Conn:   conn,
		reader: allReader,
	}
}
