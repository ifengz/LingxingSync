package api

import (
	"encoding/json"
	"testing"
)

// TestApplyTopLevelTotal 锁定「顶层 total 兜底」契约。
//
// 历史 bug（静默截断，任务仍显示 success）：
// /erp/sc/data/mws/orders 把 data 返回成裸数组，总数放响应顶层：
//
//	{"code":0,"message":"操作成功","data":[...200 行...],"total":905}
//
// apiResponse 当时不收 total，parseFetchResult 又只拿得到 data，于是 Total=0；
// worker 的翻页判定在 has_more 缺失时看 offset+len>=total，Total=0 → 第 1 页终止，
// 905 条订单落库成 200 条，日志一片 success。
func TestApplyTopLevelTotal(t *testing.T) {
	tests := []struct {
		name          string
		dataTotal     int // parseFetchResult 从 data 里解出的 total
		topLevelTotal int // 响应顶层的 total
		want          int
	}{
		{
			name:          "data 裸数组无 total，用顶层 total（本次修的场景）",
			dataTotal:     0,
			topLevelTotal: 905,
			want:          905,
		},
		{
			name:          "data.total 存在时优先，不被顶层覆盖",
			dataTotal:     42,
			topLevelTotal: 905,
			want:          42,
		},
		{
			name:          "两处都没有 total，保持 0（不凭空推断）",
			dataTotal:     0,
			topLevelTotal: 0,
			want:          0,
		},
		{
			name:          "顶层 total 为 0 时不覆盖 data.total",
			dataTotal:     7,
			topLevelTotal: 0,
			want:          7,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &FetchResult{Total: tt.dataTotal}
			applyTopLevelTotal(r, tt.topLevelTotal)
			if r.Total != tt.want {
				t.Errorf("Total = %d, 期望 %d", r.Total, tt.want)
			}
		})
	}
}

// TestApiResponseCapturesTopLevelTotal 保证响应壳结构体真的会解出顶层 total。
// 这是上面兜底逻辑的前提：漏了 json tag 就等于没修。
func TestApiResponseCapturesTopLevelTotal(t *testing.T) {
	// 取自 /erp/sc/data/mws/orders 的真实响应形状（data 裸数组 + 顶层 total）。
	raw := []byte(`{"code":0,"message":"操作成功","data":[{"amazon_order_id":"112-1"}],` +
		`"total":905,"request_id":"abc","response_time":"2026-08-08 14:48:04","error_details":[]}`)

	var ar apiResponse
	if err := json.Unmarshal(raw, &ar); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ar.Total != 905 {
		t.Errorf("apiResponse.Total = %d, 期望 905（顶层 total 没被解出来）", ar.Total)
	}

	// 端到端串一遍：data 裸数组 → Total 应为顶层的 905。
	result, err := parseFetchResult(ar.Data)
	if err != nil {
		t.Fatalf("parseFetchResult: %v", err)
	}
	if result.Total != 0 {
		t.Fatalf("测试前提不成立：data 是裸数组，parseFetchResult 不该解出 total，实得 %d", result.Total)
	}
	applyTopLevelTotal(result, ar.Total)
	if result.Total != 905 {
		t.Errorf("兜底后 Total = %d，期望 905", result.Total)
	}
	if len(result.List) != 1 {
		t.Errorf("List 长度 = %d，期望 1", len(result.List))
	}
}
