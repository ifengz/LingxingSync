package worker

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

func TestDailyProjectionChannelIsLimitedToFactSources(t *testing.T) {
	for _, tc := range []struct {
		table   string
		channel string
		ok      bool
	}{
		{"ls_sc_sales_report", "sc_fba", true},
		{"ls_sc_sales_revenue", "sc_fba", true},
		{"ls_sc_refunds", "sc_fba", true},
		{"ls_fba_inventory", "sc_fba", true},
		{"ls_vc_sales_report", "vc", true},
		{"ls_vc_inventory", "vc", true},
		{"ls_vc_traffic", "", false},
		{"ls_ad_hsa_campaign", "hsa", true},
	} {
		got, ok := dailyProjectionChannel(tc.table)
		if got != tc.channel || ok != tc.ok {
			t.Fatalf("dailyProjectionChannel(%q)=(%q,%v), want (%q,%v)", tc.table, got, ok, tc.channel, tc.ok)
		}
	}
}

func TestDailyProjectionDatesExpandsInclusiveWindow(t *testing.T) {
	ep := config.Endpoint{WindowStartField: "startDate", WindowEndField: "endDate"}
	dates, err := dailyProjectionDates(ep, map[string]any{"startDate": "2026-08-09", "endDate": "2026-08-11"}, time.Now())
	if err != nil || len(dates) != 3 || dates[0].Format("2006-01-02") != "2026-08-09" || dates[2].Format("2006-01-02") != "2026-08-11" {
		t.Fatalf("window targets=%v err=%v", dates, err)
	}
}

func TestProjectDailyFailsLoudForMissingPublisherOrProjectionError(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{Table: "ls_vc_sales_report"}, Account: config.Account{ID: "account-1"}}
	targets := []DailyProjectionTarget{
		{Store: "store-1", Channel: "vc", Date: time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)},
		{Store: "store-1", Channel: "vc", Date: time.Date(2026, 8, 12, 0, 0, 0, 0, time.UTC)},
	}
	if err := w.projectDaily(context.Background(), targets); err == nil || !strings.Contains(err.Error(), "not configured") {
		t.Fatalf("missing publisher error = %v", err)
	}
	calls := 0
	w.SetDailyProjector(func(_ context.Context, accountID string, got []DailyProjectionTarget, _ time.Time) error {
		calls++
		if accountID != "account-1" || len(got) != 2 {
			t.Fatalf("batch scope account=%q targets=%#v", accountID, got)
		}
		return errors.New("publish failed")
	})
	if err := w.projectDaily(context.Background(), targets); err == nil || !strings.Contains(err.Error(), "publish failed") {
		t.Fatalf("projection error = %v", err)
	}
	if calls != 1 {
		t.Fatalf("daily publisher calls=%d, want one atomic batch", calls)
	}
}

func TestDailyProjectionDatesUseRequestDateAndSnapshotToday(t *testing.T) {
	ep := config.Endpoint{DateField: "report_date"}
	dates, err := dailyProjectionDates(ep, map[string]any{"report_date": "2026-08-11"}, time.Date(2026, 8, 12, 10, 0, 0, 0, time.FixedZone("CST", 8*60*60)))
	if err != nil || len(dates) != 1 || dates[0].Format("2006-01-02") != "2026-08-11" {
		t.Fatalf("date-field targets=%v err=%v", dates, err)
	}
	ep = config.Endpoint{Table: "ls_fba_inventory"}
	dates, err = dailyProjectionDates(ep, map[string]any{}, time.Date(2026, 8, 12, 10, 0, 0, 0, time.UTC))
	if err != nil || len(dates) != 1 || dates[0].Format("2006-01-02") != "2026-08-12" {
		t.Fatalf("snapshot targets=%v err=%v", dates, err)
	}
}
