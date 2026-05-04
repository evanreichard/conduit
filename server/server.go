package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	ctx  context.Context
	host string
	cfg  *config.ServerConfig

	upgrader websocket.Upgrader
	tunnels  *maps.Map[string, *tunnel.Tunnel]
}

func NewServer(ctx context.Context, cfg *config.ServerConfig) (*Server, error) {
	serverURL, err := url.Parse(cfg.ServerAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to parse server address: %v", err)
	} else if serverURL.Host == "" {
		return nil, errors.New("invalid server address")
	}

	return &Server{
		ctx:     ctx,
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
	// HTTP Server - Uses stdlib http.Server for proper HTTP response handling
	// including Content-Length, chunked encoding, and keep-alive semantics.
	httpServer := &http.Server{
		Addr:    s.cfg.BindAddress,
		Handler: s,
	}

	// Context Cancellation - Gracefully shut down when the context is cancelled.
	go func() {
		<-s.ctx.Done()
		log.Info("conduit server shutting down")
		httpServer.Close()
	}()

	// Start Server
	log.Infof("conduit server listening on %s", s.cfg.BindAddress)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get True Host
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		r.RemoteAddr = xff
	}

	// Validate Host
	if !strings.Contains(r.Host, s.host) {
		http.Error(w, fmt.Sprintf("unknown host: %s", r.Host), http.StatusBadRequest)
		return
	}

	// Extract Subdomain
	tunnelName := strings.TrimSuffix(strings.Replace(r.Host, s.host, "", 1), ".")
	if strings.Count(tunnelName, ".") != 0 {
		http.Error(w, fmt.Sprintf("cannot tunnel nested subdomains: %s", r.Host), http.StatusBadRequest)
		return
	}

	// Handle Control Endpoints
	if tunnelName == "" {
		s.handleAsHTTP(w, r)
		return
	}

	// Handle Tunnel Requests
	s.handleTunnelRequest(w, r, tunnelName)
}

func (s *Server) handleTunnelRequest(w http.ResponseWriter, r *http.Request, tunnelName string) {
	// Get Tunnel
	conduitTunnel, exists := s.tunnels.Get(tunnelName)
	if !exists {
		http.Error(w, fmt.Sprintf("unknown tunnel: %s", tunnelName), http.StatusNotFound)
		return
	}

	// Hijack Connection - Take over the raw TCP connection from the HTTP server
	// so we can forward the full request (including body) through the tunnel.
	hj, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	conn, bufrw, err := hj.Hijack()
	if err != nil {
		http.Error(w, fmt.Sprintf("hijack failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Re-Serialize Request Headers - The HTTP server already consumed the request
	// from the connection. We re-serialize it so the tunnel client receives a
	// complete HTTP request to forward to the local target.
	var reqBuf bytes.Buffer
	fmt.Fprintf(&reqBuf, "%s %s %s\r\n", r.Method, r.RequestURI, r.Proto)
	fmt.Fprintf(&reqBuf, "Host: %s\r\n", r.Host)
	_ = r.Header.Write(&reqBuf)
	reqBuf.WriteString("\r\n")

	// Reconstruct Connection - Combine re-serialized headers with any buffered
	// body data (from the hijacked reader) and the raw connection.
	reconstructedConn := newReconstructedConn(conn, &reqBuf, bufrw)

	// Create Stream
	streamID := fmt.Sprintf("stream_%d", time.Now().UnixNano())
	tunnelStream := tunnel.NewStream(reconstructedConn, r.RemoteAddr, conduitTunnel.Source())

	// Add Stream
	if err := conduitTunnel.AddStream(tunnelStream, streamID); err != nil {
		log.WithError(err).Error("failed to add stream")
		conn.Close()
		return
	}

	// Start Stream
	conduitTunnel.StartStream(tunnelStream, streamID)
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
		http.Error(w, "Missing tunnelName parameter", http.StatusBadRequest)
		return
	}

	// Validate Unique
	if _, exists := s.tunnels.Get(tunnelName); exists {
		http.Error(w, "Tunnel already registered", http.StatusConflict)
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

	// Start Tunnel - This is blocking
	conduitTunnel.Start(s.ctx)

	// Cleanup Tunnel
	s.tunnels.Delete(tunnelName)
	_ = wsConn.Close()
}
