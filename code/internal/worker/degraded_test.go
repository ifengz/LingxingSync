// degraded_test.go 覆盖「表结构不匹配时只坏这一个接口」的判定逻辑。
//
// 被测的是纯函数 missingDeclaredColumns（不需要 DB）：配置声明了某些列、
// 目标表实际没有 → 这些字段会被 Upsert 静默丢弃，必须被识别出来做成可见告警。
// 缺表那条路（fatalErr）需要真实 DB 连接，不在单测覆盖范围，见回复里的说明。
package worker

import (
	"reflect"
	"testing"

	"lingxing-sync/internal/config"
)

func TestMissingDeclaredColumns(t *testing.T) {
	cases := []struct {
		name string
		ep   config.Endpoint
		cols []string
		want []string
	}{
		{
			name: "全部声明列都存在时无告警",
			ep: config.Endpoint{
				RecordIDFields: []string{"order_id", "sku"},
				FieldPaths:     map[string]string{"asin": "asins[0].asin"},
			},
			cols: []string{"account_id", "order_id", "sku", "asin", "synced_at"},
			want: nil,
		},
		{
			name: "唯一键列缺失（去重语义可能已错，但同步照跑）",
			ep:   config.Endpoint{RecordIDFields: []string{"order_id", "sku"}},
			cols: []string{"account_id", "order_id"},
			want: []string{"sku"},
		},
		{
			name: "field_paths 目标列缺失（捞出来又被丢掉）",
			ep:   config.Endpoint{FieldPaths: map[string]string{"asin": "asins[0].asin"}},
			cols: []string{"account_id", "order_id"},
			want: []string{"asin"},
		},
		{
			name: "两类声明同时缺，结果排序去重",
			ep: config.Endpoint{
				RecordIDFields: []string{"sku", "order_id"},
				FieldPaths:     map[string]string{"asin": "a[0].b", "sku": "x.sku"},
			},
			cols: []string{"account_id", "order_id"},
			want: []string{"asin", "sku"}, // sku 在两处都声明，只报一次
		},
		{
			name: "probe 接口跳过（表本来就没建）",
			ep: config.Endpoint{
				Probe:          true,
				RecordIDFields: []string{"order_id"},
			},
			cols: []string{"account_id"},
			want: nil,
		},
		{
			name: "空列集合跳过（缺表已由 fatalErr 覆盖，不重复报）",
			ep:   config.Endpoint{RecordIDFields: []string{"order_id"}},
			cols: nil,
			want: nil,
		},
		{
			name: "空字符串声明被忽略，不误报",
			ep:   config.Endpoint{RecordIDFields: []string{"", "order_id"}},
			cols: []string{"account_id", "order_id"},
			want: nil,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := missingDeclaredColumns(c.ep, c.cols)
			if len(got) == 0 && len(c.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Fatalf("missingDeclaredColumns() = %v, 期望 %v", got, c.want)
			}
		})
	}
}
