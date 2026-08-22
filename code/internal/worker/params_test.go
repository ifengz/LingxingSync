package worker

import (
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

// TestBaseParamsInjectsSingleDate 验证通用单日期机制：DateField 非空时注入
// 「今天往前 DateOffsetDays 天」的单个 YYYY-MM-DD，且不误注入 window 范围。
func TestBaseParamsInjectsSingleDate(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		DateField:      "event_date",
		DateOffsetDays: 1, // 昨天
		ExtraParams:    map[string]any{"asin_type": 1, "type": 1},
	}}

	params := w.baseParams()

	want := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	if got := params["event_date"]; got != want {
		t.Fatalf("event_date = %v, want %v (昨天)", got, want)
	}
	// extra_params 应被 string 化保留
	if got := params["asin_type"]; got != "1" {
		t.Fatalf("asin_type = %v, want \"1\"（extra_params 应 string 化）", got)
	}
	// 单日期模式不应注入 window 范围
	if _, ok := params["start_date"]; ok {
		t.Fatal("单日期模式误注入了 start_date")
	}
	if _, ok := params["end_date"]; ok {
		t.Fatal("单日期模式误注入了 end_date")
	}
}

// TestBaseParamsNoDateFieldMeansNoInjection 验证 DateField 为空时整套机制不生效。
func TestBaseParamsNoDateFieldMeansNoInjection(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		DateOffsetDays: 3, // 有 offset 但没 DateField → 不应注入任何日期
	}}

	params := w.baseParams()

	if len(params) != 0 {
		t.Fatalf("DateField 为空时不应注入任何参数，got %v", params)
	}
}

func TestBaseParamsForManualRangeOverridesConfiguredWindow(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		WindowDays:       7,
		WindowStartField: "startDate",
		WindowEndField:   "endDate",
	}}
	params, err := w.baseParamsFor(triggerReq{kind: "manual", dateFrom: "2026-08-01", dateTo: "2026-08-03"})
	if err != nil {
		t.Fatal(err)
	}
	if params["startDate"] != "2026-08-01" || params["endDate"] != "2026-08-03" {
		t.Fatalf("manual date range = %#v, want exact override", params)
	}
}

func TestSingleDayWindowBuildsDailyCompensationRange(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		WindowDays: 7, SingleDayWindow: true, RowDateField: "business_date",
	}}
	now := time.Date(2026, time.August, 12, 23, 59, 59, 0, time.UTC)
	sets, err := w.paramSetsForAt(triggerReq{kind: "cron"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 7 {
		t.Fatalf("daily compensation sets=%d, want 7", len(sets))
	}
	for i, params := range sets {
		want := now.AddDate(0, 0, -i).Format("2006-01-02")
		if params["start_date"] != want || params["end_date"] != want {
			t.Fatalf("set %d = %#v, want %s for both bounds", i, params, want)
		}
		if _, sent := params["business_date"]; sent {
			t.Fatal("row_date_field must not be sent upstream")
		}
	}
}

func TestSingleDayWindowAppliesDateOffset(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		WindowDays: 1, SingleDayWindow: true, DateOffsetDays: 1,
		WindowStartField: "startDate", WindowEndField: "endDate",
	}}
	now := time.Date(2026, time.August, 22, 5, 0, 0, 0, time.UTC)
	sets, err := w.paramSetsForAt(triggerReq{kind: "cron"}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(sets) != 1 || sets[0]["startDate"] != "2026-08-21" || sets[0]["endDate"] != "2026-08-21" {
		t.Fatalf("sets=%#v, want yesterday as an exact one-day range", sets)
	}
}

func TestSingleDayWindowCrossesCalendarBoundaries(t *testing.T) {
	tests := []struct {
		name string
		now  time.Time
		want []string
	}{
		{name: "month", now: time.Date(2026, time.March, 2, 12, 0, 0, 0, time.UTC), want: []string{"2026-03-02", "2026-03-01", "2026-02-28"}},
		{name: "year", now: time.Date(2026, time.January, 2, 12, 0, 0, 0, time.UTC), want: []string{"2026-01-02", "2026-01-01", "2025-12-31"}},
		{name: "leap day", now: time.Date(2024, time.March, 1, 12, 0, 0, 0, time.UTC), want: []string{"2024-03-01", "2024-02-29", "2024-02-28"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := &EndpointWorker{Endpoint: config.Endpoint{WindowDays: len(tc.want), SingleDayWindow: true}}
			sets, err := w.paramSetsForAt(triggerReq{kind: "cron"}, tc.now)
			if err != nil {
				t.Fatal(err)
			}
			for i, want := range tc.want {
				if sets[i]["start_date"] != want || sets[i]["end_date"] != want {
					t.Fatalf("set %d=%#v, want %s for both bounds", i, sets[i], want)
				}
			}
		})
	}
}

func TestSingleDayWindowBuildsManualDailyRange(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{WindowDays: 2, SingleDayWindow: true}}
	sets, err := w.paramSetsFor(triggerReq{kind: "manual", dateFrom: "2026-08-01", dateTo: "2026-08-03"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"2026-08-01", "2026-08-02", "2026-08-03"}
	if len(sets) != len(want) {
		t.Fatalf("manual sets=%d", len(sets))
	}
	for i, date := range want {
		if sets[i]["start_date"] != date || sets[i]["end_date"] != date {
			t.Fatalf("set %d=%#v", i, sets[i])
		}
	}
}

func TestSingleDayWindowRequiresExplicitManualDate(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{WindowDays: 2, SingleDayWindow: true}}
	if _, err := w.paramSetsFor(triggerReq{kind: "manual"}); err == nil {
		t.Fatal("single-day endpoint accepted manual trigger without explicit date")
	}
}

func TestSingleDayWindowRejectsManualRangeOver92Days(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{WindowDays: 7, SingleDayWindow: true}}
	if sets, err := w.paramSetsFor(triggerReq{kind: "manual", dateFrom: "2024-01-01", dateTo: "2024-04-01"}); err != nil || len(sets) != 92 {
		t.Fatalf("92-day manual range rejected: sets=%d err=%v", len(sets), err)
	}
	if _, err := w.paramSetsFor(triggerReq{kind: "manual", dateFrom: "2024-01-01", dateTo: "2024-04-02"}); err == nil {
		t.Fatal("93-day manual range was accepted")
	}
}

func TestForEachParamSetStopsOnFirstFailure(t *testing.T) {
	visited := 0
	records, pages, ok := forEachParamSet([]map[string]any{{}, {}, {}}, func(map[string]any) (int, int, bool) {
		visited++
		return 1, 1, visited != 2
	})
	if ok || visited != 2 || records != 2 || pages != 2 {
		t.Fatalf("visited=%d records=%d pages=%d ok=%v", visited, records, pages, ok)
	}
}
