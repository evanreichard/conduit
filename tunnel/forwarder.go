package tunnel

import (
	"context"
	"net/url"

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
	// Only parse as URL for HTTP targets. Bare host:port (e.g., "127.0.0.1:5432")
	// is not a valid URL and should be treated as a raw TCP target.
	targetURL, err := url.Parse(target)
	if err == nil {
		switch targetURL.Scheme {
		case "http", "https":
			return newHTTPForwarder(targetURL, tunnelStore)
		}
	}

	return newTCPForwarder(target, tunnelStore), nil
}
