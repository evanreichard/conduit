package server

import (
	"io"
	"net"
)

var _ io.ReadWriteCloser = (*reconstructedConn)(nil)

// reconstructedConn wraps a net.Conn and overrides Read to handle captured data.
type reconstructedConn struct {
	net.Conn
	reader io.Reader
}

// Read reads from the reconstructed reader (prepended data + original conn).
func (rc *reconstructedConn) Read(p []byte) (n int, err error) {
	return rc.reader.Read(p)
}

// newReconstructedConn creates a reconstructed connection that replays the provided
// readers in order before reading from the underlying connection.
func newReconstructedConn(conn net.Conn, readers ...io.Reader) net.Conn {
	allReaders := append(readers, conn)
	return &reconstructedConn{
		Conn:   conn,
		reader: io.MultiReader(allReaders...),
	}
}
