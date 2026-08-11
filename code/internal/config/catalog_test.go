package config

import (
	"reflect"
	"strings"
	"testing"
)

// TestCatalogToEndpoint 验证模板 → Endpoint 的字段映射：Name 拼进账号、Account 落位、
// 合同字段（path/method/table/唯一键/rate/cron/参数形态）如实复制。
func TestCatalogToEndpoint(t *testing.T) {
	e, err := FindCatalogEntry("sc_inventory")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	ep := e.ToEndpoint("sc_us")

	if ep.Name != "sc_inventory_sc_us" {
		t.Fatalf("Name = %q, want sc_inventory_sc_us", ep.Name)
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
	if len(ep.RecordIDFields) != 2 || ep.RecordIDFields[0] != "sid" || ep.RecordIDFields[1] != "fnsku" {
		t.Fatalf("record_id_fields = %v, want [sid fnsku]", ep.RecordIDFields)
	}
	if !ep.IterateByStore || ep.StoreParamName != "sid" {
		t.Fatalf("SC 库存模板未保留按店铺同步合同: %+v", ep)
	}
	if ep.StoreType != "SC" {
		t.Fatalf("store_type = %q, want SC", ep.StoreType)
	}

	// 改动生成结果的切片不应影响清单里的种子（ToEndpoint 必须深拷贝 RecordIDs）。
	ep.RecordIDFields[0] = "mutated"
	again, _ := FindCatalogEntry("sc_inventory")
	if again.RecordIDs[0] != "sid" {
		t.Fatal("ToEndpoint 泄漏了种子切片，被调用方改动污染")
	}
}

func TestSCFBAInventoryCatalogTable(t *testing.T) {
	e, err := FindCatalogEntry("sc_inventory")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	if e.Table != "ls_fba_inventory" {
		t.Fatalf("SC FBA 库存目标表 = %q, want ls_fba_inventory", e.Table)
	}
}

func TestVCStoresCatalogContract(t *testing.T) {
	e, err := FindCatalogEntry("vc_stores")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	ep := e.ToEndpoint("sc_us_2")
	if ep.Path != "/basicOpen/platformAuth/vcSeller/pageList" || ep.Method != "POST" || ep.Table != "ls_stores" {
		t.Fatalf("VC 店铺来源合同错误: %+v", ep)
	}
	if len(ep.RecordIDFields) != 1 || ep.RecordIDFields[0] != "sid" {
		t.Fatalf("record_id_fields = %v, want [sid]", ep.RecordIDFields)
	}
	if !ep.IsStoreSource || ep.StoreType != "VC" {
		t.Fatalf("VC 店铺来源标记错误: %+v", ep)
	}
}

// TestCatalogSCListingContract 固化真实 probe 已验证的 SC Listing 接入合同。
// seller_sku 在两账号样本中均非空；listing_id 存在空值，不能替代唯一键。
func TestCatalogSCListingContract(t *testing.T) {
	e, err := FindCatalogEntry("sc_listing")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	ep := e.ToEndpoint("sc_us")

	if ep.Path != "/erp/sc/data/mws/listing" || ep.Method != "POST" || ep.Table != "ls_sc_listing" {
		t.Fatalf("SC Listing path/method/table 合同错误: %+v", ep)
	}
	if len(ep.RecordIDFields) != 2 || ep.RecordIDFields[0] != "sid" || ep.RecordIDFields[1] != "seller_sku" {
		t.Fatalf("record_id_fields = %v, want [sid seller_sku]", ep.RecordIDFields)
	}
	if !ep.IterateByStore || ep.StoreParamName != "sid" || ep.StoreType != "SC" {
		t.Fatalf("SC Listing 按 SC 店铺迭代合同错误: %+v", ep)
	}
}

// TestCatalogSCProductsContract 固化 productList 两账号全量审计后的原始表合同。
func TestCatalogSCProductsContract(t *testing.T) {
	e, err := FindCatalogEntry("sc_products")
	if err != nil {
		t.Fatalf("FindCatalogEntry: %v", err)
	}
	ep := e.ToEndpoint("sc_us")

	if ep.Path != "/erp/sc/routing/data/local_inventory/productList" || ep.Method != "POST" || ep.Table != "ls_sc_products" {
		t.Fatalf("SC 产品列表 path/method/table 合同错误: %+v", ep)
	}
	if len(ep.RecordIDFields) != 1 || ep.RecordIDFields[0] != "sku" {
		t.Fatalf("record_id_fields = %v, want [sku]", ep.RecordIDFields)
	}
	if ep.IterateByStore {
		t.Fatal("SC 产品列表不按店铺迭代，应按账号全量分页")
	}
}

// 016-022 已完成真实请求、落库验证；除需账号专属 VC 店铺 ID 的 vc_listing 外，
// 其余模板必须能在部署后的清单中按账号直接启用，不能依赖本地 ignored config.yaml。
func TestCatalogIncludesVerifiedSyncTemplates(t *testing.T) {
	tests := []struct {
		key      string
		path     string
		table    string
		advanced func(Endpoint) bool
	}{
		{
			key: "vc_margin", path: "/basicOpen/vc/report/nppm/list", table: "ls_vc_margin",
			advanced: func(ep Endpoint) bool {
				return ep.IterateByStore && ep.StoreType == "VC" && ep.WindowStartField == "startDate" &&
					ep.WindowEndField == "endDate" && reflect.DeepEqual(ep.ForceInjectParams, []string{"sid"})
			},
		},
		{
			key: "sc_refunds", path: "/erp/sc/data/mws_report/refundOrders", table: "ls_sc_refunds",
			advanced: func(ep Endpoint) bool {
				return ep.IterateByStore && ep.StoreType == "SC" && reflect.DeepEqual(ep.InjectParams, []string{"sid"})
			},
		},
		{
			key: "ad_accounts", path: "/basicOpen/baseData/account/list", table: "ls_ad_accounts",
			advanced: func(ep Endpoint) bool { return ep.ExtraParams["type"] == "seller" },
		},
		{
			key: "ad_sp_product", path: "/pb/openapi/newad/spProductAdReports", table: "ls_ad_sp_product",
			advanced: func(ep Endpoint) bool {
				return ep.IterateByAdAccount && ep.AdAccountType == "seller" && ep.RequestHeaders["X-API-VERSION"] == "2" &&
					ep.DateField == "report_date" && ep.DateOffsetDays == 2 && reflect.DeepEqual(ep.ForceInjectParams, []string{"sid", "profile_id"})
			},
		},
		{
			key: "ad_sp_campaign", path: "/pb/openapi/newad/spCampaignReports", table: "ls_ad_sp_campaign",
			advanced: func(ep Endpoint) bool {
				return ep.IterateByAdAccount && ep.AdAccountType == "seller" && ep.DateField == "report_date" &&
					ep.DateOffsetDays == 2 && reflect.DeepEqual(ep.ForceInjectParams, []string{"sid", "profile_id"})
			},
		},
		{
			key: "ad_sd_product", path: "/pb/openapi/newad/sdProductAdReports", table: "ls_ad_sd_product",
			advanced: func(ep Endpoint) bool {
				return ep.IterateByAdAccount && ep.AdAccountType == "seller" && ep.RequestHeaders["X-API-VERSION"] == "2" &&
					ep.DateField == "report_date" && ep.DateOffsetDays == 2 && reflect.DeepEqual(ep.ForceInjectParams, []string{"sid", "profile_id"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			e, err := FindCatalogEntry(tc.key)
			if err != nil {
				t.Fatalf("FindCatalogEntry(%q): %v", tc.key, err)
			}
			ep := e.ToEndpoint("sc_us")
			if ep.Path != tc.path || ep.Table != tc.table || !tc.advanced(ep) {
				t.Fatalf("template contract = %#v", ep)
			}
		})
	}
}

func TestCatalogIncludesVerifiedReportTemplates(t *testing.T) {
	tests := []struct {
		key      string
		path     string
		table    string
		ids      []string
		contract func(Endpoint) bool
	}{
		{
			key: "sc_sales_report", path: "/erp/sc/data/sales_report/asinDailyLists", table: "ls_sc_sales_report",
			ids: []string{"sid", "r_date", "asin"},
			contract: func(ep Endpoint) bool {
				return ep.Cron == "*/30 * * * *" && ep.DateField == "event_date" && ep.DateOffsetDays == 2 &&
					ep.IterateByStore && ep.StoreType == "SC" && ep.ExtraParams["type"] == 2
			},
		},
		{
			key: "sc_sales_revenue", path: "/erp/sc/data/sales_report/asinDailyLists", table: "ls_sc_sales_revenue",
			ids: []string{"sid", "r_date", "asin"},
			contract: func(ep Endpoint) bool {
				return ep.Cron == "*/30 * * * *" && ep.DateField == "event_date" && ep.DateOffsetDays == 2 &&
					ep.IterateByStore && ep.StoreType == "SC" && ep.ExtraParams["type"] == 1
			},
		},
		{
			key: "vc_orders", path: "/basicOpen/platformOrder/vcOrder/pageList", table: "ls_vc_orders",
			ids: []string{"vc_store_id", "local_po_number"},
			contract: func(ep Endpoint) bool {
				return ep.Cron == "*/30 * * * *" && ep.WindowDays == 7
			},
		},
		{
			key: "vc_sales_report", path: "/basicOpen/vc/report/sales/list", table: "ls_vc_sales_report",
			ids: []string{"sid", "asin", "date"},
			contract: func(ep Endpoint) bool {
				return ep.Cron == "*/30 * * * *" && ep.IterateByStore && ep.StoreType == "VC" &&
					reflect.DeepEqual(ep.InjectParams, []string{"sid"})
			},
		},
		{
			key: "vc_realtime_sales", path: "/basicOpen/vc/report/realtimeSales/list", table: "ls_vc_realtime_sales",
			ids: []string{"sid", "asin", "startTime"},
			contract: func(ep Endpoint) bool {
				return ep.Cron == "*/30 * * * *" && ep.IterateByStore && ep.StoreType == "VC" &&
					reflect.DeepEqual(ep.InjectParams, []string{"sid"})
			},
		},
		{
			key: "vc_traffic", path: "/basicOpen/vc/report/traffic/list", table: "ls_vc_traffic",
			ids: []string{"sid", "asin", "date"},
			contract: func(ep Endpoint) bool {
				return ep.IterateByStore && ep.StoreType == "VC" &&
					reflect.DeepEqual(ep.ForceInjectParams, []string{"sid"})
			},
		},
		{
			key: "vc_inventory", path: "/basicOpen/vc/report/inventory/list", table: "ls_vc_inventory",
			ids: []string{"sid", "date", "asin"},
			contract: func(ep Endpoint) bool {
				return ep.WindowDays == 1 && ep.IterateByStore && ep.StoreType == "VC" &&
					ep.ExtraParams["view"] == "sourcing" &&
					reflect.DeepEqual(ep.ForceInjectParams, []string{"sid"})
			},
		},
		{
			key: "sc_performance", path: "/bd/productPerformance/openApi/asinList", table: "ls_sc_performance",
			ids: []string{"sid", "asin"},
			contract: func(ep Endpoint) bool {
				return ep.IterateByStore && ep.StoreType == "SC" && ep.FieldPaths["asin"] == "asins[0].asin" &&
					reflect.DeepEqual(ep.InjectParams, []string{"sid"})
			},
		},
		{
			key: "ad_sd_campaign", path: "/pb/openapi/newad/sdCampaignReports", table: "ls_ad_sd_campaign",
			ids: []string{"sid", "profile_id", "report_date", "campaign_id"},
			contract: func(ep Endpoint) bool {
				return ep.IterateByAdAccount && ep.AdAccountType == "seller" && ep.DateField == "report_date" &&
					ep.ExtraParams["show_detail"] == 0 &&
					reflect.DeepEqual(ep.ForceInjectParams, []string{"sid", "profile_id"})
			},
		},
		{
			key: "ad_hsa_campaign", path: "/pb/openapi/newad/hsaCampaignReports", table: "ls_ad_hsa_campaign",
			ids: []string{"sid", "profile_id", "report_date", "campaign_id"},
			contract: func(ep Endpoint) bool {
				_, hasShowDetail := ep.ExtraParams["show_detail"]
				return ep.IterateByAdAccount && ep.AdAccountType == "seller" && ep.DateField == "report_date" &&
					!hasShowDetail && reflect.DeepEqual(ep.ForceInjectParams, []string{"sid", "profile_id"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			e, err := FindCatalogEntry(tc.key)
			if err != nil {
				t.Fatalf("FindCatalogEntry(%q): %v", tc.key, err)
			}
			ep := e.ToEndpoint("sc_us")
			if ep.Path != tc.path || ep.Table != tc.table || !reflect.DeepEqual(ep.RecordIDFields, tc.ids) || !tc.contract(ep) {
				t.Fatalf("template contract = %#v", ep)
			}
		})
	}
}

func TestCatalogVCPODetailsContract(t *testing.T) {
	e, err := FindCatalogEntry("vc_po_details")
	if err != nil {
		t.Fatalf("FindCatalogEntry(vc_po_details): %v", err)
	}
	ep := e.ToEndpoint("sc_us_1")
	if ep.Path != "/basicOpen/platformOrder/vcOrderPo/detail" || ep.Method != "POST" || ep.Table != "ls_vc_po_details" {
		t.Fatalf("VC PO detail path/method/table contract = %#v", ep)
	}
	if !reflect.DeepEqual(ep.RecordIDFields, []string{"vc_store_id", "local_po_number"}) {
		t.Fatalf("record_id_fields = %v, want [vc_store_id local_po_number]", ep.RecordIDFields)
	}
	if ep.ResponseShape != "object" || ep.WindowDays != 7 || !ep.IterateByVCOrders {
		t.Fatalf("VC PO detail request contract = %#v", ep)
	}
	if !reflect.DeepEqual(ep.ForceInjectParams, []string{"vc_store_id", "local_po_number"}) {
		t.Fatalf("force_inject_params = %v", ep.ForceInjectParams)
	}
}

func TestCatalogSalesQuantityAndRevenueVariantsCoexist(t *testing.T) {
	quantity, err := FindCatalogEntry("sc_sales_report")
	if err != nil {
		t.Fatal(err)
	}
	revenue, err := FindCatalogEntry("sc_sales_revenue")
	if err != nil {
		t.Fatal(err)
	}
	cfg := &Config{
		Database: Database{Host: "h", User: "u", DB: "d"},
		Accounts: []Account{{ID: "sc_us", AppKey: "key", AppSecret: "secret"}},
		Endpoints: []Endpoint{
			quantity.ToEndpoint("sc_us"),
			revenue.ToEndpoint("sc_us"),
		},
	}
	if err := cfg.validate(); err != nil {
		t.Fatalf("verified sales metric variants must remain independent raw lanes: %v", err)
	}
}

func TestHalfHourlyCatalogDefaults(t *testing.T) {
	for _, key := range []string{"sc_stores", "vc_stores", "sc_listing", "sc_products"} {
		e, err := FindCatalogEntry(key)
		if err != nil {
			t.Fatalf("FindCatalogEntry(%q): %v", key, err)
		}
		if e.DefaultCron != "*/30 * * * *" {
			t.Errorf("%s DefaultCron = %q, want */30 * * * *", key, e.DefaultCron)
		}
	}
}

func TestDeletedSalesOrdersCatalogEntry(t *testing.T) {
	if _, err := FindCatalogEntry("sc_sales_orders"); err == nil {
		t.Fatal("sc_sales_orders 已确认删除，不应重新出现在接口清单")
	}
}

// TestCatalogIncludesStockAndAddressReadInterfaces 固化两个生产 probe 已验证的只读合同。
// 退仓订单必须保留 seller_id 维度；FBA 订单响应不回 sid，必须从请求参数回填。
func TestCatalogIncludesStockAndAddressReadInterfaces(t *testing.T) {
	tests := []struct {
		key      string
		path     string
		table    string
		ids      []string
		contract func(Endpoint) bool
	}{
		{
			key: "sc_removal_orders", path: "/erp/sc/routing/data/order/removalOrderListNew", table: "ls_sc_removal_orders",
			ids: []string{"seller_id", "order_id", "sku", "fnsku", "disposition"},
			contract: func(ep Endpoint) bool {
				return ep.IterateByStore && ep.StoreType == "SC" && ep.WindowDays == 30 &&
					ep.ExtraParams["search_field_time"] == "last_updated_date"
			},
		},
		{
			key: "sc_fba_order_addresses", path: "/erp/sc/data/mws_report/fbaOrders", table: "ls_sc_fba_order_addresses",
			ids: []string{"sid", "shipment_id", "shipment_item_id"},
			contract: func(ep Endpoint) bool {
				return ep.IterateByStore && ep.StoreType == "SC" && ep.WindowDays == 30 &&
					ep.ExtraParams["date_type"] == 1 && reflect.DeepEqual(ep.InjectParams, []string{"sid"})
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			e, err := FindCatalogEntry(tc.key)
			if err != nil {
				t.Fatalf("FindCatalogEntry(%q): %v", tc.key, err)
			}
			ep := e.ToEndpoint("sc_us")
			if ep.Path != tc.path || ep.Method != "POST" || ep.Table != tc.table ||
				!reflect.DeepEqual(ep.RecordIDFields, tc.ids) || !tc.contract(ep) {
				t.Fatalf("template contract = %#v", ep)
			}
		})
	}
}

// TestCatalogPathsHaveNoOpenapiPrefix 守卫清单里的 path 不带 /openapi 前缀。
//
// 历史 bug：baseURL 本身就是 https://openapi.lingxing.com，模板里又写
// "/openapi/erp/sc/orders/list"，拼出 /openapi/openapi/erp/... → 领星回
// {"code":500,"msg":"404 NOT_FOUND"}，历史订单接口长期 0 记录。
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
