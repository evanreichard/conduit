package tunnel

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"reichard.io/conduit/store"
)

type ForwarderType int

const (
	ForwarderTCP ForwarderType = iota
	ForwarderHTTP
)

type Forwarder interface {
	Type() ForwarderType
	Initialize(sourceAddress string) (Stream, error)
	Start(context.Context) error
}

func NewForwarder(target string, tunnelStore store.TunnelStore) (Forwarder, error) {
	if !strings.Contains(target, "://") {
		return nil, fmt.Errorf("target must include a scheme: tcp://, http://, or https://")
	}

	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, fmt.Errorf("target is invalid: %w", err)
	}
	if targetURL.Host == "" {
		return nil, fmt.Errorf("target must include a host")
	}

	switch targetURL.Scheme {
	case "http", "https":
		return newHTTPForwarder(targetURL, tunnelStore)
	case "tcp":
		return newTCPForwarder(targetURL.Host, tunnelStore), nil
	default:
		return nil, fmt.Errorf("unsupported target scheme %q: use tcp://, http://, or https://", targetURL.Scheme)
	}
}
