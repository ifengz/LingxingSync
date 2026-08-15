package reportexport

import (
	"context"
	"net"
	"net/http"
	"testing"
)

func TestDefaultDownloadClientUsesIPv4Transport(t *testing.T) {
	client := newDownloadClient()
	if client.Timeout != defaultDownloadTimeout {
		t.Fatalf("default download client = %#v", client)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("download transport type = %T, want *http.Transport", client.Transport)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen IPv4: %v", err)
	}
	defer listener.Close()
	conn, err := transport.DialContext(context.Background(), "tcp6", listener.Addr().String())
	if err != nil {
		t.Fatalf("download transport did not force IPv4: %v", err)
	}
	conn.Close()
}
