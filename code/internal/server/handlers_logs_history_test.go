package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fixedReportHistoryReader struct {
	reportQuery         reportHistoryQuery
	reconciliationQuery reconciliationHistoryQuery
	reportPage          reportHistoryPage
	reconciliationPage  reconciliationHistoryPage
}

func (r *fixedReportHistoryReader) History(_ context.Context, query reportHistoryQuery) (reportHistoryPage, error) {
	r.reportQuery = query
	return r.reportPage, nil
}

func (r *fixedReportHistoryReader) Reconciliations(_ context.Context, query reconciliationHistoryQuery) (reconciliationHistoryPage, error) {
	r.reconciliationQuery = query
	return r.reconciliationPage, nil
}

func TestReportHistoryEndpointForwardsFiltersAndPagination(t *testing.T) {
	reader := &fixedReportHistoryReader{reportPage: reportHistoryPage{Items: []reportHistoryItem{{ID: 7, ReportType: "GET_FBA_FULFILLMENT_CUSTOMER_RETURNS_DATA", Status: "SUCCESS", RowsImported: 18}}, Total: 1}}
	s := &Server{reportHistory: reader}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/report-exports/history?account=sc_us&store_id=store-1&type=returns&status=SUCCESS&date_from=2026-08-01&date_to=2026-08-25&page=2&page_size=10", nil)

	s.apiReportHistory(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"report_audit_id":7`) || !strings.Contains(rec.Body.String(), `"rows_imported":18`) || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("history response status=%d body=%s", rec.Code, rec.Body.String())
	}
	if reader.reportQuery.Account != "sc_us" || reader.reportQuery.StoreID != "store-1" || reader.reportQuery.ReportType != "returns" || reader.reportQuery.Status != "SUCCESS" || reader.reportQuery.DateFrom != "2026-08-01" || reader.reportQuery.DateTo != "2026-08-25" || reader.reportQuery.Page != 2 || reader.reportQuery.PageSize != 10 {
		t.Fatalf("history query=%+v", reader.reportQuery)
	}
}

func TestReportReconciliationEndpointForwardsAuditFiltersAndPagination(t *testing.T) {
	reader := &fixedReportHistoryReader{reconciliationPage: reconciliationHistoryPage{Items: []reconciliationHistoryItem{{ReportAuditID: 7, ReportTaskID: "task-7", BusinessDate: "2026-08-24", Status: "corrected", DatabaseMissing: 2}}, Total: 1}}
	s := &Server{reportHistory: reader}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/report-reconciliations?audit_id=7&account=sc_us&store_id=store-1&type=returns&status=corrected&business_date=2026-08-24&page=3&page_size=5", nil)

	s.apiReportReconciliations(rec, req)

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"report_audit_id":7`) || !strings.Contains(rec.Body.String(), `"database_missing":2`) || !strings.Contains(rec.Body.String(), `"total":1`) {
		t.Fatalf("reconciliation response status=%d body=%s", rec.Code, rec.Body.String())
	}
	if reader.reconciliationQuery.AuditID != 7 || reader.reconciliationQuery.Account != "sc_us" || reader.reconciliationQuery.StoreID != "store-1" || reader.reconciliationQuery.ReportType != "returns" || reader.reconciliationQuery.Status != "corrected" || reader.reconciliationQuery.BusinessDate != "2026-08-24" || reader.reconciliationQuery.Page != 3 || reader.reconciliationQuery.PageSize != 5 {
		t.Fatalf("reconciliation query=%+v", reader.reconciliationQuery)
	}
}

func TestReportHistoryItemsExposeDownloadEvidenceWithoutURL(t *testing.T) {
	finished := time.Date(2026, 8, 25, 10, 0, 0, 0, time.UTC)
	doc := "doc-7"
	sha := "sha-7"
	when := rfc3339(finished)
	item := reportHistoryItem{ID: 7, ReportTaskID: "task-7", ReportDocumentID: &doc, DownloadSHA256: &sha, DownloadedAt: &when}
	encoded, err := json.Marshal(item)
	if err != nil || strings.Contains(string(encoded), "download_url") {
		t.Fatalf("report history must not expose short-lived download URL: %s", encoded)
	}
}
