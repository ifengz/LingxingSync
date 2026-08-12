package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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
