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
