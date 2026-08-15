// Package httptransport provides shared outbound HTTP transport settings.
package httptransport

import (
	"context"
	"net"
	"net/http"
	"time"
)

// NewIPv4Transport clones the standard transport and forces outbound dials to IPv4.
func NewIPv4Transport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := newIPv4Dialer()
	transport.DialContext = func(ctx context.Context, _ string, address string) (net.Conn, error) {
		return dialer.DialContext(ctx, "tcp4", address)
	}
	return transport
}

func newIPv4Dialer() *net.Dialer {
	return &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
}

// NewIPv4Client returns an HTTP client with the shared IPv4-only transport.
func NewIPv4Client(timeout time.Duration) *http.Client {
	return &http.Client{Transport: NewIPv4Transport(), Timeout: timeout}
}
