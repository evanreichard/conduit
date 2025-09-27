package tunnel

import (
	"context"
	"net"

	"reichard.io/conduit/store"
)

func newTCPForwarder(target string, tunnelStore store.TunnelStore) Forwarder {
	return &tcpConnBuilder{
		target:      target,
		tunnelStore: tunnelStore,
	}
}

type tcpConnBuilder struct {
	target      string
	tunnelStore store.TunnelStore
}

func (l *tcpConnBuilder) Type() ForwarderType {
	return ForwarderTCP
}

func (l *tcpConnBuilder) Initialize() (Stream, error) {
	conn, err := net.Dial("tcp", l.target)
	if err != nil {
		return nil, err
	}

	return &streamImpl{conn, l.target}, nil
}

func (l *tcpConnBuilder) Start(ctx context.Context) error {
	return nil
}
