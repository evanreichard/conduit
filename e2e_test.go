package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"reichard.io/conduit/config"
	"reichard.io/conduit/server"
	"reichard.io/conduit/store"
	"reichard.io/conduit/tunnel"
	"reichard.io/conduit/web"
)

// ---------- Helpers ----------

// startConduitServer creates and starts a conduit server on a random port.
// Returns the server address (host:port) and a cancel func for teardown.
func startConduitServer(t *testing.T, apiKey string) (string, context.CancelFunc) {
	t.Helper()

	// Find Free Port
	port := getFreePort(t)
	bindAddr := fmt.Sprintf("127.0.0.1:%d", port)
	serverAddr := fmt.Sprintf("http://%s", bindAddr)

	cfg := &config.ServerConfig{
		BaseConfig: config.BaseConfig{
			ServerAddress: serverAddr,
			APIKey:        apiKey,
			LogLevel:      "error",
			LogFormat:     "text",
		},
		BindAddress: bindAddr,
	}

	ctx, cancel := context.WithCancel(context.Background())
	srv, err := server.NewServer(ctx, cfg)
	if err != nil {
		cancel()
		t.Fatalf("failed to create server: %v", err)
	}

	// Start Server in Background
	errCh := make(chan error, 1)
	go func() { errCh <- srv.Start() }()

	// Wait for Server to Accept
	waitForPort(t, bindAddr, 3*time.Second)

	// Check Early Errors
	select {
	case err := <-errCh:
		cancel()
		t.Fatalf("server exited early: %v", err)
	default:
	}

	return bindAddr, cancel
}

// startHTTPTarget creates a simple HTTP server that echoes request info.
func startHTTPTarget(t *testing.T) (string, context.CancelFunc) {
	t.Helper()

	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Header", "present")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "echo: %s %s", r.Method, r.URL.Path)
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("/post", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "received: %s", string(body))
	})

	srv := &http.Server{Addr: addr, Handler: mux}
	ctx, cancel := context.WithCancel(context.Background())

	go func() { srv.ListenAndServe() }()
	go func() { <-ctx.Done(); srv.Close() }()

	waitForPort(t, addr, 3*time.Second)

	return addr, cancel
}

// startTCPEchoTarget creates a TCP server that echoes back whatever it receives.
func startTCPEchoTarget(t *testing.T) (string, context.CancelFunc) {
	t.Helper()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start tcp echo: %v", err)
	}
	addr := listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())

	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				io.Copy(c, c)
			}(conn)
		}
	}()

	return addr, cancel
}

// connectTunnel creates a conduit tunnel client and starts it.
func connectTunnel(t *testing.T, serverAddr, targetAddr, tunnelName, apiKey string) context.CancelFunc {
	t.Helper()

	cfg := &config.ClientConfig{
		BaseConfig: config.BaseConfig{
			ServerAddress: fmt.Sprintf("http://%s", serverAddr),
			APIKey:        apiKey,
			LogLevel:      "error",
			LogFormat:     "text",
		},
		TunnelName:   tunnelName,
		TunnelTarget: targetAddr,
	}

	// Create Tunnel Store
	tunnelStore := store.NewTunnelStore(100)

	// Create Forwarder
	forwarder, err := tunnel.NewForwarder(cfg.TunnelTarget, tunnelStore)
	if err != nil {
		t.Fatalf("failed to create forwarder: %v", err)
	}

	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())

	// Start Forwarder
	wg.Add(1)
	go func() {
		defer wg.Done()
		forwarder.Start(ctx)
	}()

	// Create & Start Tunnel
	tun, err := tunnel.NewClientTunnel(cfg, forwarder)
	if err != nil {
		cancel()
		t.Fatalf("failed to create tunnel: %v", err)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		tun.Start(ctx)
	}()

	// Start Web Server
	webServer := web.NewWebServer(tunnelStore)
	wg.Add(1)
	go func() {
		defer wg.Done()
		webServer.Start(ctx)
	}()

	// Brief Settle Time
	time.Sleep(100 * time.Millisecond)

	cleanup := func() {
		cancel()
		wg.Wait()
	}
	return cleanup
}

// sendHTTPViaTunnel sends an HTTP request through the conduit server to a tunnel.
func sendHTTPViaTunnel(t *testing.T, serverAddr, tunnelName, method, path, body string) *http.Response {
	t.Helper()

	url := fmt.Sprintf("http://%s%s", serverAddr, path)
	var bodyReader io.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	// Route via Subdomain
	req.Host = fmt.Sprintf("%s.%s", tunnelName, serverAddr)

	client := &http.Client{
		Timeout:   10 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}

func readBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read body: %v", err)
	}
	return string(b)
}

func getFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to get free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func waitForPort(t *testing.T, addr string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("port %s not ready after %s", addr, timeout)
}

// ---------- Tests ----------

func TestHTTPTunnelRoundTrip(t *testing.T) {
	apiKey := "test-key-http"

	// Start Target HTTP Server
	targetAddr, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", targetAddr), "http-test", apiKey)
	defer stopTunnel()

	// GET /
	resp := sendHTTPViaTunnel(t, serverAddr, "http-test", "GET", "/", "")
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "echo: GET /") {
		t.Errorf("unexpected body: %s", body)
	}

	// GET /health
	resp = sendHTTPViaTunnel(t, serverAddr, "http-test", "GET", "/health", "")
	body = readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if body != "ok" {
		t.Errorf("expected 'ok', got %q", body)
	}
}

func TestHTTPTunnelPOST(t *testing.T) {
	apiKey := "test-key-post"

	// Start Target HTTP Server
	targetAddr, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", targetAddr), "post-test", apiKey)
	defer stopTunnel()

	// POST /post
	resp := sendHTTPViaTunnel(t, serverAddr, "post-test", "POST", "/post", "hello world")
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "received: hello world") {
		t.Errorf("unexpected body: %s", body)
	}
}

func TestUnknownTunnelReturns404(t *testing.T) {
	apiKey := "test-key-404"

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Request to Non-Existent Tunnel
	resp := sendHTTPViaTunnel(t, serverAddr, "no-such-tunnel", "GET", "/", "")
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "unknown tunnel") {
		t.Errorf("expected 'unknown tunnel' error, got: %s", body)
	}
}

func TestDuplicateTunnelNameRejected(t *testing.T) {
	apiKey := "test-key-dup"

	// Start Target HTTP Server
	targetAddr, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect First Tunnel
	stopTunnel1 := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", targetAddr), "dup-test", apiKey)
	defer stopTunnel1()

	// Attempt Duplicate — this should fail at WebSocket dial
	cfg := &config.ClientConfig{
		BaseConfig: config.BaseConfig{
			ServerAddress: fmt.Sprintf("http://%s", serverAddr),
			APIKey:        apiKey,
			LogLevel:      "error",
			LogFormat:     "text",
		},
		TunnelName:   "dup-test",
		TunnelTarget: fmt.Sprintf("http://%s", targetAddr),
	}

	tunnelStore := store.NewTunnelStore(100)
	forwarder, err := tunnel.NewForwarder(cfg.TunnelTarget, tunnelStore)
	if err != nil {
		t.Fatalf("failed to create forwarder: %v", err)
	}

	_, err = tunnel.NewClientTunnel(cfg, forwarder)
	if err == nil {
		t.Error("expected error for duplicate tunnel name, got nil")
	}
}

func TestUnauthorizedControlAccess(t *testing.T) {
	apiKey := "test-key-auth"

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Request Info with Wrong API Key
	url := fmt.Sprintf("http://%s/_conduit/info?apiKey=wrong-key", serverAddr)
	req, _ := http.NewRequest("GET", url, nil)
	req.Host = serverAddr

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", resp.StatusCode)
	}
}

func TestInfoEndpointListsTunnels(t *testing.T) {
	apiKey := "test-key-info"

	// Start Target HTTP Server
	targetAddr, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", targetAddr), "info-test", apiKey)
	defer stopTunnel()

	// Query Info Endpoint
	url := fmt.Sprintf("http://%s/_conduit/info?apiKey=%s", serverAddr, apiKey)
	req, _ := http.NewRequest("GET", url, nil)
	req.Host = serverAddr

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	body := readBody(t, resp)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
	if !strings.Contains(body, "info-test") {
		t.Errorf("expected tunnel 'info-test' in response: %s", body)
	}
}

func TestMultipleTunnelsRouteCorrectly(t *testing.T) {
	apiKey := "test-key-multi"

	// Start Two Separate Target Servers
	target1Addr, stopTarget1 := startHTTPTarget(t)
	defer stopTarget1()

	port2 := getFreePort(t)
	addr2 := fmt.Sprintf("127.0.0.1:%d", port2)
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, "target-two")
	})
	srv2 := &http.Server{Addr: addr2, Handler: mux2}
	go srv2.ListenAndServe()
	defer srv2.Close()
	waitForPort(t, addr2, 3*time.Second)

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Two Tunnels
	stopTunnel1 := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", target1Addr), "multi-one", apiKey)
	defer stopTunnel1()

	stopTunnel2 := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", addr2), "multi-two", apiKey)
	defer stopTunnel2()

	// Request to First Tunnel
	resp1 := sendHTTPViaTunnel(t, serverAddr, "multi-one", "GET", "/", "")
	body1 := readBody(t, resp1)
	if !strings.Contains(body1, "echo: GET /") {
		t.Errorf("tunnel one unexpected body: %s", body1)
	}

	// Request to Second Tunnel
	resp2 := sendHTTPViaTunnel(t, serverAddr, "multi-two", "GET", "/", "")
	body2 := readBody(t, resp2)
	if body2 != "target-two" {
		t.Errorf("tunnel two expected 'target-two', got: %s", body2)
	}
}

func TestServerGracefulShutdown(t *testing.T) {
	apiKey := "test-key-shutdown"

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)

	// Cancel Server
	stopServer()

	// Verify Port Is Closed
	time.Sleep(200 * time.Millisecond)
	conn, err := net.DialTimeout("tcp", serverAddr, 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Error("expected server port to be closed after shutdown")
	}
}

// ---------- HTTP Response Quality Tests ----------

func TestHTTPResponseHasProperHeaders(t *testing.T) {
	apiKey := "test-key-headers"

	// Start Target HTTP Server
	targetAddr, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", targetAddr), "hdr-test", apiKey)
	defer stopTunnel()

	// Send Request Through Tunnel
	resp := sendHTTPViaTunnel(t, serverAddr, "hdr-test", "GET", "/", "")
	defer resp.Body.Close()

	// Verify Proper HTTP Semantics
	if resp.Proto != "HTTP/1.1" {
		t.Errorf("expected HTTP/1.1, got %s", resp.Proto)
	}
	if resp.Header.Get("X-Test-Header") != "present" {
		t.Errorf("expected X-Test-Header: present, got %q", resp.Header.Get("X-Test-Header"))
	}
	if resp.ContentLength <= 0 && resp.TransferEncoding == nil {
		t.Errorf("expected Content-Length or Transfer-Encoding, got neither")
	}
}

func TestHTTPControlEndpointResponseQuality(t *testing.T) {
	apiKey := "test-key-ctrl-quality"

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// 404 on Unknown Tunnel — Verify stdlib response format
	resp := sendHTTPViaTunnel(t, serverAddr, "nope", "GET", "/", "")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404, got %d", resp.StatusCode)
	}

	// Content-Type Should Be Set by http.Error
	ct := resp.Header.Get("Content-Type")
	if !strings.Contains(ct, "text/plain") {
		t.Errorf("expected text/plain Content-Type, got %q", ct)
	}

	// Content-Length Should Be Present
	if resp.ContentLength <= 0 {
		t.Errorf("expected positive Content-Length, got %d", resp.ContentLength)
	}

	// Info Endpoint — JSON response quality
	url := fmt.Sprintf("http://%s/_conduit/info?apiKey=%s", serverAddr, apiKey)
	req, _ := http.NewRequest("GET", url, nil)
	req.Host = serverAddr

	resp2, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		t.Fatalf("info request failed: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.Header.Get("Content-Type") != "application/json" {
		t.Errorf("expected application/json, got %q", resp2.Header.Get("Content-Type"))
	}
}

// ---------- Large Body Tests ----------

func TestHTTPLargeResponseBody(t *testing.T) {
	apiKey := "test-key-large-resp"

	// Start Target That Returns a Large Body
	largeBody := strings.Repeat("A", 1024*1024) // 1 MB
	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/large", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(largeBody))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	defer srv.Close()
	waitForPort(t, addr, 3*time.Second)

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", addr), "large-resp", apiKey)
	defer stopTunnel()

	// Request Large Response
	resp := sendHTTPViaTunnel(t, serverAddr, "large-resp", "GET", "/large", "")
	body := readBody(t, resp)

	if len(body) != len(largeBody) {
		t.Errorf("expected %d bytes, got %d", len(largeBody), len(body))
	}
}

func TestHTTPLargeRequestBody(t *testing.T) {
	apiKey := "test-key-large-req"

	// Start Target That Echoes Body Size
	port := getFreePort(t)
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "size:%d", len(data))
	})
	srv := &http.Server{Addr: addr, Handler: mux}
	go srv.ListenAndServe()
	defer srv.Close()
	waitForPort(t, addr, 3*time.Second)

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", addr), "large-req", apiKey)
	defer stopTunnel()

	// Send Large Request Body (512 KB)
	largePayload := strings.Repeat("B", 512*1024)
	resp := sendHTTPViaTunnel(t, serverAddr, "large-req", "POST", "/upload", largePayload)
	body := readBody(t, resp)

	expected := fmt.Sprintf("size:%d", len(largePayload))
	if body != expected {
		t.Errorf("expected %q, got %q", expected, body)
	}
}

// ---------- TCP Tunnel Tests ----------

func TestTCPTunnelEcho(t *testing.T) {
	apiKey := "test-key-tcp"

	// Start TCP Echo Server
	tcpAddr, stopTCP := startTCPEchoTarget(t)
	defer stopTCP()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect TCP Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("tcp://%s", tcpAddr), "tcp-test", apiKey)
	defer stopTunnel()

	// Send Raw HTTP Request Through Tunnel to TCP Echo
	conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Write a Raw HTTP Request (the TCP echo will bounce it back)
	reqLine := fmt.Sprintf("GET / HTTP/1.1\r\nHost: tcp-test.%s\r\n\r\n", serverAddr)
	_, err = conn.Write([]byte(reqLine))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Read Echoed Data
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("failed to read echo: %v", err)
	}

	response := string(buf[:n])
	if !strings.Contains(response, "GET / HTTP/1.1") {
		t.Errorf("expected echoed request, got: %q", response)
	}
}

func TestTCPTunnelLargePayload(t *testing.T) {
	apiKey := "test-key-tcp-large"

	// Start TCP Echo Server
	tcpAddr, stopTCP := startTCPEchoTarget(t)
	defer stopTCP()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect TCP Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("tcp://%s", tcpAddr), "tcp-large", apiKey)
	defer stopTunnel()

	// Connect and Send Large Payload
	conn, err := net.DialTimeout("tcp", serverAddr, 5*time.Second)
	if err != nil {
		t.Fatalf("failed to connect: %v", err)
	}
	defer conn.Close()

	// Route via Host Header Then Send 64 KB Payload
	header := fmt.Sprintf("POST /data HTTP/1.1\r\nHost: tcp-large.%s\r\nContent-Length: 65536\r\n\r\n", serverAddr)
	payload := header + strings.Repeat("X", 64*1024)

	_, err = conn.Write([]byte(payload))
	if err != nil {
		t.Fatalf("failed to write: %v", err)
	}

	// Read All Echoed Data
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	var received []byte
	buf := make([]byte, 8192)
	for len(received) < len(payload) {
		n, err := conn.Read(buf)
		if err != nil {
			break
		}
		received = append(received, buf[:n]...)
	}

	if len(received) != len(payload) {
		t.Errorf("expected %d bytes echoed, got %d", len(payload), len(received))
	}
}

// ---------- Concurrency Tests ----------

func TestConcurrentHTTPRequests(t *testing.T) {
	apiKey := "test-key-concurrent"

	// Start Target HTTP Server
	targetAddr, stopTarget := startHTTPTarget(t)
	defer stopTarget()

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Tunnel
	stopTunnel := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", targetAddr), "conc-test", apiKey)
	defer stopTunnel()

	// Fire 20 Concurrent Requests
	const numRequests = 20
	var wg sync.WaitGroup
	errors := make(chan string, numRequests)

	for i := range numRequests {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			path := fmt.Sprintf("/item/%d", idx)
			resp := sendHTTPViaTunnel(t, serverAddr, "conc-test", "GET", path, "")
			body := readBody(t, resp)

			expected := fmt.Sprintf("echo: GET %s", path)
			if !strings.Contains(body, expected) {
				errors <- fmt.Sprintf("request %d: expected %q, got %q", idx, expected, body)
			}
			if resp.StatusCode != http.StatusOK {
				errors <- fmt.Sprintf("request %d: expected 200, got %d", idx, resp.StatusCode)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for errMsg := range errors {
		t.Error(errMsg)
	}
}

func TestConcurrentMultiTunnelRequests(t *testing.T) {
	apiKey := "test-key-conc-multi"

	// Start Two Target Servers
	target1Addr, stopTarget1 := startHTTPTarget(t)
	defer stopTarget1()

	port2 := getFreePort(t)
	addr2 := fmt.Sprintf("127.0.0.1:%d", port2)
	mux2 := http.NewServeMux()
	mux2.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "server-two: %s", r.URL.Path)
	})
	srv2 := &http.Server{Addr: addr2, Handler: mux2}
	go srv2.ListenAndServe()
	defer srv2.Close()
	waitForPort(t, addr2, 3*time.Second)

	// Start Conduit Server
	serverAddr, stopServer := startConduitServer(t, apiKey)
	defer stopServer()

	// Connect Two Tunnels
	stopTunnel1 := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", target1Addr), "cm-one", apiKey)
	defer stopTunnel1()

	stopTunnel2 := connectTunnel(t, serverAddr, fmt.Sprintf("http://%s", addr2), "cm-two", apiKey)
	defer stopTunnel2()

	// Fire Concurrent Requests to Both Tunnels
	const perTunnel = 10
	var wg sync.WaitGroup
	errors := make(chan string, perTunnel*2)

	for i := range perTunnel {
		wg.Add(2)

		// Requests to Tunnel One
		go func(idx int) {
			defer wg.Done()
			path := fmt.Sprintf("/a/%d", idx)
			resp := sendHTTPViaTunnel(t, serverAddr, "cm-one", "GET", path, "")
			body := readBody(t, resp)
			if !strings.Contains(body, fmt.Sprintf("echo: GET %s", path)) {
				errors <- fmt.Sprintf("tunnel-one req %d: got %q", idx, body)
			}
		}(i)

		// Requests to Tunnel Two
		go func(idx int) {
			defer wg.Done()
			path := fmt.Sprintf("/b/%d", idx)
			resp := sendHTTPViaTunnel(t, serverAddr, "cm-two", "GET", path, "")
			body := readBody(t, resp)
			if !strings.Contains(body, fmt.Sprintf("server-two: %s", path)) {
				errors <- fmt.Sprintf("tunnel-two req %d: got %q", idx, body)
			}
		}(i)
	}

	wg.Wait()
	close(errors)

	for errMsg := range errors {
		t.Error(errMsg)
	}
}
