package config

import "testing"

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
	if len(ep.RecordIDFields) != 1 || ep.RecordIDFields[0] != "order_id" {
		t.Fatalf("record_id_fields = %v, want [order_id]", ep.RecordIDFields)
	}
	if ep.WindowDays != 7 {
		t.Fatalf("window_days = %d, want 7", ep.WindowDays)
	}

	// 改动生成结果的切片不应影响清单里的种子（ToEndpoint 必须深拷贝 RecordIDs）。
	ep.RecordIDFields[0] = "mutated"
	again, _ := FindCatalogEntry("sc_sales_orders")
	if again.RecordIDs[0] != "order_id" {
		t.Fatal("ToEndpoint 泄漏了种子切片，被调用方改动污染")
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
