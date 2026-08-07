package api

import "testing"

// TestParseFetchResultFailLoud 验证 parseFetchResult 在领星返回格式异常时报错，
// 不静默兜底成 0 行假成功。这是 fail-loud 红线（CLAUDE.md §3 / 宪法 §5）的核心防线。
func TestParseFetchResultFailLoud(t *testing.T) {
	tests := []struct {
		name    string
		data    string // data 字段的 JSON 原文
		wantErr bool   // 是否期望报错
		listLen int    // 期望 List 长度（wantErr=false 时校验）
		hasMore bool
	}{
		{
			name:    "标准分页响应 list+total+has_more",
			data:    `{"list":[{"a":1},{"a":2}],"total":2,"has_more":false}`,
			wantErr: false,
			listLen: 2,
			hasMore: false,
		},
		{
			name:    "has_more=true 正常翻页",
			data:    `{"list":[{"a":1}],"total":50,"has_more":true}`,
			wantErr: false,
			listLen: 1,
			hasMore: true,
		},
		{
			name:    "data 为 null（合法空响应）",
			data:    `null`,
			wantErr: false,
			listLen: 0,
		},
		{
			name:    "data 为空对象（合法无数据）",
			data:    `{}`,
			wantErr: false,
			listLen: 0,
		},
		{
			name:    "data 是数组（少数接口直接返回数组）",
			data:    `[{"a":1},{"a":2}]`,
			wantErr: false,
			listLen: 2,
			hasMore: false,
		},
		{
			name:    "FAIL-LOUD: 对象但缺 list 字段（领星改字段名必须报错，不能静默吞成0行）",
			data:    `{"records":[1,2],"total":2}`,
			wantErr: true,
		},
		{
			name:    "FAIL-LOUD: data 是字符串（非对象非数组）",
			data:    `"unexpected"`,
			wantErr: true,
		},
		{
			name:    "FAIL-LOUD: data 是数字（非对象非数组）",
			data:    `12345`,
			wantErr: true,
		},
		{
			name:    "FAIL-LOUD: list 存在但不是数组（类型错误）",
			data:    `{"list":"notarray","total":0}`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := parseFetchResult([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("期望报错但 parseFetchResult 返回 nil err，List=%v（fail-loud 失守）", r.List)
				}
				return
			}
			if err != nil {
				t.Fatalf("不期望报错但 parseFetchResult 返回 err: %v", err)
			}
			if len(r.List) != tt.listLen {
				t.Errorf("List 长度 = %d, 期望 %d (data=%s)", len(r.List), tt.listLen, tt.data)
			}
			if r.HasMore != tt.hasMore {
				t.Errorf("HasMore = %v, 期望 %v (data=%s)", r.HasMore, tt.hasMore, tt.data)
			}
		})
	}
}

// TestParseFetchResultNoFieldGuessing 验证不再用 records/items/rows/data 等别名猜测 list。
// 领星若改字段名，这里必须报错暴露问题，而不是猜中后假装正常。
func TestParseFetchResultNoFieldGuessing(t *testing.T) {
	guessNames := []string{"records", "items", "rows", "data"}
	for _, alias := range guessNames {
		t.Run("别名 "+alias+" 不应被猜测为 list", func(t *testing.T) {
			data := `{"` + alias + `":[{"x":1}],"total":1}`
			_, err := parseFetchResult([]byte(data))
			if err == nil {
				t.Fatalf("parseFetchResult 猜中了别名 %s 并返回 nil err——fail-loud 失守", alias)
			}
		})
	}
}

// TestParseFetchResultNoHasMoreInference 验证不再用 len<total 近似推断 has_more。
// 推断可能导致：total 偏大时多翻页撞限流，或 total 偏小时漏数据。
// 现在无 has_more 字段就视为无更多，由显式字段说了算。
func TestParseFetchResultNoHasMoreInference(t *testing.T) {
	// 有 total=100 但无 has_more 字段：旧逻辑会推断 has_more=true，现在必须为 false
	data := `{"list":[{"a":1}],"total":100}`
	r, err := parseFetchResult([]byte(data))
	if err != nil {
		t.Fatalf("正常对象不应报错: %v", err)
	}
	if r.HasMore {
		t.Fatal("无 has_more 字段时不应推断为 true（旧 len<total 近似推断已被移除）")
	}
}
