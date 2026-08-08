package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/server"
	"lingxing-sync/internal/worker"
)

func TestLogsPageUsesSharedUIPrimitives(t *testing.T) {
	assets := server.Assets{
		FS:         webFS,
		TemplateFS: "web/templates",
		StaticFS:   "web/static",
	}
	srv := server.New(&config.Config{}, nil, worker.NewRegistry(), nil, "", assets, nil, nil, nil, "")
	routes := srv.Routes()

	page := httptest.NewRecorder()
	routes.ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/logs", nil))
	if page.Code != http.StatusOK {
		t.Fatalf("GET /logs status=%d, want 200", page.Code)
	}

	body := page.Body.String()
	for _, fragment := range []string{
		`href="/static/ui.css"`,
		`class="ui-app-bar`,
		`class="ui-app-nav`,
		`class="ui-page-header`,
		`class="ui-panel`,
		`class="ui-field`,
		`class="ui-table`,
		`class="ui-pagination`,
		`class="ui-drawer`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("rendered /logs missing shared UI primitive %q", fragment)
		}
	}

	stylesheet := httptest.NewRecorder()
	routes.ServeHTTP(stylesheet, httptest.NewRequest(http.MethodGet, "/static/ui.css", nil))
	if stylesheet.Code != http.StatusOK {
		t.Fatalf("GET /static/ui.css status=%d, want 200", stylesheet.Code)
	}
	if contentType := stylesheet.Header().Get("Content-Type"); !strings.Contains(contentType, "text/css") {
		t.Fatalf("GET /static/ui.css Content-Type=%q, want text/css", contentType)
	}
}

func TestLogsPageShowsRefreshFeedbackAndSingleDateRange(t *testing.T) {
	assets := server.Assets{
		FS:         webFS,
		TemplateFS: "web/templates",
		StaticFS:   "web/static",
	}
	cfg := &config.Config{Accounts: []config.Account{{ID: "sc_us_1", Name: "美国自营"}}}
	srv := server.New(cfg, nil, worker.NewRegistry(), nil, "", assets, nil, nil, nil, "")
	page := httptest.NewRecorder()
	srv.Routes().ServeHTTP(page, httptest.NewRequest(http.MethodGet, "/logs", nil))

	body := page.Body.String()
	for _, fragment := range []string{
		`class="ui-table-loading"`,
		`x-show="refreshing"`,
		`最后更新`,
		`日期范围`,
		`applyDatePreset('last_7_days')`,
		`accountOptions`,
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("rendered /logs missing refresh or date-range feedback %q", fragment)
		}
	}
	if !strings.Contains(body, "美国自营") {
		t.Fatal("rendered /logs did not inject the account display name")
	}
	if strings.Contains(body, `x-text="a.id"`) {
		t.Fatal("account selector must display account name, not account ID")
	}
	for _, fragment := range []string{"起始日期", "结束日期"} {
		if strings.Contains(body, fragment) {
			t.Fatalf("rendered /logs still has separate date field %q", fragment)
		}
	}
}
