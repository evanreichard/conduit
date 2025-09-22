package tunnel

import (
	"fmt"
	"io"
	"net"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/types"
)

func NewServerTunnel(name string, wsConn *websocket.Conn) *Tunnel {
	return &Tunnel{
		name:    name,
		wsConn:  wsConn,
		streams: make(map[string]io.ReadWriteCloser),
	}
}

func NewClientTunnel(name, target string, wsConn *websocket.Conn) *Tunnel {
	return &Tunnel{
		name:    name,
		wsConn:  wsConn,
		streams: make(map[string]io.ReadWriteCloser),
		connBuilder: func() (io.ReadWriteCloser, error) {
			return net.Dial("tcp", target)
		},
	}
}

type Tunnel struct {
	name        string
	wsConn      *websocket.Conn
	streams     map[string]io.ReadWriteCloser
	connBuilder func() (io.ReadWriteCloser, error)

	wsMu, streamsMu sync.Mutex
}

func (t *Tunnel) Start() {
	for {
		var msg types.Message
		err := t.wsConn.ReadJSON(&msg)
		if err != nil {
			return
		}

		// Validate Stream
		if msg.StreamID == "" {
			log.Warnf("tunnel %s missing streamID", t.name)
			continue
		}

		// Ensure Stream
		if err := t.initStreamConnection(msg.StreamID); err != nil {
			log.WithError(err).Errorf("failed to initialize stream %s connection", t.name)
			continue
		}

		// Handle Messages
		switch msg.Type {
		case types.MessageTypeClose:
			_ = t.CloseStream(msg.StreamID)
		case types.MessageTypeData:
			_ = t.WriteStream(msg.StreamID, msg.Data)
		}
	}
}

func (t *Tunnel) initStreamConnection(streamID string) error {
	if t.connBuilder == nil {
		return nil
	}

	if _, found := t.getStream(streamID); found {
		return nil
	}

	conn, err := t.connBuilder()
	if err != nil {
		return err
	}

	if err := t.AddStream(streamID, conn); err != nil {
		return err
	}

	go t.StartStream(streamID)
	return nil
}

func (t *Tunnel) AddStream(streamID string, conn io.ReadWriteCloser) error {
	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()

	if _, found := t.streams[streamID]; found {
		return fmt.Errorf("stream %s already exists", streamID)
	}
	t.streams[streamID] = conn
	return nil
}

func (t *Tunnel) StartStream(streamID string) error {
	// Get Stream
	conn, found := t.getStream(streamID)
	if !found {
		return fmt.Errorf("stream %s does not exist", streamID)
	}

	// Close Stream
	defer func() {
		_ = t.sendWS(&types.Message{
			Type:     types.MessageTypeClose,
			StreamID: streamID,
		})

		t.CloseStream(streamID)
	}()

	// Start Stream
	buffer := make([]byte, 4096)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			return err
		}

		if err := t.sendWS(&types.Message{
			Type:     types.MessageTypeData,
			Data:     buffer[:n],
			StreamID: streamID,
		}); err != nil {
			return err
		}
	}
}

func (t *Tunnel) WriteStream(streamID string, data []byte) error {
	// Get Stream
	conn, found := t.getStream(streamID)
	if !found {
		return fmt.Errorf("stream %s does not exist", streamID)
	}

	_, err := conn.Write(data)
	return err
}

func (t *Tunnel) CloseStream(streamID string) error {
	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()
	if conn, ok := t.streams[streamID]; ok {
		delete(t.streams, streamID)
		return conn.Close()
	}
	return nil
}

func (t *Tunnel) Source() string {
	return t.wsConn.RemoteAddr().String()
}

func (t *Tunnel) sendWS(msg *types.Message) error {
	t.wsMu.Lock()
	defer t.wsMu.Unlock()
	return t.wsConn.WriteJSON(msg)
}

func (t *Tunnel) getStream(streamID string) (io.ReadWriteCloser, bool) {
	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()

	if conn, ok := t.streams[streamID]; ok {
		return conn, true
	}
	return nil, false
}
