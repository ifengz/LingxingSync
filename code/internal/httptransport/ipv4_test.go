package httptransport

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestNewIPv4TransportForcesTCP4AndClonesDefaults(t *testing.T) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen IPv4: %v", err)
	}
	defer listener.Close()

	transport := NewIPv4Transport()
	defaultTransport := http.DefaultTransport.(*http.Transport)
	if transport == defaultTransport {
		t.Fatal("transport reused the mutable default transport")
	}
	if transport.Proxy == nil || transport.ForceAttemptHTTP2 != defaultTransport.ForceAttemptHTTP2 || transport.TLSHandshakeTimeout != defaultTransport.TLSHandshakeTimeout || transport.IdleConnTimeout != defaultTransport.IdleConnTimeout {
		t.Fatal("transport did not preserve default proxy, TLS, or HTTP/2 settings")
	}

	conn, err := transport.DialContext(context.Background(), "tcp6", listener.Addr().String())
	if err != nil {
		t.Fatalf("DialContext did not force IPv4: %v", err)
	}
	conn.Close()
}

func TestIPv4DialerPreservesStandardDialSettings(t *testing.T) {
	dialer := newIPv4Dialer()
	if dialer.Timeout != 30*time.Second || dialer.KeepAlive != 30*time.Second {
		t.Fatalf("dialer timeout/keepalive = %s/%s, want 30s/30s", dialer.Timeout, dialer.KeepAlive)
	}
}
