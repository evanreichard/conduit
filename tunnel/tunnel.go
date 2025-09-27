package tunnel

import (
	"context"
	"fmt"
	"net/url"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/config"
	"reichard.io/conduit/pkg/maps"
	"reichard.io/conduit/types"
)

// NewServerTunnel creates a new tunnel with name and websocket connection. The tunnel is
// generally instantiated after an upgrade request from the server.
func NewServerTunnel(name string, wsConn *websocket.Conn) *Tunnel {
	return &Tunnel{
		name:    name,
		streams: maps.New[string, Stream](),
		wsConn:  wsConn,
	}
}

// NewClientTunnel creates a new tunnel with the provided configuration and forwarder. A
// forwarder is effectively the protocol being forwarded. For example HTTP (Proxy), and TCP.
func NewClientTunnel(cfg *config.ClientConfig, forwarder Forwarder) (*Tunnel, error) {
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
		name:      cfg.TunnelName,
		wsConn:    serverConn,
		streams:   maps.New[string, Stream](),
		forwarder: forwarder,
	}, nil
}

type Tunnel struct {
	ctx       context.Context
	name      string
	wsConn    *websocket.Conn
	streams   *maps.Map[string, Stream]
	forwarder Forwarder

	mu sync.Mutex
}

func (t *Tunnel) Start(ctx context.Context) {
	log.Infof("initiated tunnel %q with %s", t.name, t.wsConn.RemoteAddr().String())
	defer log.Infof("closed tunnel %q with %s", t.name, t.wsConn.RemoteAddr().String())

	t.ctx = ctx

	// Start Message Receiver
	for {
		msg, err := t.readWSWithContext(ctx)
		if err != nil {
			return
		}

		// Validate Stream
		if msg.StreamID == "" {
			log.Warnf("tunnel %s missing streamID", t.name)
			continue
		}

		// Get Stream
		stream, err := t.getStream(msg.StreamID)
		if err != nil {
			if msg.Type != types.MessageTypeClose {
				log.WithError(err).Errorf("failed to get stream %s", msg.StreamID)
			}
			continue
		}

		// Handle Messages
		switch msg.Type {
		case types.MessageTypeClose:
			_ = t.closeStream(stream, msg.StreamID)
		case types.MessageTypeData:
			_, err = stream.Write(msg.Data)
		}

		// Log Error
		if err != nil {
			log.WithError(err).Errorf("failed to handle message %s", msg.StreamID)
		}
	}
}

func (t *Tunnel) readWSWithContext(ctx context.Context) (*types.Message, error) {
	type result struct {
		msg *types.Message
		err error
	}

	resultChan := make(chan result, 1)
	go func() {
		var msg types.Message
		err := t.wsConn.ReadJSON(&msg)
		resultChan <- result{&msg, err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result.msg, result.err
	}
}

func (t *Tunnel) AddStream(stream Stream, streamID string) error {
	if t.streams.HasKey(streamID) {
		return fmt.Errorf("stream %s already exists", streamID)
	}
	log.Infof("tunnel %q initiated stream with %s", t.name, stream.Source())
	t.streams.Set(streamID, stream)
	return nil
}

func (t *Tunnel) Source() string {
	return t.wsConn.RemoteAddr().String()
}

func (t *Tunnel) StartStream(stream Stream, streamID string) error {
	// Close Stream
	defer t.closeStream(stream, streamID)

	// Start Stream
	for {
		data, err := t.readStreamWithContext(t.ctx, stream)
		if err != nil {
			return err
		}

		if err := t.sendWS(&types.Message{
			Type:       types.MessageTypeData,
			StreamID:   streamID,
			Data:       data,
			SourceAddr: stream.Source(),
		}); err != nil {
			return err
		}
	}
}

func (t *Tunnel) closeStream(stream Stream, streamID string) error {
	log.Infof("tunnel %q closed stream with %s", t.name, stream.Source())
	t.streams.Delete(streamID)
	return stream.Close()
}

func (t *Tunnel) getStream(streamID string) (Stream, error) {
	// Check Existing Stream
	if stream, found := t.streams.Get(streamID); found {
		return stream, nil
	}

	// Check Forwarder
	if t.forwarder == nil {
		return nil, fmt.Errorf("stream %s does not exist", streamID)
	}

	// Initialize Forwarder & Add Stream
	stream, err := t.forwarder.Initialize()
	if err != nil {
		return nil, err
	}
	if err := t.AddStream(stream, streamID); err != nil {
		return nil, err
	}
	go t.StartStream(stream, streamID)
	return stream, nil
}

func (t *Tunnel) readStreamWithContext(ctx context.Context, stream Stream) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}

	resultChan := make(chan result, 1)
	go func() {
		buffer := make([]byte, 4096)
		n, err := stream.Read(buffer)
		resultChan <- result{buffer[:n], err}
	}()

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultChan:
		return result.data, result.err
	}
}

func (t *Tunnel) sendWS(msg *types.Message) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.wsConn.WriteJSON(msg)
}
