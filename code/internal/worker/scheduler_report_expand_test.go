package worker

import (
	"context"
	"testing"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/reportexport"
)

// expand_stores=false（默认）保持旧行为：一条配置注册一个 job。
func TestRegisterReportJobsWithoutExpandKeepsSingleEntry(t *testing.T) {
	cfg := &config.Config{ReportExports: []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "@every 1h",
		Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
	}}}
	s := NewScheduler(cfg, NewRegistry(), nil, nil)
	s.customerReturnsRun = func(_ context.Context, _ reportexport.Request) (reportexport.Result, error) {
		return reportexport.Result{Status: "SUCCESS"}, nil
	}
	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Stop)
	if len(s.reportEntries) != 1 {
		t.Fatalf("report entries = %d, want 1", len(s.reportEntries))
	}
}

// expand_stores=true 且无 db：必须 fail-loud，不能静默按单店注册。
func TestRegisterReportJobsWithExpandRequiresDB(t *testing.T) {
	cfg := &config.Config{ReportExports: []config.ReportExport{{
		Type: config.ReportExportCustomerReturns, Enabled: true, Cron: "@every 1h",
		Account: "sc_us", SellerID: "SELLER-1", StoreID: "STORE-1",
		Region: "na", MarketplaceIDs: []string{"ATVPDKIKX0DER"}, WindowDays: 3,
		ExpandStores: true,
	}}}
	s := NewScheduler(cfg, NewRegistry(), nil, nil)
	s.customerReturnsRun = func(_ context.Context, _ reportexport.Request) (reportexport.Result, error) {
		return reportexport.Result{Status: "SUCCESS"}, nil
	}
	err := s.registerReportJobsLocked(cfg)
	if err == nil {
		t.Fatal("expand_stores without db handle must fail loudly")
	}
	if len(s.reportEntries) != 0 {
		t.Fatalf("report entries = %d, want 0", len(s.reportEntries))
	}
}
