package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"lingxing-sync/internal/config"
)

func TestClientRegistrySharesClientByAccount(t *testing.T) {
	registry := NewClientRegistry([]config.Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret"}}, "https://example.test")

	first := registry.Get("sc_us")
	second := registry.Get("sc_us")

	if first == nil || second == nil {
		t.Fatal("configured account did not return a client")
	}
	if first != second {
		t.Fatal("same account received separate clients and token holders")
	}
}

func TestTokenHolderMarksSuccessfulRefreshAsKnown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != tokenEndpoint {
			t.Fatalf("token request path = %s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
	}))
	defer server.Close()

	holder := NewTokenHolder(&config.Account{ID: "sc_us", AppKey: "key", AppSecret: "secret"}, server.URL, server.Client())
	if holder.IsKnown() {
		t.Fatal("unverified token holder reported known")
	}
	if err := holder.ForceRefresh(context.Background()); err != nil {
		t.Fatalf("refresh token: %v", err)
	}
	if !holder.IsKnown() {
		t.Fatal("successful refresh did not mark token holder known")
	}
}
