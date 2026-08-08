package worker

import (
	"testing"

	"lingxing-sync/internal/api"
)

// TestShouldContinuePaging 锁定宪法 §4 分页终止契约（doc/core/08-api-reference.md）：
// has_more==false 或 offset+length>=total 终止。历史 bug：worker 只认 has_more，
// 对「只给 total 不给 has_more」的报表类接口（VC 实时/销量报表、SC 销量统计）
// 在首页后就停，静默截断到 200 行。本测试是该修复的回归防线。
func TestShouldContinuePaging(t *testing.T) {
	tests := []struct {
		name     string
		hasMore  bool
		present  bool // has_more 字段是否出现在响应里
		total    int
		pageLen  int
		fetched  int // 含本页在内已累计记录数
		wantCont bool
	}{
		// —— 空页：一律停，防死循环 ——
		{"空页即停_即使has_more_true", true, true, 1000, 0, 0, false},
		{"空页即停_即使total未取满", false, false, 1000, 0, 200, false},

		// —— has_more 存在：以它为准（老接口行为不变）——
		{"has_more_true_继续", true, true, 50, 1, 1, true},
		{"has_more_false_停_即使total更大", false, true, 100, 1, 1, false},
		{"has_more_false_已取满_停", false, true, 2, 2, 2, false},

		// —— has_more 缺失但有 total：走 offset+len>=total（本次修复核心）——
		{"仅total_未取满_继续", false, false, 1000, 200, 200, true},
		{"仅total_恰好取满_停", false, false, 200, 200, 200, false},
		{"仅total_超量_停", false, false, 200, 200, 400, false},
		{"仅total_差一条_继续", false, false, 201, 200, 200, true},

		// —— 无任何分页信号（裸数组：has_more 缺失 + total<=0）：停在首页 ——
		{"无信号_停在首页", false, false, 0, 200, 200, false},
		{"total为0_停", false, false, 0, 5, 5, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &api.FetchResult{
				Total:          tt.total,
				HasMore:        tt.hasMore,
				HasMorePresent: tt.present,
			}
			got := shouldContinuePaging(r, tt.pageLen, tt.fetched)
			if got != tt.wantCont {
				t.Errorf("shouldContinuePaging(has_more=%v present=%v total=%d pageLen=%d fetched=%d) = %v, 期望 %v",
					tt.hasMore, tt.present, tt.total, tt.pageLen, tt.fetched, got, tt.wantCont)
			}
		})
	}
}
