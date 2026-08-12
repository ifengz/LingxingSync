package listingdaily

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"
)

func TestInventoryProjectionContractsSeparateVCDailyAndSCSnapshotSources(t *testing.T) {
	for _, want := range []string{"FROM ls_vc_inventory", "`date` = ?", "sellableOnHandInventoryUnits", "unhealthyInventoryUnits", "aged90PlusDaysSellableInventoryUnits"} {
		if !strings.Contains(vcInventorySQL, want) {
			t.Fatalf("VC inventory query missing %q: %s", want, vcInventorySQL)
		}
	}
	for _, want := range []string{"FROM ls_fba_inventory", "DATE(synced_at) = ?", "afn_fulfillable_quantity", "afn_inbound_receiving_quantity", "reserved_customerorders", "reserved_fc_transfers", "afn_unsellable_quantity"} {
		if !strings.Contains(scInventorySQL, want) {
			t.Fatalf("SC inventory query missing %q: %s", want, scInventorySQL)
		}
	}
}

func TestSCPerformanceDailyContractMapsTrafficAndReviews(t *testing.T) {
	for _, want := range []string{"FROM ls_sc_performance_daily", "business_date = ?", "sessions", "sessions_mobile", "sessions_total", "reviews_count", "avg_star"} {
		if !strings.Contains(scPerformanceDailySQL, want) {
			t.Fatalf("SC performance daily query missing %q: %s", want, scPerformanceDailySQL)
		}
	}
	values := scPerformanceValues(
		sql.NullInt64{Int64: 10, Valid: true}, sql.NullInt64{Int64: 4, Valid: true}, sql.NullInt64{Int64: 14, Valid: true},
		sql.NullInt64{Int64: 8, Valid: true}, sql.NullFloat64{Float64: 4.5, Valid: true},
	)
	if *values.SessionsDesktop != 10 || *values.SessionsMobile != 4 || *values.SessionsTotal != 14 || *values.ReviewCount != 8 || *values.Rating != 4.5 {
		t.Fatalf("SC performance mapping = %#v", values)
	}
}

func TestSCInventoryKeepsComponentsWithoutInventingTotals(t *testing.T) {
	values := scInventoryValues(
		sql.NullInt64{Int64: 10, Valid: true},
		sql.NullInt64{Int64: 1, Valid: true}, sql.NullInt64{Int64: 2, Valid: true}, sql.NullInt64{Int64: 3, Valid: true},
		sql.NullInt64{Int64: 4, Valid: true}, sql.NullInt64{Int64: 5, Valid: true}, sql.NullInt64{Int64: 6, Valid: true},
		sql.NullInt64{Int64: 7, Valid: true},
	)
	if values.InventoryInbound != nil || values.InventoryReserved != nil {
		t.Fatalf("SC component values must not be written as invented totals: %#v", values)
	}
	if values.InventoryInboundReceiving == nil || *values.InventoryInboundReceiving != 1 || values.InventoryReservedFCTransfers == nil || *values.InventoryReservedFCTransfers != 6 {
		t.Fatalf("SC inventory components were not retained: %#v", values)
	}
}

func TestVCInventoryIdentityDoesNotUseInventoryMSKU(t *testing.T) {
	if strings.Contains(vcInventorySQL, "SELECT asin, msku") {
		t.Fatalf("VC inventory must share ls_vc_listing identity, query=%s", vcInventorySQL)
	}
}

func TestReportReturnsQueryUsesExactSuccessfulCoveringTask(t *testing.T) {
	for _, want := range []string{"task.id = ?", "task.report_task_id = ?", "task.date_from", "task.date_to", "task.status = 'SUCCESS'"} {
		if !strings.Contains(reportReturnsSQL, want) {
			t.Fatalf("formal report query missing exact-task guard %q: %s", want, reportReturnsSQL)
		}
	}
	if strings.Contains(reportReturnsSQL, "MAX(latest.id)") {
		t.Fatalf("formal report query must not choose a different task by store: %s", reportReturnsSQL)
	}
}

func TestVCListingSKUUsesOnlyUniqueSameDomainValues(t *testing.T) {
	if got, err := uniqueVCListingSKU(sql.NullString{String: "MSKU-1", Valid: true}, sql.NullString{}); err != nil || got != "MSKU-1" {
		t.Fatalf("unique VC listing msku = %q, %v", got, err)
	}
	if got, err := uniqueVCListingSKU(sql.NullString{}, sql.NullString{String: "LOCAL-1", Valid: true}); err != nil || got != "LOCAL-1" {
		t.Fatalf("unique VC listing local_sku = %q, %v", got, err)
	}
	if _, err := uniqueVCListingSKU(sql.NullString{String: "MSKU-1", Valid: true}, sql.NullString{String: "MSKU-2", Valid: true}); err == nil {
		t.Fatal("ambiguous VC listing values were accepted")
	}
}

func TestVCSalesContractUsesShippedMetricsAndSameDomainListing(t *testing.T) {
	for _, want := range []string{"FROM ls_vc_sales_report", "shippedUnits", "shippedRevenueAmount", "customerReturns", "`date` = ?"} {
		if !strings.Contains(vcSalesSQL, want) {
			t.Fatalf("VC sales query missing %q: %s", want, vcSalesSQL)
		}
	}
}

func TestProjectAndPublishFromSQLPublishesProjectedRowsAndKeepsUnknownCoverage(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	units := int64(3)
	reader := sourceReaderFunc(func(_ context.Context, accountID, storeID, channel string, gotDate time.Time) (SQLProjection, error) {
		if accountID != "account-1" || storeID != "store-1" || channel != "sc_fba" || !gotDate.Equal(date) {
			t.Fatalf("source scope = %q %q %q %s", accountID, storeID, channel, gotDate)
		}
		return SQLProjection{
			Records: []RawRecord{{Source: SourceAPI, Input: Input{Key: Key{Store: storeID, Channel: channel, ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: &units}}}},
			Unknown: []CoverageUnknown{{Source: "ls_sc_sales_report", Store: storeID, ASIN: "B02", Date: date, Reason: "missing listing SKU"}},
		}, nil
	})
	store := &fakeStore{}
	projection, err := ProjectAndPublishFromSQL(context.Background(), reader, store, "account-1", "store-1", "sc_fba", date, date, ReportAbsent)
	if err != nil {
		t.Fatal(err)
	}
	if len(store.rows) != 1 || store.rows[0].Values.SalesUnits == nil || *store.rows[0].Values.SalesUnits != 3 {
		t.Fatalf("published rows = %#v", store.rows)
	}
	if len(projection.Unknown) != 1 || projection.Unknown[0].ASIN != "B02" {
		t.Fatalf("coverage unknown = %#v", projection.Unknown)
	}
}

func TestProjectAndPublishFromSQLPublishesReportCorrectionAndReturnsFieldDiff(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	apiReturns, reportReturns := int64(1), int64(2)
	reader := &reconciledSourceReader{
		api:    SQLProjection{Records: []RawRecord{{Source: SourceAPI, Input: Input{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &apiReturns}}}}},
		report: []RawRecord{{Source: SourceReport, Input: Input{Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &reportReturns}}}},
	}
	store := &fakeStore{}
	evidence := ReportEvidence{AuditID: 42, ReportTaskID: "task-42"}
	projection, err := ProjectAndPublishFromSQL(context.Background(), reader, store, "account-1", "store-1", "sc_fba", date, date.AddDate(0, 0, 1), ReportReconciled, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Reconciliation == nil || len(projection.Reconciliation.FieldDiffs) != 1 {
		t.Fatalf("reconciliation = %#v", projection.Reconciliation)
	}
	if len(store.rows) != 1 || store.rows[0].Values.ReturnsQty == nil || *store.rows[0].Values.ReturnsQty != reportReturns {
		t.Fatalf("published correction = %#v", store.rows)
	}
	if reader.evidence != evidence {
		t.Fatalf("report evidence = %#v, want %#v", reader.evidence, evidence)
	}
}

func TestBuildFromSQLRequiresExactReportEvidenceBeforeReadingReport(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	reader := &reconciledSourceReader{}
	if _, _, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date, ReportReconciled); err == nil || !strings.Contains(err.Error(), "exact audit") {
		t.Fatalf("missing exact report evidence error = %v", err)
	}
	if reader.evidence.AuditID != 0 {
		t.Fatalf("report reader was called without exact evidence: %#v", reader.evidence)
	}
}

func TestBuildFromSQLUsesReportRowsMissingFromAPIAsCorrections(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	returns := int64(2)
	reader := &reconciledSourceReader{report: []RawRecord{{Source: SourceReport, Input: Input{
		Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &returns},
	}}}}
	projection, rows, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date.AddDate(0, 0, 1), ReportReconciled, ReportEvidence{AuditID: 42, ReportTaskID: "task-42"})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Reconciliation == nil || len(projection.Reconciliation.MissingInDB) != 1 {
		t.Fatalf("missing-in-api reconciliation = %#v", projection.Reconciliation)
	}
	if len(rows) != 1 || rows[0].Values.ReturnsQty == nil || *rows[0].Values.ReturnsQty != 2 || rows[0].Sources["returns_qty"] != SourceReport {
		t.Fatalf("report-only corrected rows = %#v", rows)
	}
}

func TestBuildFromSQLRejectsEmptyReportWhenAPIHasReturns(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	returns := int64(1)
	reader := &reconciledSourceReader{api: SQLProjection{Records: []RawRecord{{Source: SourceAPI, Input: Input{
		Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{ReturnsQty: &returns},
	}}}}}
	projection, rows, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date.AddDate(0, 0, 1), ReportReconciled, ReportEvidence{AuditID: 42, ReportTaskID: "task-42"})
	if err == nil || !strings.Contains(err.Error(), "report has no returns rows") {
		t.Fatalf("empty report error = %v", err)
	}
	if len(rows) != 0 || projection.Reconciliation == nil || len(projection.Reconciliation.MissingInReport) != 1 {
		t.Fatalf("empty report projection=%#v rows=%#v", projection, rows)
	}
}

type sourceReaderFunc func(context.Context, string, string, string, time.Time) (SQLProjection, error)

func (f sourceReaderFunc) Read(ctx context.Context, accountID, storeID, channel string, businessDate time.Time) (SQLProjection, error) {
	return f(ctx, accountID, storeID, channel, businessDate)
}

type reconciledSourceReader struct {
	api      SQLProjection
	report   []RawRecord
	evidence ReportEvidence
}

func (r reconciledSourceReader) Read(context.Context, string, string, string, time.Time) (SQLProjection, error) {
	return r.api, nil
}

func (r *reconciledSourceReader) ReadReportReturns(_ context.Context, _ string, _ string, _ string, _ time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	r.evidence = evidence
	return r.report, nil
}
