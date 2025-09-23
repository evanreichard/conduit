package tunnel

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sync"
)

func HTTPConnectionBuilder(targetURL *url.URL) (ConnBuilder, error) {
	multiConnListener := newMultiConnListener()

	// Create Reverse Proxy
	proxy := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.Host = targetURL.Host
			req.URL.Host = targetURL.Host
			req.URL.Scheme = targetURL.Scheme
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, fmt.Sprintf("Proxy error: %v", err), http.StatusBadGateway)
		},
	}

	// Start HTTP Proxy
	go func() {
		defer multiConnListener.Close()
		_ = http.Serve(multiConnListener, proxy)
	}()

	// Return Connection Builder
	return func() (conn io.ReadWriteCloser, err error) {
		clientConn, serverConn := net.Pipe()

		if err := multiConnListener.addConn(serverConn); err != nil {
			_ = clientConn.Close()
			_ = serverConn.Close()
			return nil, err
		}

		return clientConn, nil
	}, nil
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
