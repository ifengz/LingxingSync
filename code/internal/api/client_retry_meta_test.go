package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

func TestFetchBusinessErrorCarriesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		w.Header().Set("Retry-After", "15")
		_, _ = w.Write([]byte(`{"code":3001008,"message":"new requests too frequently"}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, status, code, err := client.Fetch(context.Background(), http.MethodGet, "/test", nil)
	if err == nil {
		t.Fatal("Fetch should return the business error")
	}
	if status != http.StatusOK || code != 3001008 {
		t.Fatalf("status/code = %d/%d, want 200/3001008", status, code)
	}
	var fetchErr *FetchError
	if !asFetchError(err, &fetchErr) {
		t.Fatalf("error type = %T, want *FetchError", err)
	}
	if fetchErr.RetryAfter != 15*time.Second {
		t.Fatalf("RetryAfter = %s, want 15s", fetchErr.RetryAfter)
	}
}

func TestParseRetryAfterSupportsHTTPDate(t *testing.T) {
	now := time.Date(2026, 8, 9, 1, 0, 0, 0, time.UTC)
	delay, ok := parseRetryAfter("Sun, 09 Aug 2026 01:00:12 GMT", now)
	if !ok || delay != 12*time.Second {
		t.Fatalf("parseRetryAfter = %s, %v, want 12s, true", delay, ok)
	}
}

func TestTokenExpiryCodesTriggerRefreshClassification(t *testing.T) {
	for _, code := range []int{2001003, 2001005} {
		if !isTokenExpiredCode(code, "") {
			t.Fatalf("code %d should be classified as token expiry", code)
		}
	}
}

func asFetchError(err error, target **FetchError) bool {
	for err != nil {
		if candidate, ok := err.(*FetchError); ok {
			*target = candidate
			return true
		}
		err = unwrapError(err)
	}
	return false
}

func unwrapError(err error) error {
	type unwrapper interface{ Unwrap() error }
	if u, ok := err.(unwrapper); ok {
		return u.Unwrap()
	}
	return nil
}

func TestFetchErrorJSONShapeRemainsParsable(t *testing.T) {
	value, err := json.Marshal(&FetchError{HTTPStatus: 200, APICode: 3001008, APIMessage: "rate"})
	if err != nil || len(value) == 0 {
		t.Fatalf("FetchError should remain a normal error value: %v", err)
	}
}
