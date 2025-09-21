package tunnel

import (
	"io"
	"sync"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/types"
)

func NewTunnel(name string, wsConn *websocket.Conn) *Tunnel {
	return &Tunnel{
		name:    name,
		wsConn:  wsConn,
		streams: make(map[string]io.ReadWriteCloser),
	}
}

type Tunnel struct {
	name    string
	wsConn  *websocket.Conn
	streams map[string]io.ReadWriteCloser

	wsMu, streamsMu sync.Mutex
}

// Start starts the tunnel and is the primary loop that handles all websocket messages.
// Messages are relayed to the local stream.
func (t *Tunnel) Start() {
	for {
		var msg types.Message
		err := t.wsConn.ReadJSON(&msg)
		if err != nil {
			return
		}

		if msg.StreamID == "" {
			log.Warnf("tunnel %s missing streamID", t.name)
			continue
		}

		switch msg.Type {
		case types.MessageTypeClose:
			t.CloseStream(msg.StreamID)
		case types.MessageTypeData:
			t.WriteStream(msg.StreamID, msg.Data)
		}
	}
}

func (t *Tunnel) NewStream(streamID string, localConn io.ReadWriteCloser) {
	t.streamsMu.Lock()
	t.streams[streamID] = localConn
	t.streamsMu.Unlock()

	defer t.CloseStream(streamID)
	buffer := make([]byte, 4096)
	for {
		n, err := localConn.Read(buffer)
		if err != nil {
			return
		}

		if err := t.sendWS(&types.Message{
			Type:     types.MessageTypeData,
			Data:     buffer[:n],
			StreamID: streamID,
		}); err != nil {
			return
		}
	}
}

func (t *Tunnel) WriteStream(streamID string, data []byte) {
	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()
	if localConn, ok := t.streams[streamID]; ok {
		_, _ = localConn.Write(data)
	} else {
		log.Infof("stream %s does not exist", streamID)
	}
}

func (t *Tunnel) CloseStream(streamID string) {
	_ = t.sendWS(&types.Message{
		Type:     types.MessageTypeClose,
		StreamID: streamID,
	})

	t.streamsMu.Lock()
	defer t.streamsMu.Unlock()
	if localConn, ok := t.streams[streamID]; ok {
		delete(t.streams, streamID)
		_ = localConn.Close()
	}
}

func (t *Tunnel) Source() string {
	return t.wsConn.RemoteAddr().String()
}

func (t *Tunnel) sendWS(msg *types.Message) error {
	t.wsMu.Lock()
	defer t.wsMu.Unlock()
	return t.wsConn.WriteJSON(msg)
}
