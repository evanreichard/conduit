package tunnel

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"

	"reichard.io/conduit/store"
)

func newHTTPForwarder(targetURL *url.URL, tunnelStore store.TunnelStore) (Forwarder, error) {
	return &httpConnBuilder{
		multiConnListener: newMultiConnListener(),
		tunnelStore:       tunnelStore,
		targetURL:         targetURL,
	}, nil
}

type httpConnBuilder struct {
	multiConnListener *multiConnListener
	tunnelStore       store.TunnelStore
	targetURL         *url.URL
}

func (c *httpConnBuilder) Type() ForwarderType {
	return ForwarderHTTP
}

func (c *httpConnBuilder) Start(ctx context.Context) error {
	// Create Reverse Proxy Server
	server := &http.Server{
		Handler: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.Host = c.targetURL.Host
				req.URL.Host = c.targetURL.Host
				req.URL.Scheme = c.targetURL.Scheme
				c.tunnelStore.RecordRequest(req)
			},
			ModifyResponse: c.tunnelStore.RecordResponse,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
			},
		},
	}

	// Context & Cleanup
	go func() {
		<-ctx.Done()
		server.Shutdown(ctx)
		c.multiConnListener.Close()
	}()

	// Start HTTP Proxy
	if err := server.Serve(c.multiConnListener); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (c *httpConnBuilder) Initialize() (Stream, error) {
	clientConn, serverConn := net.Pipe()

	if err := c.multiConnListener.addConn(serverConn); err != nil {
		_ = clientConn.Close()
		_ = serverConn.Close()
		return nil, err
	}

	return &streamImpl{clientConn, c.targetURL.String()}, nil
}

type multiConnListener struct {
	connCh chan net.Conn
	closed chan struct{}
	once   sync.Once
}

func newMultiConnListener() *multiConnListener {
	return &multiConnListener{
		connCh: make(chan net.Conn, 100),
		closed: make(chan struct{}),
	}
}

func (l *multiConnListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.connCh:
		if conn == nil {
			return nil, fmt.Errorf("listener closed")
		}
		return conn, nil
	case <-l.closed:
		return nil, fmt.Errorf("listener closed")
	}
}

func (l *multiConnListener) Close() error {
	l.once.Do(func() {
		close(l.closed)
		// Drain any remaining connections
		go func() {
			for conn := range l.connCh {
				if conn != nil {
					conn.Close()
				}
			}
		}()
		close(l.connCh)
	})
	return nil
}

func (l *multiConnListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (l *multiConnListener) addConn(conn net.Conn) error {
	select {
	case l.connCh <- conn:
		return nil
	case <-l.closed:
		conn.Close()
		return fmt.Errorf("listener is closed")
	default:
		conn.Close()
		return fmt.Errorf("connection queue full")
	}
}
