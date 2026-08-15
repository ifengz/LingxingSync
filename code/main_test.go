package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
	"lingxing-sync/internal/listingdaily"
	"lingxing-sync/internal/reportexport"
	"lingxing-sync/internal/server"
	"lingxing-sync/internal/worker"
)

func TestCustomerReturnsRunUsesConfiguredAccount(t *testing.T) {
	cfg := &config.Config{Accounts: []config.Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret"}}}
	clients := api.NewClientRegistry(cfg.Accounts, "http://example.test")
	run := customerReturnsRun(cfg, clients, db.NewReportStore(nil), worker.NewLimiter(1, 1), nil, nil)
	_, err := run(context.Background(), reportexport.Request{AccountID: "missing"})
	if err == nil || !strings.Contains(err.Error(), "missing") {
		t.Fatalf("unknown report account error = %v", err)
	}
	_, err = run(context.Background(), reportexport.Request{
		AccountID: "SC_US", SellerID: "SELLER-1", StoreID: "STORE-1", Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z",
	})
	if err == nil || strings.Contains(err.Error(), "账号不存在") {
		t.Fatalf("case-insensitive configured account error = %v", err)
	}
}

func TestPrepareDatabaseUsesReportSchemaValidationInsteadOfMigrations(t *testing.T) {
	migrations, validations := 0, 0
	err := prepareDatabase(true,
		func() error {
			migrations++
			return nil
		},
		func() error {
			validations++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if migrations != 0 || validations != 1 {
		t.Fatalf("report database preparation migrations=%d validations=%d, want 0/1", migrations, validations)
	}
}

func TestPrepareDatabaseKeepsMigrationsForNormalServer(t *testing.T) {
	migrations, validations := 0, 0
	err := prepareDatabase(false,
		func() error {
			migrations++
			return nil
		},
		func() error {
			validations++
			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if migrations != 1 || validations != 0 {
		t.Fatalf("server database preparation migrations=%d validations=%d, want 1/0", migrations, validations)
	}
}

func TestValidateCustomerReturnsSchemaFailsBeforeRunOnMissingColumn(t *testing.T) {
	requirements := customerReturnsSchemaRequirements()
	loadColumns := func(table string) ([]string, error) {
		columns := append([]string(nil), requirements[table]...)
		if table == "ls_report_export_tasks" {
			for i, column := range columns {
				if column == "report_document_id" {
					return append(columns[:i], columns[i+1:]...), nil
				}
			}
		}
		return columns, nil
	}
	err := validateCustomerReturnsSchema(loadColumns)
	if err == nil || !strings.Contains(err.Error(), "ls_report_export_tasks.report_document_id") {
		t.Fatalf("missing report schema error = %v", err)
	}
}

func TestValidateCustomerReturnsSchemaAcceptsCompleteContract(t *testing.T) {
	requirements := customerReturnsSchemaRequirements()
	err := validateCustomerReturnsSchema(func(table string) ([]string, error) {
		return append([]string(nil), requirements[table]...), nil
	})
	if err != nil {
		t.Fatalf("complete report schema = %v", err)
	}
}

func TestValidateCustomerShipmentSalesSchemaDoesNotRequireReturnsTable(t *testing.T) {
	requirements := formalReportSchemaRequirements(reportexport.CustomerShipmentSalesReportType)
	if _, ok := requirements["ls_fba_fulfillment_customer_returns"]; ok {
		t.Fatal("shipment sales schema must not require customer returns raw table")
	}
	if err := validateFormalReportSchema(reportexport.CustomerShipmentSalesReportType, func(table string) ([]string, error) {
		return append([]string(nil), requirements[table]...), nil
	}); err != nil {
		t.Fatalf("complete shipment sales schema = %v", err)
	}
}

func TestReservedInventorySchemaRequiresFCTransfers(t *testing.T) {
	requirements := formalReportSchemaRequirements(reportexport.ReservedInventoryReportType)
	columns := requirements["ls_fba_reserved_inventory"]
	if !slices.Contains(columns, "reserved_fc-transfers") {
		t.Fatalf("Reserved Inventory schema columns = %#v", columns)
	}
}

func TestShipmentReplacementsIsRawOnlyWithoutDailyProjection(t *testing.T) {
	if reportRequiresDailyProjection(reportexport.CustomerShipmentReplacementsReportType) {
		t.Fatal("replacement report must not be mapped onto an unrelated daily metric")
	}
}

func TestAdditionalFBAReportsHaveIndependentRawSchemas(t *testing.T) {
	tests := map[string]string{
		reportexport.FBAStrandedInventoryReportType:    "ls_fba_stranded_inventory",
		reportexport.FBAEstimatedFeesReportType:        "ls_fba_estimated_fees",
		reportexport.FBAInboundNoncomplianceReportType: "ls_fba_inbound_noncompliance",
	}
	for reportType, table := range tests {
		requirements := formalReportSchemaRequirements(reportType)
		if len(requirements[table]) <= 6 {
			t.Fatalf("%s raw schema = %#v", reportType, requirements)
		}
		if reportRequiresDailyProjection(reportType) {
			t.Fatalf("%s must remain raw-only without a matching daily metric", reportType)
		}
		if _, coupled := requirements["listing_daily_metrics"]; coupled {
			t.Fatalf("%s raw-only schema unexpectedly requires listing_daily_metrics", reportType)
		}
	}
}

func TestRemovalReportsHaveIndependentRawSchemas(t *testing.T) {
	tests := map[string]string{
		reportexport.FBARecommendedRemovalReportType: "ls_fba_recommended_removals",
		reportexport.FBARemovalOrderReportType:       "ls_fba_removal_order_details",
		reportexport.FBARemovalShipmentReportType:    "ls_fba_removal_shipment_details",
	}
	for reportType, table := range tests {
		requirements := formalReportSchemaRequirements(reportType)
		if len(requirements[table]) <= 6 || reportRequiresDailyProjection(reportType) {
			t.Fatalf("%s schema is not raw-only: %#v", reportType, requirements)
		}
		if _, coupled := requirements["listing_daily_metrics"]; coupled {
			t.Fatalf("%s unexpectedly requires listing_daily_metrics", reportType)
		}
	}
}

func TestValidateFormalReportSchemaRejectsUnsupportedType(t *testing.T) {
	err := validateFormalReportSchema("GET_UNSUPPORTED_REPORT", func(string) ([]string, error) {
		t.Fatal("unsupported report type must fail before schema reads")
		return nil, nil
	})
	if err == nil || !strings.Contains(err.Error(), "不支持") {
		t.Fatalf("unsupported report schema error = %v", err)
	}
}

func TestReportBusinessDatesUsesInclusiveCalendarDays(t *testing.T) {
	from, to, err := reportBusinessDates(reportexport.Request{DateFrom: "2026-08-09T00:00:00+08:00", DateTo: "2026-08-11T23:59:59+08:00"})
	if err != nil {
		t.Fatal(err)
	}
	if from.Format("2006-01-02") != "2026-08-09" || to.Format("2006-01-02") != "2026-08-11" {
		t.Fatalf("report dates = %s..%s", from, to)
	}
	if _, _, err := reportBusinessDates(reportexport.Request{DateFrom: "bad", DateTo: "2026-08-11T23:59:59Z"}); err == nil {
		t.Fatal("invalid report date was accepted")
	}
}

func TestFormalReportBusinessDatesUsesOnlyUTCInventorySnapshotDay(t *testing.T) {
	now := time.Date(2026, 8, 15, 18, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	from, to, err := formalReportBusinessDates(reportexport.FBAInventoryReportType, reportexport.Request{
		DateFrom: "2026-08-01T00:00:00Z", DateTo: "2026-08-14T23:59:59Z",
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-08-15"
	if from.Format("2006-01-02") != want || to.Format("2006-01-02") != want || from.Location() != time.UTC || to.Location() != time.UTC {
		t.Fatalf("inventory snapshot dates = %s..%s (%s/%s), want UTC %s", from, to, from.Location(), to.Location(), want)
	}
}

func TestProjectCustomerReturnsProjectsEveryCoveredDay(t *testing.T) {
	reader := &reportProjectionReader{}
	store := &reportProjectionStore{}
	request := reportexport.Request{
		AccountID: "account-1", StoreID: "store-1",
		DateFrom: "2026-08-09T00:00:00Z", DateTo: "2026-08-11T23:59:59Z",
	}
	result := reportexport.Result{AuditID: 42, ReportTaskID: "task-42"}
	if err := projectCustomerReturns(context.Background(), reader, store, request, result); err != nil {
		t.Fatal(err)
	}
	if len(reader.dates) != 3 || reader.dates[0] != "2026-08-09" || reader.dates[2] != "2026-08-11" {
		t.Fatalf("projected report dates = %v", reader.dates)
	}
	if len(store.rows) != 3 {
		t.Fatalf("published report rows = %d, want 3", len(store.rows))
	}
	if store.reportCalls != 1 || store.persistCalls != 0 || len(store.audits) != 3 {
		t.Fatalf("report publish calls=%d regular=%d audits=%d", store.reportCalls, store.persistCalls, len(store.audits))
	}
	for _, evidence := range reader.evidence {
		if evidence.AuditID != result.AuditID || evidence.ReportTaskID != result.ReportTaskID {
			t.Fatalf("report evidence=%#v, want audit=%d task=%s", evidence, result.AuditID, result.ReportTaskID)
		}
	}
	if len(store.audits[0].Reconciliation.FieldDiffs) != 1 {
		t.Fatalf("persisted reconciliation=%#v", store.audits[0])
	}
}

func TestProjectCustomerShipmentSalesUsesSalesEvidence(t *testing.T) {
	reader := &reportProjectionReader{sales: true}
	store := &reportProjectionStore{}
	request := reportexport.Request{
		ReportType: reportexport.CustomerShipmentSalesReportType,
		AccountID:  "account-1", StoreID: "store-1",
		DateFrom: "2026-08-11T00:00:00Z", DateTo: "2026-08-11T23:59:59Z",
	}
	result := reportexport.Result{AuditID: 44, ReportTaskID: "task-44"}
	if err := projectCustomerShipmentSales(context.Background(), reader, store, request, result); err != nil {
		t.Fatal(err)
	}
	if len(reader.evidence) != 1 || reader.evidence[0].ReportType != reportexport.CustomerShipmentSalesReportType {
		t.Fatalf("shipment sales evidence=%#v", reader.evidence)
	}
	if len(store.rows) != 1 || store.rows[0].Values.SalesUnits == nil || *store.rows[0].Values.SalesUnits != 2 {
		t.Fatalf("shipment sales rows=%#v", store.rows)
	}
}

func TestProjectCustomerReturnsDoesNotPublishEarlierDatesWhenLateDateFails(t *testing.T) {
	reader := &reportProjectionReader{failDate: "2026-08-11"}
	store := &reportProjectionStore{}
	request := reportexport.Request{AccountID: "account-1", StoreID: "store-1", DateFrom: "2026-08-09T00:00:00Z", DateTo: "2026-08-11T23:59:59Z"}
	result := reportexport.Result{AuditID: 43, ReportTaskID: "task-43"}
	if err := projectCustomerReturns(context.Background(), reader, store, request, result); err == nil || !strings.Contains(err.Error(), "source failed") {
		t.Fatalf("late projection error=%v", err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("late failure published %d earlier rows", len(store.rows))
	}
	if len(store.audits) != 1 || store.audits[0].Status != listingdaily.ReconciliationFailed || store.audits[0].BusinessDate.Format("2006-01-02") != "2026-08-11" {
		t.Fatalf("failed reconciliation audit=%#v", store.audits)
	}
}

func TestProjectDailyBatchDoesNotPublishEarlierTargetWhenLateTargetFails(t *testing.T) {
	reader := &reportProjectionReader{failDate: "2026-08-12"}
	store := &reportProjectionStore{}
	targets := []worker.DailyProjectionTarget{
		{Store: "store-1", Channel: "sc_fba", Date: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)},
		{Store: "store-1", Channel: "sc_fba", Date: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)},
	}
	if err := projectDailyBatch(context.Background(), reader, store, "account-1", targets, time.Now()); err == nil || !strings.Contains(err.Error(), "source failed") {
		t.Fatalf("late daily projection error=%v", err)
	}
	if store.persistCalls != 0 || len(store.rows) != 0 {
		t.Fatalf("late daily failure calls=%d rows=%d", store.persistCalls, len(store.rows))
	}
}

type reportProjectionReader struct {
	dates    []string
	evidence []listingdaily.ReportEvidence
	failDate string
	sales    bool
}

func (r *reportProjectionReader) Read(_ context.Context, accountID, storeID, channel string, date time.Time) (listingdaily.SQLProjection, error) {
	if accountID != "account-1" || storeID != "store-1" || channel != "sc_fba" {
		return listingdaily.SQLProjection{}, fmt.Errorf("unexpected scope %s/%s/%s", accountID, storeID, channel)
	}
	r.dates = append(r.dates, date.Format("2006-01-02"))
	if date.Format("2006-01-02") == r.failDate {
		return listingdaily.SQLProjection{}, fmt.Errorf("source failed")
	}
	value := int64(1)
	values := listingdaily.Values{ReturnsQty: &value}
	if r.sales {
		values = listingdaily.Values{SalesUnits: &value}
	}
	return listingdaily.SQLProjection{Records: []listingdaily.RawRecord{{
		Source: listingdaily.SourceAPI,
		Input:  listingdaily.Input{Key: listingdaily.Key{Store: storeID, Channel: channel, ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: listingdaily.ScopeListing, Values: values},
	}}}, nil
}

func (r *reportProjectionReader) ReadReportReturns(_ context.Context, _ string, storeID, channel string, date time.Time, evidence listingdaily.ReportEvidence) ([]listingdaily.RawRecord, error) {
	r.evidence = append(r.evidence, evidence)
	value := int64(2)
	return []listingdaily.RawRecord{{
		Source: listingdaily.SourceReport,
		Input:  listingdaily.Input{Key: listingdaily.Key{Store: storeID, Channel: channel, ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: listingdaily.ScopeListing, Values: listingdaily.Values{ReturnsQty: &value}},
	}}, nil
}

func (r *reportProjectionReader) ReadReportSales(_ context.Context, _ string, storeID, channel string, date time.Time, evidence listingdaily.ReportEvidence) ([]listingdaily.RawRecord, error) {
	r.evidence = append(r.evidence, evidence)
	value := int64(2)
	return []listingdaily.RawRecord{{
		Source: listingdaily.SourceReport,
		Input:  listingdaily.Input{Key: listingdaily.Key{Store: storeID, Channel: channel, ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: listingdaily.ScopeListing, Values: listingdaily.Values{SalesUnits: &value}},
	}}, nil
}

type reportProjectionStore struct {
	rows         []listingdaily.Metric
	audits       []listingdaily.ReconciliationAudit
	persistCalls int
	reportCalls  int
}

func (s *reportProjectionStore) PersistFailedReconciliations(_ context.Context, audits []listingdaily.ReconciliationAudit) error {
	for _, audit := range audits {
		if audit.Status != listingdaily.ReconciliationFailed {
			return fmt.Errorf("non-failed audit passed to failure store: %s", audit.Status)
		}
	}
	s.audits = append(s.audits, audits...)
	return nil
}

func (s *reportProjectionStore) Persist(_ context.Context, rows []listingdaily.Metric) error {
	s.persistCalls++
	s.rows = append(s.rows, rows...)
	return nil
}

func (s *reportProjectionStore) PersistReportBatch(_ context.Context, rows []listingdaily.Metric, audits []listingdaily.ReconciliationAudit) error {
	s.reportCalls++
	s.rows = append(s.rows, rows...)
	s.audits = append(s.audits, audits...)
	return nil
}

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
