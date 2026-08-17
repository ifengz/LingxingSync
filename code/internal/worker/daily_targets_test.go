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

func TestFBAProjectionTargetKeepsOneTaskStartAcrossDateBoundary(t *testing.T) {
	startedAt := time.Date(2026, 8, 17, 23, 59, 59, 0, time.UTC)
	targets := projectionTargets(config.Endpoint{Table: "ls_fba_inventory"}, "store-1", []map[string]any{{}}, startedAt)
	if len(targets) != 1 || !targets[0].StartedAt.Equal(startedAt) || targets[0].Date.Format("2006-01-02") != "2026-08-17" {
		t.Fatalf("FBA snapshot targets=%#v", targets)
	}
}

func TestFBAProjectionFailsLoudWithoutHistoricalSnapshotPublisher(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{Table: "ls_fba_inventory"}, Account: config.Account{ID: "account-1"}}
	w.SetDailyProjector(func(context.Context, string, []DailyProjectionTarget, time.Time) error { return nil })
	err := w.projectDaily(context.Background(), []DailyProjectionTarget{{
		Store: "store-1", Channel: "sc_fba", Date: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC),
	}})
	if err == nil || !strings.Contains(err.Error(), "inventory snapshot publisher is not configured") {
		t.Fatalf("missing FBA history publisher error = %v", err)
	}
}

func TestFBAProjectionPublishesHistoryBeforeListingDaily(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{Table: "ls_fba_inventory"}, Account: config.Account{ID: "account-1"}}
	calls := make([]string, 0, 2)
	startedAt := time.Date(2026, 8, 17, 1, 2, 3, 0, time.UTC)
	w.SetInventorySnapshotter(func(_ context.Context, accountID string, targets []DailyProjectionTarget) error {
		calls = append(calls, "snapshot")
		if accountID != "account-1" || len(targets) != 1 || targets[0].Store != "store-1" || targets[0].Date.Format("2006-01-02") != "2026-08-17" || targets[0].StartedAt.IsZero() {
			t.Fatalf("snapshot batch account=%q targets=%#v", accountID, targets)
		}
		return nil
	})
	w.SetDailyProjector(func(context.Context, string, []DailyProjectionTarget, time.Time) error {
		calls = append(calls, "daily")
		return nil
	})
	err := w.projectDaily(context.Background(), []DailyProjectionTarget{
		{Store: "store-1", Channel: "sc_fba", Date: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC), StartedAt: startedAt},
		{Store: "store-1", Channel: "sc_fba", Date: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC), StartedAt: startedAt},
	})
	if err != nil {
		t.Fatalf("projectDaily: %v", err)
	}
	if got := strings.Join(calls, ","); got != "snapshot,daily" {
		t.Fatalf("publisher order=%q, want snapshot,daily", got)
	}
}

func TestFBAProjectionStopsWhenHistoricalSnapshotFails(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{Table: "ls_fba_inventory"}, Account: config.Account{ID: "account-1"}}
	w.SetInventorySnapshotter(func(context.Context, string, []DailyProjectionTarget) error {
		return errors.New("history unavailable")
	})
	dailyCalls := 0
	w.SetDailyProjector(func(context.Context, string, []DailyProjectionTarget, time.Time) error {
		dailyCalls++
		return nil
	})
	err := w.projectDaily(context.Background(), []DailyProjectionTarget{{
		Store: "store-1", Channel: "sc_fba", Date: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC),
	}})
	if err == nil || !strings.Contains(err.Error(), "history unavailable") {
		t.Fatalf("snapshot failure = %v", err)
	}
	if dailyCalls != 0 {
		t.Fatalf("daily publisher calls=%d after snapshot failure, want 0", dailyCalls)
	}
}
