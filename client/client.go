package client

import (
	"fmt"
	"net"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/config"
	"reichard.io/conduit/types"
)

func NewTunnel(cfg *config.ClientConfig) (*Tunnel, error) {
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

	return &Tunnel{
		name:       cfg.TunnelName,
		target:     cfg.TunnelTarget,
		serverURL:  serverURL,
		serverConn: serverConn,
		localConns: make(map[string]net.Conn),
	}, nil

}

type Tunnel struct {
	name      string
	target    string
	serverURL *url.URL

	serverConn *websocket.Conn
	localConns map[string]net.Conn
	mu         sync.RWMutex
}

func (t *Tunnel) Start() error {
	log.Infof("starting tunnel: %s.%s -> %s\n", t.name, t.serverURL.Hostname(), t.target)
	defer t.serverConn.Close()

	// Handle Messages
	for {
		// Read Message
		var msg types.Message
		err := t.serverConn.ReadJSON(&msg)
		if err != nil {
			log.Errorf("error reading from tunnel: %v", err)
			break
		}

		switch msg.Type {
		case types.MessageTypeData:
			localConn, err := t.getLocalConn(msg.StreamID)
			if err != nil {
				log.Errorf("failed to get local connection: %v", err)
				continue
			}

			// Write data to local connection
			if _, err := localConn.Write(msg.Data); err != nil {
				log.Errorf("error writing to local connection: %v", err)
				localConn.Close()
				t.mu.Lock()
				delete(t.localConns, msg.StreamID)
				t.mu.Unlock()
			}

		case types.MessageTypeClose:
			t.mu.Lock()
			if localConn, exists := t.localConns[msg.StreamID]; exists {
				localConn.Close()
				delete(t.localConns, msg.StreamID)
			}
			t.mu.Unlock()
		}
	}

	return nil
}

func (t *Tunnel) getLocalConn(streamID string) (net.Conn, error) {
	// Get Cached Connection
	t.mu.RLock()
	localConn, exists := t.localConns[streamID]
	t.mu.RUnlock()
	if exists {
		return localConn, nil
	}

	// Initiate Connection & Cache
	localConn, err := net.Dial("tcp", t.target)
	if err != nil {
		log.Errorf("failed to connect to %s: %v", t.target, err)
		return nil, err
	}
	t.mu.Lock()
	t.localConns[streamID] = localConn
	t.mu.Unlock()

	// Start Response Relay & Return Connection
	go t.startResponseRelay(streamID, localConn)
	return localConn, nil
}

func (t *Tunnel) startResponseRelay(streamID string, localConn net.Conn) {
	defer func() {
		t.mu.Lock()
		delete(t.localConns, streamID)
		t.mu.Unlock()
		localConn.Close()
	}()

	buffer := make([]byte, 4096)
	for {
		n, err := localConn.Read(buffer)
		if err != nil {
			break
		}

		response := types.Message{
			Type:     types.MessageTypeData,
			StreamID: streamID,
			Data:     buffer[:n],
		}

		if err := t.serverConn.WriteJSON(response); err != nil {
			break
		}
	}
}
