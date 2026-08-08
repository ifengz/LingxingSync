package server

import (
	"embed"
	"html/template"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
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
	s.renderPage(recorder, "sync_manage", pageData{Active: "sync_manage"})
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

func TestValidateSyncDateRange(t *testing.T) {
	if err := validateSyncDateRange("2026-08-01", "2026-08-03"); err != nil {
		t.Fatalf("valid date range rejected: %v", err)
	}
	for _, tc := range []struct{ from, to string }{
		{"2026-08-03", "2026-08-01"},
		{"2026/08/01", "2026-08-03"},
		{"", "2026-08-03"},
	} {
		if err := validateSyncDateRange(tc.from, tc.to); err == nil {
			t.Fatalf("invalid date range accepted: %#v", tc)
		}
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

func TestAccountStoreSyncRejectsAccountWithoutStoreSource(t *testing.T) {
	s := &Server{
		cfg: &config.Config{Accounts: []config.Account{{ID: "sc_us"}}},
		reg: worker.NewRegistry(),
	}
	req := httptest.NewRequest(http.MethodPost, "/api/accounts/sc_us/stores/sync", nil)
	req.SetPathValue("id", "sc_us")
	rec := httptest.NewRecorder()

	s.apiAccountStoreSync(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("missing source status=%d, want %d; body=%s", rec.Code, http.StatusConflict, rec.Body.String())
	}
}

func TestFindVCStoreForProfileRequiresMatchingVCStore(t *testing.T) {
	stores := []db.StoreSummary{
		{SID: "sc-1", StoreType: "SC"},
		{SID: "vc-1", StoreType: "VC"},
	}

	store, status, err := findVCStoreForProfile(stores, "vc-1")
	if err != nil || status != http.StatusOK || store.SID != "vc-1" {
		t.Fatalf("VC store result = %#v, status=%d, err=%v", store, status, err)
	}
	if _, status, err = findVCStoreForProfile(stores, "sc-1"); err == nil || status != http.StatusBadRequest {
		t.Fatalf("SC store status=%d, err=%v; want 400", status, err)
	}
	if _, status, err = findVCStoreForProfile(stores, "missing"); err == nil || status != http.StatusNotFound {
		t.Fatalf("unknown store status=%d, err=%v; want 404", status, err)
	}
}

func TestSaveVCStoreProfileRejectsMissingProfileField(t *testing.T) {
	s := &Server{cfg: &config.Config{Accounts: []config.Account{{ID: "vc_account"}}}}
	req := httptest.NewRequest(http.MethodPut, "/api/accounts/vc_account/stores/vc-1/vc-profile", strings.NewReader(`{}`))
	req.SetPathValue("id", "vc_account")
	req.SetPathValue("sid", "vc-1")
	rec := httptest.NewRecorder()

	s.apiSaveVCStoreProfile(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("missing profile_id status=%d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}
