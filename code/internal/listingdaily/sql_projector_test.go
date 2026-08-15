package listingdaily

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestInventoryProjectionContractsSeparateVCDailyAndSCSnapshotSources(t *testing.T) {
	for _, want := range []string{"FROM ls_vc_inventory", "`date` = ?", "sellableOnHandInventoryUnits", "unhealthyInventoryUnits", "aged90PlusDaysSellableInventoryUnits"} {
		if !strings.Contains(vcInventorySQL, want) {
			t.Fatalf("VC inventory query missing %q: %s", want, vcInventorySQL)
		}
	}
	for _, want := range []string{"FROM ls_fba_inventory", "DATE(synced_at) = ?", "fnsku", "afn_fulfillable_quantity", "afn_inbound_receiving_quantity", "reserved_customerorders", "reserved_fc_transfers", "afn_unsellable_quantity"} {
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

func TestReservedInventoryReportIncludesFCTransfers(t *testing.T) {
	if !strings.Contains(reportReservedInventorySQL, "`reserved_fc-transfers`") || !strings.Contains(reportReservedInventorySQL, "reserved_fc_transfers") {
		t.Fatalf("reserved inventory SQL missing FC transfers: %s", reportReservedInventorySQL)
	}
	fields := inventoryReportFields("GET_RESERVED_INVENTORY_DATA")
	found := false
	for _, field := range fields {
		found = found || field == "inventory_reserved_fc_transfers"
	}
	if !found {
		t.Fatalf("reserved inventory fields = %#v", fields)
	}
	value := int64(3)
	values := Values{}
	setMetricField(&values, "inventory_reserved_fc_transfers", &value)
	if values.InventoryReservedFCTransfers == nil || *values.InventoryReservedFCTransfers != 3 {
		t.Fatalf("reserved FC transfers mapping = %#v", values.InventoryReservedFCTransfers)
	}
}

func TestReservedInventoryReportOmitsFieldsAbsentFromProductionVariant(t *testing.T) {
	value := int64(8)
	rows := []Metric{{Values: Values{InventoryReserved: &value}}}
	fields := reportFieldsPresent(inventoryReportFields("GET_RESERVED_INVENTORY_DATA"), rows)
	if len(fields) != 1 || fields[0] != "inventory_reserved" {
		t.Fatalf("fields = %#v", fields)
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

func TestBuildFromSQLReconcilesShipmentSalesUnitsFromFormalReport(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	apiUnits, reportUnits := int64(1), int64(3)
	reader := &reconciledSourceReader{
		api: SQLProjection{Records: []RawRecord{{Source: SourceAPI, Input: Input{
			Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: &apiUnits},
		}}}},
		salesReport: []RawRecord{{Source: SourceReport, Input: Input{
			Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{SalesUnits: &reportUnits},
		}}},
	}
	projection, rows, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date.AddDate(0, 0, 1), ReportReconciled, ReportEvidence{AuditID: 43, ReportTaskID: "sales-task-43", ReportType: "GET_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_SALES_DATA"})
	if err != nil {
		t.Fatal(err)
	}
	if projection.Reconciliation == nil || len(projection.Reconciliation.FieldDiffs) != 1 {
		t.Fatalf("sales reconciliation = %#v", projection.Reconciliation)
	}
	if len(rows) != 1 || rows[0].Values.SalesUnits == nil || *rows[0].Values.SalesUnits != reportUnits || rows[0].Sources["sales_units"] != SourceReport {
		t.Fatalf("sales correction = %#v", rows)
	}
}

func TestBuildFromSQLReconcilesFBAInventoryFieldsFromFormalReport(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	apiSellable, reportSellable, reportReserved := int64(8), int64(10), int64(3)
	reader := &reconciledSourceReader{
		api: SQLProjection{Records: []RawRecord{{Source: SourceAPI, Input: Input{
			Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing, Values: Values{InventorySellable: &apiSellable},
		}}}},
		inventoryReport: []RawRecord{{Source: SourceReport, Input: Input{
			Key: Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing,
			Values: Values{InventorySellable: &reportSellable, InventoryReserved: &reportReserved},
		}}},
	}
	evidence := ReportEvidence{AuditID: 44, ReportTaskID: "inventory-task-44", ReportType: "GET_FBA_MYI_UNSUPPRESSED_INVENTORY_DATA"}
	projection, rows, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date.AddDate(0, 0, 1), ReportReconciled, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Reconciliation == nil || len(projection.Reconciliation.FieldDiffs) != 2 {
		t.Fatalf("inventory reconciliation = %#v", projection.Reconciliation)
	}
	if len(rows) != 1 || rows[0].Values.InventorySellable == nil || *rows[0].Values.InventorySellable != 10 || rows[0].Values.InventoryReserved == nil || *rows[0].Values.InventoryReserved != 3 {
		t.Fatalf("inventory correction = %#v", rows)
	}
	if rows[0].Sources["inventory_sellable"] != SourceReport || rows[0].Sources["inventory_reserved"] != SourceReport {
		t.Fatalf("inventory sources = %#v", rows[0].Sources)
	}
}

func TestReadInventorySumsFNSKURowsBeforeAFNReconciliation(t *testing.T) {
	now := time.Now().UTC()
	date := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	fixtures := []inventoryFixtureRow{
		{FNSKU: "FNSKU-1", Values: []driver.Value{"B01", "SKU-1", "FNSKU-1", int64(3), nil, nil, nil, nil, nil, nil, nil}},
		{FNSKU: "FNSKU-2", Values: []driver.Value{"B01", "SKU-1", "FNSKU-2", int64(5), nil, nil, nil, nil, nil, nil, nil}},
	}
	if fixtures[0].FNSKU == fixtures[1].FNSKU {
		t.Fatal("fixture must represent two FNSKUs")
	}
	db := sql.OpenDB(inventoryFixtureConnector{Rows: fixtures})
	defer db.Close()
	out := SQLProjection{}
	if err := (SQLSourceReader{DB: sqlx.NewDb(db, "inventory-fixture")}).readInventory(context.Background(), &out, "account-1", "store-1", "sc_fba", date, nil); err != nil {
		t.Fatal(err)
	}

	reportSellable := int64(10)
	key := Key{Store: "store-1", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}
	reader := &reconciledSourceReader{
		api: out,
		inventoryReport: []RawRecord{{Source: SourceReport, Input: Input{
			Key: key, Scope: ScopeListing, Values: Values{InventorySellable: &reportSellable},
		}}},
	}
	evidence := ReportEvidence{AuditID: 31, ReportTaskID: "inventory-task-31", ReportType: "GET_AFN_INVENTORY_DATA"}
	projection, rows, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date.AddDate(0, 0, 1), ReportReconciled, evidence)
	if err != nil {
		t.Fatal(err)
	}
	if projection.Reconciliation == nil || len(projection.Reconciliation.FieldDiffs) != 1 {
		t.Fatalf("inventory reconciliation = %#v", projection.Reconciliation)
	}
	databaseValue, ok := projection.Reconciliation.FieldDiffs[0].Database.(*int64)
	if !ok || databaseValue == nil || *databaseValue != 8 {
		t.Fatalf("API inventory_sellable = %#v, want 8", projection.Reconciliation.FieldDiffs[0].Database)
	}
	if len(rows) != 1 || rows[0].Values.InventorySellable == nil || *rows[0].Values.InventorySellable != reportSellable {
		t.Fatalf("AFN inventory correction = %#v", rows)
	}
}

type inventoryFixtureRow struct {
	FNSKU  string
	Values []driver.Value
}

type inventoryFixtureConnector struct{ Rows []inventoryFixtureRow }

func (c inventoryFixtureConnector) Connect(context.Context) (driver.Conn, error) {
	return &inventoryFixtureConn{rows: c.Rows}, nil
}

func (inventoryFixtureConnector) Driver() driver.Driver { return inventoryFixtureDriver{} }

type inventoryFixtureDriver struct{}

func (inventoryFixtureDriver) Open(string) (driver.Conn, error) { return nil, driver.ErrBadConn }

type inventoryFixtureConn struct{ rows []inventoryFixtureRow }

func (c *inventoryFixtureConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *inventoryFixtureConn) Close() error                        { return nil }
func (c *inventoryFixtureConn) Begin() (driver.Tx, error)           { return nil, driver.ErrSkip }
func (c *inventoryFixtureConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &inventoryFixtureRows{rows: c.rows}, nil
}

type inventoryFixtureRows struct {
	rows  []inventoryFixtureRow
	index int
}

func (*inventoryFixtureRows) Columns() []string {
	return []string{"asin", "sku", "fnsku", "afn_fulfillable_quantity", "afn_inbound_receiving_quantity", "afn_inbound_shipped_quantity", "afn_inbound_working_quantity", "reserved_customerorders", "reserved_fc_processing", "reserved_fc_transfers", "afn_unsellable_quantity"}
}

func (*inventoryFixtureRows) Close() error { return nil }

func (r *inventoryFixtureRows) Next(values []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(values, r.rows[r.index].Values)
	r.index++
	return nil
}

func TestBuildFromSQLRejectsUnknownReportTypeBeforeReadingRaw(t *testing.T) {
	date := time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)
	reader := &reconciledSourceReader{}
	_, _, err := BuildFromSQL(context.Background(), reader, "account-1", "store-1", "sc_fba", date, date, ReportReconciled, ReportEvidence{AuditID: 45, ReportTaskID: "unknown-task", ReportType: "GET_UNKNOWN"})
	if err == nil || !strings.Contains(err.Error(), "unsupported reconciled report type") {
		t.Fatalf("unknown report error = %v", err)
	}
	if reader.evidence.AuditID != 0 {
		t.Fatalf("unknown report reader was called: %#v", reader.evidence)
	}
}

type sourceReaderFunc func(context.Context, string, string, string, time.Time) (SQLProjection, error)

func (f sourceReaderFunc) Read(ctx context.Context, accountID, storeID, channel string, businessDate time.Time) (SQLProjection, error) {
	return f(ctx, accountID, storeID, channel, businessDate)
}

type reconciledSourceReader struct {
	api             SQLProjection
	report          []RawRecord
	salesReport     []RawRecord
	inventoryReport []RawRecord
	evidence        ReportEvidence
}

func (r reconciledSourceReader) Read(context.Context, string, string, string, time.Time) (SQLProjection, error) {
	return r.api, nil
}

func (r *reconciledSourceReader) ReadReportReturns(_ context.Context, _ string, _ string, _ string, _ time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	r.evidence = evidence
	return r.report, nil
}

func (r *reconciledSourceReader) ReadReportSales(_ context.Context, _ string, _ string, _ string, _ time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	r.evidence = evidence
	return r.salesReport, nil
}

func (r *reconciledSourceReader) ReadReportInventory(_ context.Context, _ string, _ string, _ string, _ time.Time, evidence ReportEvidence) ([]RawRecord, error) {
	r.evidence = evidence
	return r.inventoryReport, nil
}
