package server

import (
	"context"
	"net"
	"net/http"
	"testing"
)

func TestEgressProbeUsesIPv4Transport(t *testing.T) {
	transport, ok := egressHTTPClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("egress transport type = %T, want *http.Transport", egressHTTPClient.Transport)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen IPv4: %v", err)
	}
	defer listener.Close()
	conn, err := transport.DialContext(context.Background(), "tcp6", listener.Addr().String())
	if err != nil {
		t.Fatalf("egress transport did not force IPv4: %v", err)
	}
	conn.Close()
}
