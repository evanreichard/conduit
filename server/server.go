package server

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/config"
	"reichard.io/conduit/types"
)

type TunnelConnection struct {
	*websocket.Conn
	name    string
	streams map[string]chan []byte
}

type Server struct {
	host string
	cfg  *config.ServerConfig
	mu   sync.RWMutex

	upgrader websocket.Upgrader
	tunnels  map[string]*TunnelConnection
}

func NewServer(cfg *config.ServerConfig) (*Server, error) {
	serverURL, err := url.Parse(cfg.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server address: %v", err)
	} else if serverURL.Host == "" {
		return nil, errors.New("invalid server address")
	}

	return &Server{
		cfg:     cfg,
		host:    serverURL.Host,
		tunnels: make(map[string]*TunnelConnection),
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
		},
	}, nil
}

func (s *Server) Start() error {
	// Raw TCP Listener - This is necessary so we can conditionally either relay
	// the raw TCP connection, or handle conduit control server API requests.
	listener, err := net.Listen("tcp", s.cfg.BindAddress)
	if err != nil {
		return err
	}
	defer listener.Close()

	// Start Listening
	log.Infof("conduit server listening on %s", s.cfg.BindAddress)
	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("error accepting connection: %v", err)
			continue
		}

		go s.handleRawConnection(conn)
	}
}

func (s *Server) getStatus(w http.ResponseWriter, _ *http.Request) {
	s.mu.RLock()
	count := len(s.tunnels)
	s.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)

	response := fmt.Sprintf(`{"tunnels": %d}`, count)
	_, _ = w.Write([]byte(response))
}

func (s *Server) proxyRawConnection(clientConn net.Conn, tunnelConn *TunnelConnection, dataReader io.Reader) {
	defer clientConn.Close()

	// Create Identifiers
	streamID := fmt.Sprintf("stream_%d", time.Now().UnixNano())
	responseChan := make(chan []byte, 100)

	// Register Stream
	s.mu.Lock()
	if tunnelConn.streams == nil {
		tunnelConn.streams = make(map[string]chan []byte)
	}
	tunnelConn.streams[streamID] = responseChan
	s.mu.Unlock()

	// Clean Up
	defer func() {
		s.mu.Lock()
		delete(tunnelConn.streams, streamID)
		close(responseChan)
		s.mu.Unlock()

		// Send Close
		closeMsg := types.Message{
			Type:     types.MessageTypeClose,
			StreamID: streamID,
		}
		_ = tunnelConn.WriteJSON(closeMsg)
	}()

	// Read & Send Chunks
	go func() {
		buffer := make([]byte, 4096)
		for {
			n, err := dataReader.Read(buffer)
			if err != nil {
				return
			}

			if err := tunnelConn.WriteJSON(types.Message{
				Type:     types.MessageTypeData,
				StreamID: streamID,
				Data:     buffer[:n],
			}); err != nil {
				return
			}
		}
	}()

	// Return Response Data
	for data := range responseChan {
		if _, err := clientConn.Write(data); err != nil {
			break
		}
	}
}

func (s *Server) handleRawConnection(conn net.Conn) {
	defer conn.Close()

	// Capture Consumed Data - When determining where to route the request, we
	// have to read the host headers. This requires reading from the buffer, so
	// if we later decide to tunnel the TCP connection we need to reconstruct the
	// data from the buffer.
	var capturedData bytes.Buffer
	teeReader := io.TeeReader(conn, &capturedData)
	bufReader := bufio.NewReader(teeReader)

	// Create HTTP Request & Writer
	w := &connResponseWriter{conn: conn}
	r, err := http.ReadRequest(bufReader)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Validate Host
	if !strings.Contains(r.Host, s.host) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "unknown host: %s", r.Host)
		return
	}

	// Extract Subdomain
	subdomain := strings.TrimSuffix(strings.Replace(r.Host, s.host, "", 1), ".")
	if strings.Count(subdomain, ".") != 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, "cannot tunnel nested subdomains: %s", r.Host)
		return
	}

	// Handle Control Endpoints
	if subdomain == "" {
		s.handleAsHTTP(w, r)
		return
	}

	// Handle Tunnels
	s.mu.RLock()
	tunnelConn, exists := s.tunnels[subdomain]
	s.mu.RUnlock()
	if exists {
		log.Infof("relaying %s to tunnel", subdomain)

		// Reconstruct Data & Proxy Connection
		allReader := io.MultiReader(&capturedData, r.Body)
		s.proxyRawConnection(conn, tunnelConn, allReader)
	}
}

func (s *Server) handleAsHTTP(w http.ResponseWriter, r *http.Request) {
	// Authorize Control Endpoints
	apiKey := r.URL.Query().Get("apiKey")
	if apiKey != s.cfg.APIKey {
		log.Error("unauthorized client")
		w.WriteHeader(http.StatusUnauthorized)
		return
	}

	// Handle Control Endpoints
	switch r.URL.Path {
	case "/_conduit/tunnel":
		s.createTunnel(w, r)
	case "/_conduit/status":
		s.getStatus(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func (s *Server) handleTunnelMessages(tunnel *TunnelConnection) {
	for {
		var msg types.Message
		err := tunnel.ReadJSON(&msg)
		if err != nil {
			return
		}

		if msg.StreamID == "" {
			log.Infof("tunnel %s missing streamID", tunnel.name)
			continue
		}

		switch msg.Type {
		case types.MessageTypeClose:
			return
		case types.MessageTypeData:
			s.mu.RLock()
			streamChan, exists := tunnel.streams[msg.StreamID]
			if !exists {
				log.Infof("stream %s does not exist", msg.StreamID)
				s.mu.RUnlock()
				continue
			}

			select {
			case streamChan <- msg.Data:
			case <-time.After(time.Second):
				log.Warnf("stream %s channel full, dropping data", msg.StreamID)
			}
			s.mu.RUnlock()
		}
	}
}
func (s *Server) createTunnel(w http.ResponseWriter, r *http.Request) {
	// Get Tunnel Name
	tunnelName := r.URL.Query().Get("tunnelName")
	if tunnelName == "" {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("Missing tunnelName parameter"))
		return
	}

	// Validate Unique
	if _, exists := s.tunnels[tunnelName]; exists {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte("Tunnel already registered"))
		return
	}

	// Upgrade Connection
	wsConn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("websocket upgrade failed: %v", err)
		return
	}

	// Create & Cache TunnelConnection
	tunnel := &TunnelConnection{
		Conn:    wsConn,
		name:    tunnelName,
		streams: make(map[string]chan []byte),
	}
	s.mu.Lock()
	s.tunnels[tunnelName] = tunnel
	s.mu.Unlock()
	log.Infof("tunnel established: %s", tunnelName)

	// Keep connection alive and handle cleanup
	defer func() {
		s.mu.Lock()
		delete(s.tunnels, tunnelName)
		s.mu.Unlock()
		_ = wsConn.Close()
		log.Infof("tunnel closed: %s", tunnelName)
	}()

	// Handle tunnel messages
	s.handleTunnelMessages(tunnel)
}
