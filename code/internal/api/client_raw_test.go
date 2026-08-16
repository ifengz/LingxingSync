package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
)

func TestDoSignedJSONReturnsRawResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		if r.URL.Path != "/basicOpen/report/create/reportExportTask" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		payload, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		var body map[string]any
		if err := json.Unmarshal(payload, &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		params := make(map[string]string, len(body)+3)
		for key, value := range body {
			params[key] = anyToString(value)
		}
		params["app_key"] = "1234567890abcdef"
		params["access_token"] = "token"
		params["timestamp"] = r.URL.Query().Get("timestamp")
		if got, want := r.URL.Query().Get("sign"), Sign(params, params["app_key"], "secret"); got != want {
			t.Fatalf("sign mismatch: got %q want %q body=%s", got, want, payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"task_id":"task-1"}}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	raw, status, code, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/basicOpen/report/create/reportExportTask", map[string]any{"report_type": "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA", "marketplace_ids": []string{"ATVPDHSKDCJ6R"}})
	if err != nil {
		t.Fatalf("DoSignedJSON returned error: %v", err)
	}
	if status != http.StatusOK || code != 0 {
		t.Fatalf("status/code = %d/%d", status, code)
	}
	if string(raw) == "" {
		t.Fatal("raw response is empty")
	}
}

func TestDoSignedJSONRejectsNilClient(t *testing.T) {
	var client *Client
	if _, _, _, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/report", nil); err == nil {
		t.Fatal("expected unconfigured client error")
	}
}

func TestDoSignedJSONBusinessErrorPreservesBoundedDiagnosticsWithoutSecrets(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":429,"message":"request rejected app_secret=do-not-leak","request_id":"trace-429","error_details":{"reason":"too many requests","app_secret":"do-not-leak"},"data":null}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, status, code, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/report", nil)
	if err == nil {
		t.Fatal("DoSignedJSON should return the business error")
	}
	if status != http.StatusOK || code != http.StatusTooManyRequests {
		t.Fatalf("status/code = %d/%d, want 200/429", status, code)
	}
	message := err.Error()
	if !strings.Contains(message, "request_id=trace-429") || !strings.Contains(message, "error_details=") || !strings.Contains(message, "too many requests") {
		t.Fatalf("error=%q, want bounded request diagnostics", message)
	}
	if strings.Contains(message, "do-not-leak") || strings.Contains(message, `"data":null`) {
		t.Fatalf("error=%q, must not expose secret or raw envelope", message)
	}
	if len(message) > 1024 {
		t.Fatalf("error length=%d, want bounded diagnostics", len(message))
	}
}

func TestDoSignedJSONBusinessErrorBoundsDiagnostics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":429,"message":"request rejected","request_id":"` + strings.Repeat("r", 400) + `","error_details":["` + strings.Repeat("d", 400) + `"]}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, _, _, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/report", nil)
	if err == nil {
		t.Fatal("DoSignedJSON should return the business error")
	}
	if !strings.Contains(err.Error(), "...(truncated)") {
		t.Fatalf("error=%q, want truncation marker", err)
	}
	if len(err.Error()) > 800 {
		t.Fatalf("error length=%d, want bounded diagnostics", len(err.Error()))
	}
}

func TestDoSignedJSONHTTPErrorDoesNotExposeRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"upstream app_secret=do-not-leak","data":{"token":"do-not-leak"}}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, _, _, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/report", nil)
	if err == nil {
		t.Fatal("DoSignedJSON should return the HTTP error")
	}
	if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), `"data"`) {
		t.Fatalf("error=%q, must not expose secret or raw body", err)
	}
}

func TestFetchHTTPErrorDoesNotExposeRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"upstream app_secret=do-not-leak","data":{"token":"do-not-leak"}}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, _, _, err := client.Fetch(context.Background(), http.MethodGet, "/fetch", nil)
	if err == nil {
		t.Fatal("Fetch should return the HTTP error")
	}
	if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), `"data"`) {
		t.Fatalf("error=%q, must not expose secret or raw body", err)
	}
}

func TestFetchBusinessErrorDoesNotExposeSecretMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":429,"message":"upstream app_secret=do-not-leak","request_id":"trace-fetch","error_details":{"token":"do-not-leak"}}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, _, _, err := client.Fetch(context.Background(), http.MethodGet, "/fetch", nil)
	if err == nil {
		t.Fatal("Fetch should return the business error")
	}
	if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), `"error_details"`) {
		t.Fatalf("error=%q, must not expose secret or raw diagnostics", err)
	}
}

func TestTruncateForLogRedactsSensitiveResponse(t *testing.T) {
	for _, raw := range []string{
		`{"access_token":"do-not-leak","message":"ok"}`,
		`{"token":"do-not-leak"}`,
		`{"cookie":"do-not-leak"}`,
		`{"sign":"do-not-leak"}`,
		`{"authorization":"do-not-leak"}`,
	} {
		if got := truncateForLog([]byte(raw)); strings.Contains(got, "do-not-leak") {
			t.Fatalf("diagnostic=%q, must redact sensitive response fields", got)
		}
	}
}

func TestSanitizeDiagnosticTextRedactsCredentialAssignments(t *testing.T) {
	for _, value := range []string{
		"app_key=do-not-leak",
		"https://example.test/?access_token=do-not-leak",
		"Authorization: Bearer do-not-leak",
	} {
		if got := sanitizeDiagnosticText(value); strings.Contains(got, "do-not-leak") {
			t.Fatalf("diagnostic=%q, must redact credential assignment", got)
		}
	}
}

func TestDoSignedJSONBusinessErrorRedactsMessageAndDiagnosticURLs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":429,"message":"app_secret=do-not-leak","request_id":"trace-429","error_details":{"url":"https://example.test/?access_token=do-not-leak","reason":"too many requests"}}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, _, _, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/report", nil)
	if err == nil {
		t.Fatal("DoSignedJSON should return the business error")
	}
	if strings.Contains(err.Error(), "do-not-leak") {
		t.Fatalf("error=%q, must redact credentials from message and URLs", err)
	}
}

func TestDoSignedJSONHTTPErrorDoesNotIncludeRawBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == tokenEndpoint {
			_, _ = w.Write([]byte(`{"code":"200","data":{"access_token":"token","expires_in":7200}}`))
			return
		}
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"app_secret":"do-not-leak"}`))
	}))
	defer server.Close()

	client := NewClient(&config.Account{ID: "test", AppKey: "1234567890abcdef", AppSecret: "secret"}, server.URL)
	_, _, _, err := client.DoSignedJSON(context.Background(), http.MethodPost, "/report", nil)
	if err == nil {
		t.Fatal("DoSignedJSON should return the HTTP error")
	}
	if strings.Contains(err.Error(), "do-not-leak") || strings.Contains(err.Error(), "body=") {
		t.Fatalf("error=%q, must not include raw HTTP body", err)
	}
}
