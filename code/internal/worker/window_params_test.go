package worker

import (
	"strconv"
	"testing"
	"time"

	"lingxing-sync/internal/config"
)

// TestBaseParamsWindowFieldNames 锁定「窗口参数名可配」机制。
//
// 历史 bug：baseParams 对 WindowDays>0 硬编码注入蛇形 start_date/end_date，
// 而领星 VC 报表族（/basicOpen/vc/report/{sales,realtimeSales,traffic,inventory}/list）
// 要的是驼峰 startDate/endDate —— 名字对不上，领星一律回 code=400「参数有误」，
// 4 个 VC 报表因此从未同步成功过。
//
// 修法是把参数名做成配置项（而不是在 worker 里按 path 写 if，那会违反
// CLAUDE.md §1.3「加接口零代码改动」）。本测试守住两点：
//  1. 不配 → 默认蛇形（既有配置不必逐条回填，向后兼容）
//  2. 配了 → 用配置的名字，且不再注入默认名（避免同时发两套日期参数）
func TestBaseParamsWindowFieldNames(t *testing.T) {
	tests := []struct {
		name       string
		startField string
		endField   string
		wantStart  string
		wantEnd    string
		absent     []string // 不应出现的键
	}{
		{
			name:      "默认：不配则蛇形 start_date/end_date",
			wantStart: "start_date",
			wantEnd:   "end_date",
			absent:    []string{"startDate", "endDate"},
		},
		{
			name:       "VC 报表：驼峰 startDate/endDate",
			startField: "startDate",
			endField:   "endDate",
			wantStart:  "startDate",
			wantEnd:    "endDate",
			absent:     []string{"start_date", "end_date"},
		},
		{
			name:       "只配起始：另一个仍走默认",
			startField: "startDate",
			wantStart:  "startDate",
			wantEnd:    "end_date",
			absent:     []string{"start_date"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := &EndpointWorker{Endpoint: config.Endpoint{
				WindowDays:       7,
				WindowStartField: tt.startField,
				WindowEndField:   tt.endField,
			}}
			params := w.baseParams()

			for _, key := range []string{tt.wantStart, tt.wantEnd} {
				v, ok := params[key]
				if !ok {
					t.Fatalf("缺少窗口参数 %q，实际参数=%v", key, params)
				}
				s, _ := v.(string)
				if _, err := time.Parse("2006-01-02", s); err != nil {
					t.Errorf("%s=%q 不是 YYYY-MM-DD: %v", key, s, err)
				}
			}
			for _, key := range tt.absent {
				if _, ok := params[key]; ok {
					t.Errorf("不应注入 %q（会同时发两套日期参数），实际参数=%v", key, params)
				}
			}
		})
	}
}

// TestBaseParamsNoWindowWhenZero WindowDays=0（全量接口）不应注入任何日期参数。
func TestBaseParamsNoWindowWhenZero(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		WindowDays:       0,
		WindowStartField: "startDate",
		WindowEndField:   "endDate",
	}}
	params := w.baseParams()
	for _, key := range []string{"start_date", "end_date", "startDate", "endDate"} {
		if _, ok := params[key]; ok {
			t.Errorf("WindowDays=0 不应注入 %q，实际参数=%v", key, params)
		}
	}
}

func TestBaseParamsUsesUnixSecondsForUnixWindowFields(t *testing.T) {
	w := &EndpointWorker{Endpoint: config.Endpoint{
		WindowDays:       30,
		WindowStartField: "start_time",
		WindowEndField:   "end_time",
	}}
	params := w.baseParams()
	for _, key := range []string{"start_time", "end_time"} {
		value, ok := params[key]
		if !ok {
			t.Fatalf("missing %q in params=%v", key, params)
		}
		seconds, ok := value.(string)
		if !ok {
			t.Fatalf("%q type=%T, want string", key, value)
		}
		if _, err := strconv.ParseInt(seconds, 10, 64); err != nil {
			t.Fatalf("%q=%q is not decimal Unix seconds: %v", key, seconds, err)
		}
	}
}
