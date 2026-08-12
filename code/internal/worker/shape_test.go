package worker

import (
	"strings"
	"testing"
)

// TestShapeRowsNoConfig 未配任何整形项 → 行原样不动（向后兼容既有 21 个接口）。
func TestShapeRowsNoConfig(t *testing.T) {
	rows := []map[string]any{{"a": 1}}
	if err := shapeRows(rows, nil, nil, nil, map[string]any{"sid": "12534"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(rows[0]) != 1 || rows[0]["a"] != 1 {
		t.Fatalf("行被意外改动: %v", rows[0])
	}
}

// TestShapeRowsNestedArrayIndex 覆盖 sc_performance 的真实形状：
// 顶层没有 asin，唯一键埋在 asins[0].asin。
func TestShapeRowsNestedArrayIndex(t *testing.T) {
	rows := []map[string]any{{
		"amount": "27024.79",
		"asins": []any{
			map[string]any{"asin": "B0FGDQ72GG", "sid": "12534"},
		},
	}}
	err := shapeRows(rows, map[string]string{"asin": "asins[0].asin"}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rows[0]["asin"] != "B0FGDQ72GG" {
		t.Fatalf("asin 未提到顶层: %v", rows[0]["asin"])
	}
}

// TestShapeRowsInjectParams 领星不回显请求参数 sid，需从 params 补进行。
func TestShapeRowsInjectParams(t *testing.T) {
	rows := []map[string]any{{"asin": "B01"}, {"asin": "B02"}}
	err := shapeRows(rows, nil, []string{"sid"}, nil, map[string]any{"sid": "12534", "offset": 0})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	for i, r := range rows {
		if r["sid"] != "12534" {
			t.Fatalf("行 %d 未注入 sid: %v", i, r["sid"])
		}
		if _, ok := r["offset"]; ok {
			t.Fatal("只应注入 inject_params 列出的参数，offset 不该进行")
		}
	}
}

func TestShapeRowsInjectsBusinessDateFromSentStartDate(t *testing.T) {
	rows := []map[string]any{{"asin": "B01"}}
	err := injectRowDate(rows, "business_date", "start_date", map[string]any{"start_date": "2026-08-10", "end_date": "2026-08-10"})
	if err != nil {
		t.Fatal(err)
	}
	if rows[0]["business_date"] != "2026-08-10" {
		t.Fatalf("business_date = %#v", rows[0]["business_date"])
	}
}

// TestShapeRowsInjectDoesNotOverwrite 领星真回显了该字段时，以领星的值为准。
// 防止「请求参数悄悄覆盖真实响应」这类静默改数据。
func TestShapeRowsInjectDoesNotOverwrite(t *testing.T) {
	rows := []map[string]any{{"sid": "99999"}}
	if err := shapeRows(rows, nil, []string{"sid"}, nil, map[string]any{"sid": "12534"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rows[0]["sid"] != "99999" {
		t.Fatalf("领星回显值被请求参数覆盖了: %v", rows[0]["sid"])
	}
}

// TestShapeRowsForceInjectPreservesRequestID
// 上游把 18 位 sid 当 JSON number 返回时，解码后的末位可能已变化；
// 强制回注必须恢复本页请求使用的精确字符串。
func TestShapeRowsForceInjectPreservesRequestID(t *testing.T) {
	rows := []map[string]any{{"sid": float64(134618505906074620)}}
	if err := shapeRows(rows, nil, nil, []string{"sid"}, map[string]any{"sid": "134618505906074624"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got := rows[0]["sid"]; got != "134618505906074624" {
		t.Fatalf("force inject 未恢复请求 sid: %#v", got)
	}
}

// TestShapeRowsInjectFillsEmptyString 空串视同缺失（领星常用 "" 表示无值）。
func TestShapeRowsInjectFillsEmptyString(t *testing.T) {
	rows := []map[string]any{{"sid": ""}}
	if err := shapeRows(rows, nil, []string{"sid"}, nil, map[string]any{"sid": "12534"}); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rows[0]["sid"] != "12534" {
		t.Fatalf("空串未被补上: %v", rows[0]["sid"])
	}
}

// TestShapeRowsPathTypoFailsLoud 路径语法合法、但整页一行都取不到 → 报错。
// 判据是「整页零命中」，因为那几乎只可能是配置写错了键名（如 asinz[0].asin），
// 而不是数据恰好都缺。这条防线很重要：不报错的话，每行都静默留空，最后一路撞到
// "Column 'asin' cannot be null"，错误信息离病根隔着好几层——正是 sc_sales_report
// 那个 bug 让人查了半天的原因。
func TestShapeRowsPathTypoFailsLoud(t *testing.T) {
	rows := []map[string]any{
		{"amount": "1", "asins": []any{map[string]any{"asin": "B01"}}},
		{"amount": "2", "asins": []any{map[string]any{"asin": "B02"}}},
	}
	// 键名写错（asinz），语法合法，但整页零命中。
	err := shapeRows(rows, map[string]string{"asin": "asinz[0].asin"}, nil, nil, nil)
	if err == nil {
		t.Fatal("键名写错却静默通过了；期望整页零命中时 fail-loud")
	}
	if !strings.Contains(err.Error(), "asinz[0].asin") {
		t.Errorf("报错里应带上出问题的路径，便于直接定位配置；实际: %v", err)
	}
}

// TestShapeRowsSparseRowsTolerated 路径本身是对的，只是部分行没有这个嵌套值 →
// 不报错，best-effort 填能填的。
//
// 为什么不按行严格：整页只要有一行命中，就证明路径配对了。此时个别行缺值是领星的
// 数据稀疏，不是配置错。若按行 fail，领星返回一条异常行就能让整个接口的同步全挂，
// 比它要防的问题更糟。真正的身份缺失由目标表的 NOT NULL 主键约束兜住——那层
// 报错带表名带列名，本来就够清楚。
func TestShapeRowsSparseRowsTolerated(t *testing.T) {
	rows := []map[string]any{
		{"amount": "1", "asins": []any{map[string]any{"asin": "B01"}}}, // 命中
		{"amount": "2", "asins": []any{}},                              // 空数组
		{"amount": "3"},                                                // 整个键缺失
		{"amount": "4", "asins": []any{map[string]any{"other": "x"}}},  // 叶子键不存在
	}
	if err := shapeRows(rows, map[string]string{"asin": "asins[0].asin"}, nil, nil, nil); err != nil {
		t.Fatalf("路径已有命中行，个别行缺值不该报错: %v", err)
	}
	if got := rows[0]["asin"]; got != "B01" {
		t.Errorf("命中行应填上 asin，实际 %#v", got)
	}
	for i := 1; i < len(rows); i++ {
		if v, ok := rows[i]["asin"]; ok && !isBlank(v) {
			t.Errorf("第 %d 行取不到值，不该凭空造值，实际 %#v", i, v)
		}
	}
}

// TestShapeRowsExistingTopLevelWins 顶层已有该字段时不覆盖（对齐 polabel2
// performanceAsin：先取顶层，缺失才退到嵌套）。
func TestShapeRowsExistingTopLevelWins(t *testing.T) {
	rows := []map[string]any{{
		"asin":  "TOP",
		"asins": []any{map[string]any{"asin": "NESTED"}},
	}}
	if err := shapeRows(rows, map[string]string{"asin": "asins[0].asin"}, nil, nil, nil); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rows[0]["asin"] != "TOP" {
		t.Fatalf("顶层值被嵌套值覆盖: %v", rows[0]["asin"])
	}
}

// TestShapeRowsPlainNestedObject 不带下标的纯对象路径也要支持。
func TestShapeRowsPlainNestedObject(t *testing.T) {
	rows := []map[string]any{{
		"available_inventory": map[string]any{"afn_fulfillable_quantity": float64(20)},
	}}
	err := shapeRows(rows, map[string]string{
		"afn_fulfillable_quantity": "available_inventory.afn_fulfillable_quantity",
	}, nil, nil, nil)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if rows[0]["afn_fulfillable_quantity"] != float64(20) {
		t.Fatalf("嵌套对象取值失败: %v", rows[0]["afn_fulfillable_quantity"])
	}
}
