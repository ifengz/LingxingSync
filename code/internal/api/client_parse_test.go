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

func TestParseFetchResultObjectShape(t *testing.T) {
	r, err := parseFetchResultWithShape([]byte(`{"id":12610,"sku":"SKU-1"}`), "object")
	if err != nil {
		t.Fatalf("单对象响应不应报错: %v", err)
	}
	if len(r.List) != 1 || r.List[0]["sku"] != "SKU-1" {
		t.Fatalf("单对象应包装为一行，得到 %#v", r.List)
	}
	if r.Total != 1 || r.HasMorePresent || r.HasMore {
		t.Fatalf("单对象分页语义错误: %+v", r)
	}
}

func TestParseFetchResultObjectShapeRejectsPaginationMetadata(t *testing.T) {
	for _, data := range []string{
		`{"list":[],"total":0}`,
		`{"sku":"SKU-1","has_more":false}`,
	} {
		if _, err := parseFetchResultWithShape([]byte(data), "object"); err == nil {
			t.Fatalf("单对象模式不应吞掉分页对象: %s", data)
		}
	}
}

// TestSoftFailGuard 验证软失败闸：领星返回 code=0（看似成功）但 data=null 且 msg
// 带错误文案（如缺 summary_field）时，必须判为业务错误，不能静默记成成功 0 条。
// 这是 fail-loud 红线（CLAUDE.md §3 / 宪法 §5）在 client 层的补洞。
func TestSoftFailGuard(t *testing.T) {
	tests := []struct {
		name     string
		data     string // data 字段 JSON 原文
		msg      string // msg/message 文案
		wantSoft bool   // 是否应被软失败闸拦下
	}{
		{
			name:     "软失败: code0 + data:null + 错误文案（缺 summary_field）",
			data:     `null`,
			msg:      "[summary_field 不能为空,可取值asin,parent_asin,msku,sku]",
			wantSoft: true,
		},
		{
			name:     "正常: data:null + 空 msg（合法无数据）",
			data:     `null`,
			msg:      "",
			wantSoft: false,
		},
		{
			name:     "正常: data:null + msg=success",
			data:     `null`,
			msg:      "success",
			wantSoft: false,
		},
		{
			name:     "正常: data 有内容 + 有 msg（msg 非空但 data 非空不拦）",
			data:     `{"list":[{"a":1}]}`,
			msg:      "some note",
			wantSoft: false,
		},
		{
			name:     "正常: data:{} 空对象 + 错误文案（空对象不算空，不拦）",
			data:     `{}`,
			msg:      "whatever",
			wantSoft: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isEmptyRawData([]byte(tt.data)) && tt.msg != "" && !isSuccessMessage(tt.msg)
			if got != tt.wantSoft {
				t.Errorf("软失败判定 = %v, 期望 %v (data=%s msg=%q)", got, tt.wantSoft, tt.data, tt.msg)
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

// TestParseFetchResultHasMorePresent 锁定 HasMorePresent 语义：它区分「has_more
// 字段存在」与「不存在」，是 worker 选择终止策略（宪法 §4：has_more==false
// 还是 offset+len>=total）的依据。报表类接口只给 total 不给 has_more——present
// 必须为 false，worker 才会改走 total 判终止，不再首页即停（数据截断 bug 根因）。
func TestParseFetchResultHasMorePresent(t *testing.T) {
	tests := []struct {
		name        string
		data        string
		wantPresent bool
		wantHasMore bool
	}{
		{
			name:        "has_more:false 存在 → present=true",
			data:        `{"list":[{"a":1}],"total":2,"has_more":false}`,
			wantPresent: true,
			wantHasMore: false,
		},
		{
			name:        "has_more:true 存在 → present=true",
			data:        `{"list":[{"a":1}],"total":50,"has_more":true}`,
			wantPresent: true,
			wantHasMore: true,
		},
		{
			name:        "hasMore 驼峰形态也算存在",
			data:        `{"list":[{"a":1}],"total":50,"hasMore":true}`,
			wantPresent: true,
			wantHasMore: true,
		},
		{
			name:        "报表接口只给 total 不给 has_more → present=false（worker 改走 total 判终止）",
			data:        `{"list":[{"a":1}],"total":100}`,
			wantPresent: false,
			wantHasMore: false,
		},
		{
			name:        "裸数组无分页壳 → present=false",
			data:        `[{"a":1},{"a":2}]`,
			wantPresent: false,
			wantHasMore: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := parseFetchResult([]byte(tt.data))
			if err != nil {
				t.Fatalf("不期望报错: %v", err)
			}
			if r.HasMorePresent != tt.wantPresent {
				t.Errorf("HasMorePresent = %v, 期望 %v (data=%s)", r.HasMorePresent, tt.wantPresent, tt.data)
			}
			if r.HasMore != tt.wantHasMore {
				t.Errorf("HasMore = %v, 期望 %v (data=%s)", r.HasMore, tt.wantHasMore, tt.data)
			}
		})
	}
}
