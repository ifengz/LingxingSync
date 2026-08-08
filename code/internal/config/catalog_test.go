package config

import (
	"strings"
	"testing"
)

// TestCatalogToEndpoint 验证模板 → Endpoint 的字段映射：Name 拼进账号、Account 落位、
// 合同字段（path/method/table/唯一键/rate/cron/参数形态）如实复制。
func TestCatalogToEndpoint(t *testing.T) {
	e, err := FindCatalogEntry("sc_sales_orders")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	ep := e.ToEndpoint("sc_us")

	if ep.Name != "sc_sales_orders_sc_us" {
		t.Fatalf("Name = %q, want sc_sales_orders_sc_us", ep.Name)
	}
	if ep.Account != "sc_us" {
		t.Fatalf("Account = %q, want sc_us", ep.Account)
	}
	if !ep.Enabled {
		t.Fatal("生成的 Endpoint 应默认启用")
	}
	if ep.Path != e.Path || ep.Method != e.Method || ep.Table != e.Table {
		t.Fatalf("path/method/table 未如实复制: %+v", ep)
	}
	// amazon_order_id 是 /erp/sc/data/mws/orders 真实返回的订单号字段（probe 验证）。
	// 曾写成 order_id —— 领星不返回该字段，NOT NULL 主键写 NULL，接口永远落不了库。
	if len(ep.RecordIDFields) != 1 || ep.RecordIDFields[0] != "amazon_order_id" {
		t.Fatalf("record_id_fields = %v, want [amazon_order_id]", ep.RecordIDFields)
	}
	if ep.WindowDays != 7 {
		t.Fatalf("window_days = %d, want 7", ep.WindowDays)
	}

	// 改动生成结果的切片不应影响清单里的种子（ToEndpoint 必须深拷贝 RecordIDs）。
	ep.RecordIDFields[0] = "mutated"
	again, _ := FindCatalogEntry("sc_sales_orders")
	if again.RecordIDs[0] != "amazon_order_id" {
		t.Fatal("ToEndpoint 泄漏了种子切片，被调用方改动污染")
	}
}

// TestCatalogPathsHaveNoOpenapiPrefix 守卫清单里的 path 不带 /openapi 前缀。
//
// 历史 bug：baseURL 本身就是 https://openapi.lingxing.com，模板里又写
// "/openapi/erp/sc/orders/list"，拼出 /openapi/openapi/erp/... → 领星回
// {"code":500,"msg":"404 NOT_FOUND"}，sc_sales_orders 长期 0 记录。
// 这类错静默得很（HTTP 200 + 业务码 500），必须在测试层拦住。
func TestCatalogPathsHaveNoOpenapiPrefix(t *testing.T) {
	for _, e := range Catalog() {
		if strings.HasPrefix(e.Path, "/openapi/") {
			t.Errorf("模板 %q 的 path=%q 带了 /openapi 前缀：baseURL 已含该前缀，"+
				"会拼成 /openapi/openapi/... 导致 404", e.Key, e.Path)
		}
		if !strings.HasPrefix(e.Path, "/") {
			t.Errorf("模板 %q 的 path=%q 必须以 / 开头", e.Key, e.Path)
		}
	}
}

// TestCatalogEntriesHaveTargetTable 守卫每条模板都指定了目标表和唯一键。
// 清单的契约是「已验证过」，缺表名或缺唯一键的模板启用后启动即 FATAL。
func TestCatalogEntriesHaveTargetTable(t *testing.T) {
	for _, e := range Catalog() {
		if e.Table == "" {
			t.Errorf("模板 %q 缺 Table", e.Key)
		}
		if len(e.RecordIDs) == 0 {
			t.Errorf("模板 %q 缺 RecordIDs（唯一键）", e.Key)
		}
		if e.Method != "GET" && e.Method != "POST" {
			t.Errorf("模板 %q 的 Method=%q 非法", e.Key, e.Method)
		}
	}
}

// TestCatalogEntriesAllValid 保证每条种子模板生成的 Endpoint 都能通过 validate()——
// 即清单里不存在「启用后启动就 FATAL」的坏模板（缺唯一键、rate 非法、缺 cron 等）。
func TestCatalogEntriesAllValid(t *testing.T) {
	acc := Account{ID: "sc_us", Name: "US", AppKey: "k", AppSecret: "s"}
	for _, e := range Catalog() {
		cfg := &Config{
			Database:  Database{Host: "h", User: "u", DB: "d"},
			Accounts:  []Account{acc},
			Endpoints: []Endpoint{e.ToEndpoint("sc_us")},
		}
		if err := cfg.validate(); err != nil {
			t.Fatalf("种子模板 %q 生成的 Endpoint 未过校验: %v", e.Key, err)
		}
	}
}

// TestFindCatalogEntryUnknownFailsLoud 未知模板 key 必须报错，不静默返回空。
func TestFindCatalogEntryUnknownFailsLoud(t *testing.T) {
	if _, err := FindCatalogEntry("does_not_exist"); err == nil {
		t.Fatal("未知模板 key 应返回 error")
	}
}

// TestInventoryCatalogCarriesStoreType 固化 /sync 清单生成 SC 库存 Endpoint 时的店铺类型隔离。
func TestInventoryCatalogCarriesStoreType(t *testing.T) {
	e, err := FindCatalogEntry("sc_inventory")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	ep := e.ToEndpoint("sc_us")
	if !ep.IterateByStore || ep.StoreParamName != "sid" {
		t.Fatalf("库存模板未保留按店铺迭代合同: %+v", ep)
	}
	if ep.StoreType != "SC" {
		t.Fatalf("store_type = %q, want SC", ep.StoreType)
	}
}
