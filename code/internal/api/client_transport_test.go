package api

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

func TestNewClientAndNilTokenHolderUseIPv4Transport(t *testing.T) {
	account := &config.Account{ID: "test", AppKey: "key", AppSecret: "secret"}
	client := NewClient(account, "https://example.test")
	if client.http.Timeout != 60*time.Second {
		t.Fatalf("NewClient HTTP client = %#v", client.http)
	}
	assertIPv4Transport(t, client.http.Transport)
	if client.holder.httpClient != client.http {
		t.Fatal("NewClient token holder does not share its HTTP client")
	}

	holder := NewTokenHolder(account, "https://example.test", nil)
	if holder.httpClient.Timeout != 60*time.Second {
		t.Fatalf("nil TokenHolder HTTP client = %#v", holder.httpClient)
	}
	assertIPv4Transport(t, holder.httpClient.Transport)
}

func assertIPv4Transport(t *testing.T, roundTripper http.RoundTripper) {
	t.Helper()
	transport, ok := roundTripper.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T, want *http.Transport", roundTripper)
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen IPv4: %v", err)
	}
	defer listener.Close()
	conn, err := transport.DialContext(context.Background(), "tcp6", listener.Addr().String())
	if err != nil {
		t.Fatalf("transport did not force IPv4: %v", err)
	}
	conn.Close()
}
