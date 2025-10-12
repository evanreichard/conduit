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
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			if wsConn, ok := c.(*wsConn); ok {
				return context.WithValue(ctx, "sourceAddr", wsConn.sourceAddress)
			}
			return ctx
		},
		Handler: &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				// Rewrite Request URL
				req.Host = c.targetURL.Host
				req.URL.Host = c.targetURL.Host
				req.URL.Scheme = c.targetURL.Scheme

				// Rewrite Referer
				if referer := req.Header.Get("Referer"); referer != "" {
					if refURL, err := url.Parse(referer); err == nil {
						refURL.Host = c.targetURL.Host
						refURL.Scheme = c.targetURL.Scheme
						req.Header.Set("Referer", refURL.String())
					}
				}

				// Extract Source Address & Record Request
				sourceAddress, _ := req.Context().Value("sourceAddr").(string)
				c.tunnelStore.RecordRequest(req, sourceAddress)
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

func (c *httpConnBuilder) Initialize(sourceAddress string) (Stream, error) {
	clientConn, serverConn := net.Pipe()

	if err := c.multiConnListener.addConn(&wsConn{serverConn, sourceAddress}); err != nil {
		_ = clientConn.Close()
		_ = serverConn.Close()
		return nil, err
	}

	return &streamImpl{clientConn, sourceAddress, c.targetURL.String()}, nil
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

type wsConn struct {
	net.Conn
	sourceAddress string
}
