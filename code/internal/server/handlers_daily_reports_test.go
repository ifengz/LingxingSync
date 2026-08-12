package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

type fixedDailyPreviewReader struct {
	query dailyPreviewQuery
	page  dailyPreviewPage
	err   error
}

func (r *fixedDailyPreviewReader) Preview(_ context.Context, query dailyPreviewQuery) (dailyPreviewPage, error) {
	r.query = query
	return r.page, r.err
}

type fixedReportStatusReader struct {
	status reportExportStatusOut
	err    error
}

func (r *fixedReportStatusReader) Latest(_ context.Context) (reportExportStatusOut, error) {
	return r.status, r.err
}

func TestDailyPreviewRouteRequiresDatesAndForwardsFixedFilters(t *testing.T) {
	asin, sku := "B001", "SKU-1"
	reader := &fixedDailyPreviewReader{page: dailyPreviewPage{
		Items: []dailyPreviewItem{{BusinessDate: "2026-08-12", Store: "store-a", ASIN: &asin, SKU: &sku}},
		Total: 41,
	}}
	s := &Server{dailyPreview: reader}
	s.assets = Assets{FS: renderTestFS, TemplateFS: "testdata", StaticFS: "testdata"}
	h := s.Routes()

	missing := httptest.NewRecorder()
	h.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/api/datasets/listing-daily-v1/preview", nil))
	if missing.Code != http.StatusBadRequest || !strings.Contains(missing.Body.String(), "日期") {
		t.Fatalf("missing dates status=%d body=%s", missing.Code, missing.Body.String())
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/datasets/listing-daily-v1/preview?date_from=2026-08-01&date_to=2026-08-12&store=store-a&asin=B001&sku=SKU-1&page=2&page_size=20", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":41`) {
		t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String())
	}
	want := dailyPreviewQuery{DateFrom: "2026-08-01", DateTo: "2026-08-12", Store: "store-a", ASIN: "B001", SKU: "SKU-1", Page: 2, PageSize: 20}
	if reader.query != want {
		t.Fatalf("preview query=%+v, want %+v", reader.query, want)
	}
}

func TestDailyPreviewRejectsInvalidPagination(t *testing.T) {
	s := &Server{dailyPreview: &fixedDailyPreviewReader{}}
	for _, rawQuery := range []string{
		"date_from=2026-08-01&date_to=2026-08-02&page=0&page_size=20",
		"date_from=2026-08-01&date_to=2026-08-02&page=1&page_size=201",
	} {
		rec := httptest.NewRecorder()
		s.apiDailyPreview(rec, httptest.NewRequest(http.MethodGet, "/?"+rawQuery, nil))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("query=%s status=%d body=%s", rawQuery, rec.Code, rec.Body.String())
		}
	}
}

func TestReportExportConfigGetReturnsDisabledDefault(t *testing.T) {
	cfg := validReportTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := &Server{cfg: cfg, store: store}
	rec := httptest.NewRecorder()

	s.apiGetReportExportConfig(rec, httptest.NewRequest(http.MethodGet, "/api/report-exports/config", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"type":"fba_customer_returns"`) || !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("default config status=%d body=%s", rec.Code, rec.Body.String())
	}
	if len(store.Current().ReportExports) != 0 {
		t.Fatal("GET must not persist a default report export")
	}
}

func TestReportExportConfigRejectsAmbiguousExistingSchedules(t *testing.T) {
	cfg := validReportTestConfig()
	cfg.ReportExports = []config.ReportExport{
		{Type: config.ReportExportCustomerReturns, Enabled: false},
		{Type: config.ReportExportCustomerReturns, Enabled: false},
	}
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := &Server{cfg: cfg, store: store}
	get := httptest.NewRecorder()
	s.apiGetReportExportConfig(get, httptest.NewRequest(http.MethodGet, "/api/report-exports/config", nil))
	if get.Code != http.StatusConflict {
		t.Fatalf("ambiguous GET status=%d body=%s", get.Code, get.Body.String())
	}
	put := httptest.NewRecorder()
	s.apiPutReportExportConfig(put, httptest.NewRequest(http.MethodPut, "/api/report-exports/config", strings.NewReader(`{"report_exports":[]}`)))
	if put.Code != http.StatusConflict || len(store.Current().ReportExports) != 2 {
		t.Fatalf("ambiguous PUT status=%d reports=%d body=%s", put.Code, len(store.Current().ReportExports), put.Body.String())
	}
}

func TestReportExportConfigPutPersistsOnlyCustomerReturns(t *testing.T) {
	cfg := validReportTestConfig()
	store := config.NewStore(t.TempDir()+"/config.yaml", cfg)
	s := &Server{cfg: cfg, store: store}
	body := `{"type":"fba_customer_returns","enabled":true,"account":"sc_us","seller_id":"SELLER-1","store_id":"STORE-1","region":"na","marketplace_ids":["ATVPDKIKX0DER"],"cron":"0 4 * * *","window_days":3}`
	body = `{"report_exports":[` + body + `]}`
	rec := httptest.NewRecorder()

	s.apiPutReportExportConfig(rec, httptest.NewRequest(http.MethodPut, "/api/report-exports/config", strings.NewReader(body)))

	if rec.Code != http.StatusOK {
		t.Fatalf("put config status=%d body=%s", rec.Code, rec.Body.String())
	}
	reports := store.Current().ReportExports
	if len(reports) != 1 || reports[0].Type != config.ReportExportCustomerReturns || !reports[0].Enabled || reports[0].WindowDays != 3 {
		t.Fatalf("stored report exports=%+v", reports)
	}

	unknown := httptest.NewRecorder()
	s.apiPutReportExportConfig(unknown, httptest.NewRequest(http.MethodPut, "/api/report-exports/config", strings.NewReader(`{"report_exports":[{"type":"sales","enabled":false}]}`)))
	if unknown.Code != http.StatusBadRequest || !strings.Contains(unknown.Body.String(), "fba_customer_returns") {
		t.Fatalf("unknown type status=%d body=%s", unknown.Code, unknown.Body.String())
	}
}

func TestReportExportStatusReturnsLatestTaskAndDifferenceCounts(t *testing.T) {
	finished := time.Date(2026, 8, 13, 4, 0, 0, 0, time.UTC)
	reader := &fixedReportStatusReader{status: reportExportStatusOut{
		LatestTask:  &reportExportTaskOut{ID: 9, Status: "success", Rows: 18, FinishedAt: &finished},
		Differences: reportExportDifferenceOut{DatabaseMissing: 1, ReportMissing: 2, ValueMismatch: 3},
	}}
	s := &Server{cfg: &config.Config{ReportExports: []config.ReportExport{{Type: config.ReportExportCustomerReturns, Enabled: true}}}, reportStatus: reader}
	rec := httptest.NewRecorder()

	s.apiReportExportStatus(rec, httptest.NewRequest(http.MethodGet, "/api/report-exports/status", nil))

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"configured":true`) || !strings.Contains(rec.Body.String(), `"database_missing":1`) || !strings.Contains(rec.Body.String(), `"value_mismatch":3`) {
		t.Fatalf("report status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func TestReportExportStatusFailsLoudWhenAuditQueryFails(t *testing.T) {
	s := &Server{cfg: validReportTestConfig(), reportStatus: &fixedReportStatusReader{err: errors.New("table listing_daily_reconciliations doesn't exist")}}
	rec := httptest.NewRecorder()

	s.apiReportExportStatus(rec, httptest.NewRequest(http.MethodGet, "/api/report-exports/status", nil))

	if rec.Code != http.StatusInternalServerError || !strings.Contains(rec.Body.String(), "listing_daily_reconciliations") {
		t.Fatalf("report query failure status=%d body=%s", rec.Code, rec.Body.String())
	}
}

func validReportTestConfig() *config.Config {
	return &config.Config{
		Server:    config.Server{Port: 7799},
		Database:  config.Database{Host: "127.0.0.1", Port: 3306, User: "test", DB: "lingsync", MaxOpen: 20, MaxIdle: 5, ConnTimeoutSec: 10},
		Accounts:  []config.Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret", ConnectionCheck: config.DefaultConnectionCheck()}},
		Retention: config.Retention{TaskLogsDays: 90, TasksDays: 365, CleanupCron: "0 3 * * *"},
	}
}
