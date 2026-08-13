package api

import "testing"

// TestAnyToStringArrayJSON 锁住：数组/对象参数参与签名时按紧凑 JSON 编码，
// 而不是 fmt 的 "%v"（那会得到 "[1]" 导致签名与领星侧不一致）。
// 领星 VC 订单必填 purchase_order_type:["1"]、vc_store_ids 等都依赖这个行为。
func TestAnyToStringArrayJSON(t *testing.T) {
	cases := []struct {
		name string
		in   any
		want string
	}{
		{"string slice", []any{"1"}, `["1"]`},
		{"typed string slice", []string{"ATVPDHSKDCJ6R"}, `["ATVPDHSKDCJ6R"]`},
		{"multi slice", []any{"1", "2"}, `["1","2"]`},
		{"object", map[string]any{"k": "v"}, `{"k":"v"}`},
		// 标量路径不受影响
		{"plain string", "abc", "abc"},
		{"int", 7, "7"},
	}
	for _, c := range cases {
		if got := anyToString(c.in); got != c.want {
			t.Errorf("%s: anyToString(%v) = %q, want %q", c.name, c.in, got, c.want)
		}
	}
}
