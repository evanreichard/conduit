package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/config"
	"reichard.io/conduit/pkg/maps"
	"reichard.io/conduit/tunnel"
)

type InfoResponse struct {
	Tunnels []TunnelInfo `json:"tunnels"`
	Version string       `json:"version"`
}

type TunnelInfo struct {
	Name   string `json:"name"`
	Target string `json:"target"`
}

type Server struct {
	host string
	cfg  *config.ServerConfig

	upgrader websocket.Upgrader
	tunnels  *maps.Map[string, *tunnel.Tunnel]
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
		tunnels: maps.New[string, *tunnel.Tunnel](),
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
			log.WithError(err).Error("error accepting connection")
			continue
		}

		go s.handleRawConnection(conn)
	}
}

func (s *Server) getInfo(w http.ResponseWriter, _ *http.Request) {
	// Get Tunnels
	var allTunnels []TunnelInfo
	for t, c := range s.tunnels.Entries() {
		allTunnels = append(allTunnels, TunnelInfo{
			Name:   t,
			Target: c.Source(),
		})
	}

	// Create Response
	d, err := json.MarshalIndent(InfoResponse{
		Tunnels: allTunnels,
		Version: config.GetVersion(),
	}, "", "  ")
	if err != nil {
		log.WithError(err).Error("failed to marshal info")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	// Send Response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(d)
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
	w := &rawHTTPResponseWriter{conn: conn}
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
	conduitTunnel, exists := s.tunnels.Get(subdomain)
	if !exists {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprintf(w, "unknown tunnel: %s", subdomain)
		return
	}

	// Add & Start Stream
	reconstructedConn := newReconstructedConn(conn, &capturedData)
	streamID := fmt.Sprintf("stream_%d", time.Now().UnixNano())
	if err := conduitTunnel.AddStream(streamID, reconstructedConn); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprintf(w, "failed to add stream: %v", err)
		return
	}

	log.Infof("relaying %s to tunnel", subdomain)
	_ = conduitTunnel.StartStream(streamID)
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
	case "/_conduit/info":
		s.getInfo(w, r)
	default:
		w.WriteHeader(http.StatusNotFound)
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
	if _, exists := s.tunnels.Get(tunnelName); exists {
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

	// Create Tunnel
	conduitTunnel := tunnel.NewServerTunnel(tunnelName, wsConn)
	s.tunnels.Set(tunnelName, conduitTunnel)
	log.Infof("tunnel established: %s", tunnelName)

	// Start Tunnel - This is blocking
	conduitTunnel.Start()

	// Cleanup Tunnel
	s.tunnels.Delete(tunnelName)
	_ = wsConn.Close()
	log.Infof("tunnel closed: %s", tunnelName)
}
