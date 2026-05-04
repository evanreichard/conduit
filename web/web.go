package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	log "github.com/sirupsen/logrus"
	"reichard.io/conduit/store"
	"reichard.io/conduit/web/pages"
)

type WebServer struct {
	store  store.TunnelStore
	server *http.Server
}

func NewWebServer(store store.TunnelStore) *WebServer {
	return &WebServer{store: store}
}

func (s *WebServer) Start(ctx context.Context) error {
	log.Info("started tunnel monitor at :8181")
	defer log.Info("stopped tunnel monitor")

	rootMux := http.NewServeMux()
	rootMux.HandleFunc("/", s.handleRoot)
	rootMux.HandleFunc("/assets/network.js", s.handleNetworkScript)
	rootMux.HandleFunc("/stream", s.handleStream)

	s.server = &http.Server{
		Addr:    ":8181",
		Handler: rootMux,
	}

	// Context & Cleanup
	go func() {
		<-ctx.Done()
		s.server.Shutdown(ctx)
	}()

	// Start Tunnel Monitor
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *WebServer) Stop(ctx context.Context) error {
	if s.server == nil {
		return nil
	}
	return s.server.Shutdown(ctx)
}

func (s *WebServer) handleStream(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Flusher Interface Upgrade
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Stream Data
	ch := s.store.Subscribe()
	done := r.Context().Done()

	for {
		select {
		case record, ok := <-ch:
			if !ok {
				return
			}
			data, _ := json.Marshal(record)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case <-done:
			return
		}
	}
}

func (s *WebServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(pages.NetworkHTML()))
}

func (s *WebServer) handleNetworkScript(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	_, _ = w.Write([]byte(pages.NetworkScript()))
}
