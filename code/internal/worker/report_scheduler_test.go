package worker

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"testing"
	"time"

	"github.com/robfig/cron/v3"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/reportexport"
)

func TestCustomerReturnsWindowUsesOnlyCompleteDays(t *testing.T) {
	request, err := customerReturnsRequest(config.ReportExport{
		Type:           config.ReportExportCustomerReturns,
		Account:        "sc_us",
		SellerID:       "SELLER-1",
		StoreID:        "STORE-1",
		Region:         "na",
		MarketplaceIDs: []string{"ATVPDKIKX0DER"},
		WindowDays:     3,
	}, time.Date(2026, 8, 12, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}

	if request.DateFrom != "2026-08-09T00:00:00+08:00" {
		t.Fatalf("date_from = %q, want first complete day", request.DateFrom)
	}
	if request.DateTo != "2026-08-11T23:59:59+08:00" {
		t.Fatalf("date_to = %q, want end of yesterday", request.DateTo)
	}
}

func TestEstimatedFeesWindowEndsNowAndCoversAtLeastSeventyTwoHours(t *testing.T) {
	now := time.Date(2026, 8, 12, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60))
	request, err := customerReturnsRequest(config.ReportExport{
		Type: config.ReportExportFBAEstimatedFees, Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
	}, now)
	if err != nil {
		t.Fatal(err)
	}
	if request.DateFrom != "2026-08-09T06:35:00Z" || request.DateTo != "2026-08-12T06:35:00Z" {
		t.Fatalf("estimated fees window = %s..%s", request.DateFrom, request.DateTo)
	}
}

func TestReportJobCarriesConfiguredReportType(t *testing.T) {
	report := config.ReportExport{
		Type: config.ReportExportCustomerShipmentSales, Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 1,
	}
	request, err := customerReturnsRequest(report, time.Date(2026, 8, 12, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil {
		t.Fatal(err)
	}
	if request.ReportType != reportexport.CustomerShipmentSalesReportType {
		t.Fatalf("report type = %q, want %q", request.ReportType, reportexport.CustomerShipmentSalesReportType)
	}
}

func TestDisabledCustomerReturnsScheduleDoesNotRequireRunner(t *testing.T) {
	s := NewScheduler(&config.Config{ReportExports: []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: false,
	}}}, NewRegistry(), nil, nil)
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	if len(s.reportEntries) != 0 {
		t.Fatalf("disabled report entries = %d, want 0", len(s.reportEntries))
	}
}

func TestRegisterReportJobRequiresRunnerWhenEnabled(t *testing.T) {
	s := NewScheduler(&config.Config{ReportExports: []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "0 4 * * *", Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
	}}}, NewRegistry(), nil, nil)
	if err := s.registerReportJobsLocked(s.cfg); err == nil {
		t.Fatal("enabled customer returns schedule without runner was accepted")
	}
}

func TestRebuildRemovesDisabledCustomerReturnsJob(t *testing.T) {
	cfg := &config.Config{ReportExports: []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "@every 1h", Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
	}}}
	s := NewScheduler(cfg, NewRegistry(), nil, nil)
	s.customerReturnsRun = func(_ context.Context, request reportexport.Request) (reportexport.Result, error) {
		return reportexport.Result{Status: "SUCCESS"}, nil
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	if len(s.reportEntries) != 1 {
		t.Fatalf("report entries after Start = %d, want 1", len(s.reportEntries))
	}
	var oldEntryID cron.EntryID
	for entryID := range s.reportEntries {
		oldEntryID = entryID
	}

	disabled := *cfg
	disabled.ReportExports = append([]config.ReportExport(nil), cfg.ReportExports...)
	disabled.ReportExports[0].Enabled = false
	if err := s.Rebuild(&disabled); err != nil {
		t.Fatal(err)
	}
	if len(s.reportEntries) != 0 {
		t.Fatalf("report entries after disabling Rebuild = %d, want 0", len(s.reportEntries))
	}
	if entry := s.cron.Entry(oldEntryID); entry.ID != 0 {
		t.Fatalf("old report cron entry %d still exists after Rebuild", oldEntryID)
	}
}

func TestRebuildEnablesCustomerReturnsJobOnce(t *testing.T) {
	s := NewScheduler(&config.Config{}, NewRegistry(), nil, nil)
	s.customerReturnsRun = func(context.Context, reportexport.Request) (reportexport.Result, error) {
		return reportexport.Result{Status: "SUCCESS"}, nil
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	enabled := &config.Config{ReportExports: []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "@every 1h", Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
	}}}
	if err := s.Rebuild(enabled); err != nil {
		t.Fatal(err)
	}
	if err := s.Rebuild(enabled); err != nil {
		t.Fatal(err)
	}
	if len(s.reportEntries) != 1 {
		t.Fatalf("report entries after repeated Rebuild = %d, want 1", len(s.reportEntries))
	}
}

func TestCustomerReturnsJobPassesCompleteDayRequestToRunner(t *testing.T) {
	called := make(chan reportexport.Request, 1)
	s := NewScheduler(&config.Config{}, NewRegistry(), nil, nil)
	s.ctx = context.Background()
	s.now = func() time.Time {
		return time.Date(2026, 8, 12, 14, 35, 0, 0, time.FixedZone("CST", 8*60*60))
	}
	s.customerReturnsRun = func(_ context.Context, request reportexport.Request) (reportexport.Result, error) {
		called <- request
		return reportexport.Result{}, errors.New("upstream unavailable")
	}
	report := config.ReportExport{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "@every 1h", Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
	}
	s.cfg.ReportExports = []config.ReportExport{report}
	if err := s.registerReportJobsLocked(s.cfg); err != nil {
		t.Fatal(err)
	}
	for entryID := range s.reportEntries {
		s.cron.Entry(entryID).Job.Run()
	}
	select {
	case request := <-called:
		if request.AccountID != report.Account || request.StoreID != report.StoreID || request.DateFrom != "2026-08-09T00:00:00+08:00" || request.DateTo != "2026-08-11T23:59:59+08:00" {
			t.Fatalf("runner request = %#v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("report cron job did not call runner")
	}
}

func TestCustomerReturnsRunnerFailureIsLoggedByItsJob(t *testing.T) {
	oldWriter, oldFlags := log.Writer(), log.Flags()
	t.Cleanup(func() { log.SetOutput(oldWriter); log.SetFlags(oldFlags) })
	var output bytes.Buffer
	log.SetOutput(&output)
	log.SetFlags(0)
	s := NewScheduler(&config.Config{}, NewRegistry(), nil, nil)
	s.ctx = context.Background()
	s.customerReturnsRun = func(context.Context, reportexport.Request) (reportexport.Result, error) {
		return reportexport.Result{}, errors.New("upstream unavailable")
	}
	s.cfg.ReportExports = []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "@every 1h", Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 1,
	}}
	if err := s.registerReportJobsLocked(s.cfg); err != nil {
		t.Fatal(err)
	}
	for entryID := range s.reportEntries {
		s.cron.Entry(entryID).Job.Run()
	}
	if !strings.Contains(output.String(), "upstream unavailable") {
		t.Fatalf("runner failure log = %q", output.String())
	}
}
