package server

import (
	"embed"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/worker"
)

//go:embed testdata/*.html
var renderTestFS embed.FS

func TestRenderPageWritesLayout(t *testing.T) {
	s := &Server{
		cfg:    &config.Config{},
		assets: Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"},
		pages:  map[string]*template.Template{},
	}
	if err := s.parseTemplates(); err != nil {
		t.Fatalf("parse templates: %v", err)
	}

	recorder := httptest.NewRecorder()
	s.renderPage(recorder, "sync_center", pageData{Active: "sync_center"})
	if !strings.Contains(recorder.Body.String(), "<html>") {
		t.Fatalf("rendered page missing layout: %q", recorder.Body.String())
	}
}

func TestManualSyncRejectsDisabledEndpoint(t *testing.T) {
	registry := worker.NewRegistry()
	registry.Register(&worker.EndpointWorker{Endpoint: config.Endpoint{Name: "inventory", Enabled: false}})
	s := &Server{reg: registry}
	req := httptest.NewRequest(http.MethodPost, "/api/sync/inventory", nil)
	req.SetPathValue("name", "inventory")
	rec := httptest.NewRecorder()

	s.apiSyncTrigger(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("disabled endpoint status=%d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "已禁用") {
		t.Fatalf("disabled endpoint response=%q, want disabled explanation", rec.Body.String())
	}
}

func TestParseEgressIPRejectsNonIPResponse(t *testing.T) {
	if _, err := parseEgressIP("not an ip"); err == nil {
		t.Fatal("non-IP egress response was accepted")
	}
	got, err := parseEgressIP(" 203.0.113.9 ")
	if err != nil || got != "203.0.113.9" {
		t.Fatalf("parseEgressIP valid response = %q, %v", got, err)
	}
}

func TestAccountStoresRejectsUnknownAccount(t *testing.T) {
	s := &Server{cfg: &config.Config{Accounts: []config.Account{{ID: "sc_us"}}}}
	req := httptest.NewRequest(http.MethodGet, "/api/accounts/missing/stores", nil)
	req.SetPathValue("id", "missing")
	rec := httptest.NewRecorder()

	s.apiAccountStores(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown account stores status=%d, want %d; body=%s", rec.Code, http.StatusNotFound, rec.Body.String())
	}
}
