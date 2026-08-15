package server

import (
	"fmt"
	"strings"
	"testing"
)

func TestDailyPreviewSQLIsFixedAndParameterised(t *testing.T) {
	// SQL shape is already fixed in production constants; this focused check guards
	// against interpolating user filters or changing the two-table ownership boundary.
	where, args := dailyPreviewWhere(dailyPreviewQuery{DateFrom: "2026-08-01", DateTo: "2026-08-12", Store: "store-a", ASIN: "B001", SKU: "SKU-1"})
	if where != "WHERE m.business_date BETWEEN ? AND ? AND d.store_id = ? AND d.asin = ? AND d.sku = ?" {
		t.Fatalf("preview where=%q", where)
	}
	if got := fmt.Sprint(args); got != "[2026-08-01 2026-08-12 store-a B001 SKU-1]" {
		t.Fatalf("preview args=%v", args)
	}
	if dailyPreviewFromSQL != " FROM listing_daily_metrics m JOIN listing_dimensions d ON d.id = m.listing_dimension_id " {
		t.Fatalf("preview tables changed: %q", dailyPreviewFromSQL)
	}
}

func TestReportTaskUIStatusDoesNotTreatDoneAsImported(t *testing.T) {
	for raw, want := range map[string]string{
		"PENDING": "pending", "CREATING": "pending", "IN_QUEUE": "pending",
		"IN_PROGRESS": "running", "DONE": "running", "UNKNOWN": "running", "SUCCESS": "success",
		"ERROR": "error", "FATAL": "error", "CANCELLED": "error",
	} {
		if got := reportTaskUIStatus(raw); got != want {
			t.Fatalf("reportTaskUIStatus(%q)=%q, want %q", raw, got, want)
		}
	}
}

func TestReportStatusSQLUsesAuditIDAndProbesReconciliationTable(t *testing.T) {
	if !strings.Contains(reportDifferencesSQL, "FROM listing_daily_reconciliations WHERE report_audit_id = ?") {
		t.Fatalf("report differences SQL must bind audit id: %s", reportDifferencesSQL)
	}
	if strings.Contains(latestReportTaskSQL, "download_url") {
		t.Fatalf("report status must not expose temporary download URL: %s", latestReportTaskSQL)
	}
	for _, want := range []string{"report_task_id", "report_document_id", "download_sha256"} {
		if !strings.Contains(latestReportTaskSQL, want) {
			t.Fatalf("report status SQL must retain %s evidence: %s", want, latestReportTaskSQL)
		}
	}
	for _, want := range []string{"account_id = ?", "store_id = ?"} {
		if !strings.Contains(latestReportTaskSQL, want) {
			t.Fatalf("report status SQL must scope %s: %s", want, latestReportTaskSQL)
		}
	}
}
