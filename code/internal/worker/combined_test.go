package worker

import (
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

func TestCombinedMetricPlansSeparateDailyAndSnapshotDates(t *testing.T) {
	ep := config.Endpoint{
		WindowDays:       3,
		SingleDayWindow:  true,
		WindowStartField: "start_date",
		WindowEndField:   "end_date",
	}
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	plans, err := metricPlansForAt(ep, "combined", triggerReq{kind: "cron"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 2 {
		t.Fatalf("plans=%d, want daily + snapshot", len(plans))
	}
	if plans[0].Scope != "daily" || len(plans[0].Params) != 3 {
		t.Fatalf("daily plan=%#v", plans[0])
	}
	for i, params := range plans[0].Params {
		want := now.AddDate(0, 0, -i).Format("2006-01-02")
		if params["start_date"] != want || params["end_date"] != want {
			t.Fatalf("daily[%d]=%#v, want %s", i, params, want)
		}
	}
	if plans[1].Scope != "snapshot" || len(plans[1].Params) != 1 {
		t.Fatalf("snapshot plan=%#v", plans[1])
	}
	if got := plans[1].Params[0]["start_date"]; got != "2026-09-15" {
		t.Fatalf("snapshot date=%v, want today", got)
	}
}

func TestCombinedMetricColumnsDoNotCrossOverwrite(t *testing.T) {
	columns := []string{"sid", "asin", "business_date", "sessions", "sessions_mobile", "sessions_total", "cate_rank", "small_cate_rank", "reviews_count", "avg_star", "promotion_discount", "currency_code", "amount"}
	keys := []string{"sid", "asin", "business_date"}
	if got := combinedMetricColumns("daily", columns, keys); !sameStrings(got, []string{"sid", "asin", "business_date", "sessions", "sessions_mobile", "sessions_total", "currency_code"}) {
		t.Fatalf("daily columns=%v", got)
	}
	if got := combinedMetricColumns("snapshot", columns, keys); !sameStrings(got, []string{"sid", "asin", "business_date", "cate_rank", "small_cate_rank", "reviews_count", "avg_star", "promotion_discount", "currency_code", "amount"}) {
		t.Fatalf("snapshot columns=%v", got)
	}
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
