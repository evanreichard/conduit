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
	// Get Target URL
	targetURL, err := url.Parse(target)
	if err != nil {
		return nil, err
	}

	// Get Connection Builder
	var forwarder Forwarder
	switch targetURL.Scheme {
	case "http", "https":
		forwarder, err = newHTTPForwarder(targetURL, tunnelStore)
		if err != nil {
			return nil, err
		}
	default:
		forwarder = newTCPForwarder(target, tunnelStore)
	}

	return forwarder, nil
}
