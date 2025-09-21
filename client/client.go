package client

import (
	"fmt"
	"net/url"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/config"
	"reichard.io/conduit/tunnel"
)

func NewTunnel(cfg *config.ClientConfig) (*tunnel.Tunnel, error) {
	// Parse Server URL
	serverURL, err := url.Parse(cfg.ServerAddress)
	if err != nil {
		return nil, err
	}

	// Parse Scheme
	var wsScheme string
	switch serverURL.Scheme {
	case "https":
		wsScheme = "wss"
	case "http":
		wsScheme = "ws"
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", serverURL.Scheme)
	}

	// Create Tunnel Name
	if cfg.TunnelName == "" {
		cfg.TunnelName = generateTunnelName()
		log.Infof("tunnel name not provided; generated: %s", cfg.TunnelName)
	}

	// Connect Server WS
	wsURL := fmt.Sprintf("%s://%s/_conduit/tunnel?tunnelName=%s&apiKey=%s", wsScheme, serverURL.Host, cfg.TunnelName, cfg.APIKey)
	serverConn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to connect: %v", err)
	}

	return tunnel.NewClientTunnel(cfg.TunnelName, cfg.TunnelTarget, serverConn), nil
}
