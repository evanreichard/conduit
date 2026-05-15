package tunnel

import (
	"testing"

	"reichard.io/conduit/store"
)

func TestNewForwarderRequiresExplicitScheme(t *testing.T) {
	_, err := NewForwarder("localhost:8282", store.NewTunnelStore(1))
	if err == nil {
		t.Fatal("expected error for target without scheme")
	}
}

func TestNewForwarderSupportsExplicitSchemes(t *testing.T) {
	tests := []struct {
		name          string
		target        string
		forwarderType ForwarderType
	}{
		{name: "http", target: "http://localhost:8282", forwarderType: ForwarderHTTP},
		{name: "https", target: "https://localhost:8282", forwarderType: ForwarderHTTP},
		{name: "tcp", target: "tcp://localhost:8282", forwarderType: ForwarderTCP},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			forwarder, err := NewForwarder(tt.target, store.NewTunnelStore(1))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if forwarder.Type() != tt.forwarderType {
				t.Fatalf("expected forwarder type %v, got %v", tt.forwarderType, forwarder.Type())
			}
		})
	}
}

func TestNewForwarderRejectsUnsupportedScheme(t *testing.T) {
	_, err := NewForwarder("udp://localhost:8282", store.NewTunnelStore(1))
	if err == nil {
		t.Fatal("expected error for unsupported scheme")
	}
}
