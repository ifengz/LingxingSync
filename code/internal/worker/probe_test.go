package worker

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"lingxing-sync/internal/api"
)

// TestProbeSampleFitsTextColumn 锁定探测样本必须能写进 sync_task_logs.error_raw。
// 历史 bug：error_raw 是 MySQL TEXT（65535 字节上限），宽响应接口
// （/erp/sc/data/mws/orders，200 行 × 40 字段带 item_list）的原始 JSON 达几百 KB，
// INSERT 报 "Data too long" 被 worker 的 `_ = db.InsertTaskLog` 吞掉 ——
// 日志打印「原始 JSON 已存 task_logs」，库里却查不到这一行，探测白跑。
func TestProbeSampleFitsTextColumn(t *testing.T) {
	// 构造一个真实规模的宽响应：200 行，每行 40 个字段 + 嵌套 item_list。
	rows := make([]map[string]any, 0, 200)
	for i := 0; i < 200; i++ {
		row := map[string]any{
			"amazon_order_id": "112-3456789-0123456",
			"item_list": []any{
				map[string]any{"msku": "SKU-ABCDEFGH", "quantity": 3, "item_price": "29.99"},
				map[string]any{"msku": "SKU-IJKLMNOP", "quantity": 1, "item_price": "13.50"},
			},
		}
		for j := 0; j < 38; j++ {
			row["field_"+string(rune('a'+j%26))+string(rune('0'+j/26))] = "值-some-reasonably-long-value-0123456789"
		}
		rows = append(rows, row)
	}
	raw, err := json.Marshal(map[string]any{"code": 0, "data": rows, "total": 8000})
	if err != nil {
		t.Fatalf("构造 raw 失败: %v", err)
	}
	if len(raw) <= probeSampleMaxBytes {
		t.Fatalf("测试前提不成立：构造的 raw 只有 %d 字节，没超过上限 %d，测不到截断",
			len(raw), probeSampleMaxBytes)
	}

	got := probeSample(&api.FetchResult{List: rows, Raw: raw, Total: 8000})

	if len(got) > probeSampleMaxBytes {
		t.Errorf("probeSample 返回 %d 字节，超过上限 %d，会被 MySQL TEXT 拒收",
			len(got), probeSampleMaxBytes)
	}
	if !utf8.ValidString(got) {
		t.Error("probeSample 返回非法 UTF-8，utf8mb4 列会拒收整行")
	}
	// 探测的用途是读字段名，截断后仍必须带 fields 清单和首行样本。
	if !strings.Contains(got, "fields=") {
		t.Error("截断后丢了 fields= 字段名清单，探测失去意义")
	}
	if !strings.Contains(got, "amazon_order_id") {
		t.Error("截断后丢了 amazon_order_id，字段名读不出来")
	}
}

// TestProbeSampleShortResponseNotTruncated 短响应必须原样保留，不误伤。
func TestProbeSampleShortResponseNotTruncated(t *testing.T) {
	rows := []map[string]any{{"amazon_order_id": "112-0000001-0000001", "order_status": "Shipped"}}
	raw := []byte(`{"code":0,"data":[{"amazon_order_id":"112-0000001-0000001","order_status":"Shipped"}]}`)

	got := probeSample(&api.FetchResult{List: rows, Raw: raw, Total: 1})

	if strings.Contains(got, "[truncated]") {
		t.Error("短响应被误截断")
	}
	if !strings.Contains(got, string(raw)) {
		t.Error("短响应的原始 JSON 应完整保留")
	}
}

// TestTruncateUTF8RuneBoundary 截断点落在多字节字符中间时必须退到 rune 边界。
func TestTruncateUTF8RuneBoundary(t *testing.T) {
	// "领" 是 3 字节。构造让上限恰好切在它中间的输入。
	s := strings.Repeat("领", 40)
	for max := 20; max <= 60; max++ {
		got := truncateUTF8(s, max)
		if len(got) > max {
			t.Fatalf("max=%d: 返回 %d 字节，超限", max, len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("max=%d: 返回非法 UTF-8: %q", max, got)
		}
	}
}
