package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lingxing-sync/internal/config"
)

func TestFetchWithShapeAndHeadersSendsEndpointProtocolHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		if got := r.Header.Get("X-API-VERSION"); got != "2" {
			t.Fatalf("X-API-VERSION=%q, want 2", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type=%q, want application/json", got)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":[]}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	if _, _, _, err := client.FetchWithShapeAndHeaders(context.Background(), http.MethodPost, "/sp", nil, "list", map[string]string{"X-API-VERSION": "2"}); err != nil {
		t.Fatalf("FetchWithShapeAndHeaders returned error: %v", err)
	}
}
