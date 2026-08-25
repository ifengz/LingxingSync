// Package config 的 catalog.go 实现「接口清单」（Catalog）——一批我校验过的
// 领星接口模板，让不懂技术的人也能安全加接口：从清单挑一个 → 选账号 → 点启用，
// 全程不碰 SQL / JSON / 表名 / 唯一键。
//
// 设计意图（CLAUDE.md §1「加接口极简单」的 UI 版）：
//   - 难的部分（path/method/唯一键/限流/参数形态/目标表）在这里一次性配死并测过；
//   - 用户侧只提供两个变量：选哪个模板、给哪个账号；
//   - 生成 Endpoint 复用现有写盘链路（applyConfigWrite：校验+备份+原子写+重启提示+
//     重复限流键拦截），不新增任何运行时机制（不违宪：无队列/锁/admission/watchdog）。
//
// 边界（诚实）：清单里没有的全新接口，仍需开发者先加一条模板 + 一张建表迁移才能出现。
// 没基础的人凭空定义新接口本就不安全，这条躲不掉——但常用接口可一次性都放进清单。
package config

import (
	"fmt"
	"maps"
	"slices"
)

// CatalogEntry 是一个「接口模板」：一条领星接口的完整接入合同，
// 除了「账号」这一个变量外，其余字段都已定死。字段语义与 Endpoint 对应字段一致。
type CatalogEntry struct {
	Key           string // 模板唯一标识（英文小写下划线），启用时拼进 Endpoint.Name
	Display       string // UI 展示名（中文）
	Summary       string // 一句话说明这个接口拉的是什么，给用户在清单里看
	Path          string // 领星 API Path
	Method        string // GET / POST
	Table         string // 目标表（必须已由 migrations 建好）
	RecordIDs     []string
	Rate          Rate
	DefaultCron   string
	ResponseShape string

	// 参数形态（三选一或组合，与 Endpoint 语义一致）
	WindowDays       int            // >0：窗口天数；single-day 时为逐日补偿天数
	SingleDayWindow  bool           // true：配置窗口或手工范围逐日拆成起止同日请求
	RowDateField     string         // 从实际起始参数注入 raw row
	WindowStartField string         // 空时默认 start_date
	WindowEndField   string         // 空时默认 end_date
	DateField        string         // 非空：注入单日期（如 event_date）
	DateOffsetDays   int            // 单日期往前几天（0=今天，1=昨天）
	ExtraParams      map[string]any // 固定业务参数（如 {"type":1}）
	RequestHeaders   map[string]string

	// 多店铺
	IsStoreSource        bool
	IterateByStore       bool
	StoreParamName       string
	StoreType            string
	IterateByVCOrders    bool
	IterateBySalesOrders bool

	// 广告账号迭代与落库前行整形。
	IterateByAdAccount bool
	AdAccountType      string
	FieldPaths         map[string]string
	InjectParams       []string
	ForceInjectParams  []string
}

// ToEndpoint 用模板 + 账号 ID 生成一个可直接写入 config 的 Endpoint。
// 纯函数，不触碰任何共享状态。生成的 Name = "<key>_<accountID>"，保证同模板可用于
// 多个账号而不撞名；Enabled 默认 true（用户点了启用就是要它跑）。
//
// 重试无需在此设置：它是 worker 层的固定策略（网络/429/5xx 指数退避），不入 Endpoint 配置。
func (e CatalogEntry) ToEndpoint(accountID string) Endpoint {
	return Endpoint{
		Name:                 e.Key + "_" + accountID,
		Display:              e.Display,
		Account:              accountID,
		Path:                 e.Path,
		Method:               e.Method,
		Table:                e.Table,
		RecordIDFields:       slices.Clone(e.RecordIDs),
		ResponseShape:        e.ResponseShape,
		Rate:                 e.Rate,
		Cron:                 e.DefaultCron,
		Enabled:              true,
		WindowDays:           e.WindowDays,
		SingleDayWindow:      e.SingleDayWindow,
		RowDateField:         e.RowDateField,
		WindowStartField:     e.WindowStartField,
		WindowEndField:       e.WindowEndField,
		DateField:            e.DateField,
		DateOffsetDays:       e.DateOffsetDays,
		ExtraParams:          maps.Clone(e.ExtraParams),
		RequestHeaders:       maps.Clone(e.RequestHeaders),
		IsStoreSource:        e.IsStoreSource,
		IterateByStore:       e.IterateByStore,
		StoreParamName:       e.StoreParamName,
		StoreType:            e.StoreType,
		IterateByVCOrders:    e.IterateByVCOrders,
		IterateBySalesOrders: e.IterateBySalesOrders,
		IterateByAdAccount:   e.IterateByAdAccount,
		AdAccountType:        e.AdAccountType,
		FieldPaths:           maps.Clone(e.FieldPaths),
		InjectParams:         slices.Clone(e.InjectParams),
		ForceInjectParams:    slices.Clone(e.ForceInjectParams),
	}
}

// catalogEntries 是内置清单（种子）。每条都对应一张已在 migrations 建好的表。
//
// ⚠️ 新增模板前必须先有它的目标表（migrations/00N_*.sql），否则启用后重启时
// worker.New → GetTableColumns 找不到表会 FATAL。这是 fail-loud 的启动断言。
var catalogEntries = []CatalogEntry{
	{
		Key:           "sc_stores",
		Display:       "SC 店铺列表",
		Summary:       "亚马逊店铺清单，是所有「按店铺」接口的数据来源，建议第一个启用。",
		Path:          "/erp/sc/data/seller/lists",
		Method:        "GET",
		Table:         "ls_stores",
		RecordIDs:     []string{"sid"},
		Rate:          Rate{Bucket: 5, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:   "*/30 * * * *",
		IsStoreSource: true,
	},
	{
		Key:     "sc_inventory",
		Display: "SC FBA 库存",
		Summary: "亚马逊 FBA 在库/在途库存快照，全量。",
		// 与 006 迁移建的 ls_fba_inventory 同源的真实路径（生产已跑通，单次 5079 行）。
		// 早期误写 "/openapi/erp/sc/inventory/list"（不存在）。
		Path:      "/erp/sc/routing/fba/fbaStock/fbaList",
		Method:    "GET",
		Table:     "ls_fba_inventory",
		RecordIDs: []string{"sid", "fnsku"}, // 同一 fnsku 在多店铺各一行，必须带 sid
		Rate:      Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 10000, Dimension: "account+path"},

		DefaultCron:    "0 */2 * * *",
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
	},
	{
		Key:           "vc_stores",
		Display:       "VC 店铺列表",
		Summary:       "供应商中心店铺清单，启用后可在店铺目录填写广告 Profile ID。",
		Path:          "/basicOpen/platformAuth/vcSeller/pageList",
		Method:        "POST",
		Table:         "ls_stores",
		RecordIDs:     []string{"sid"},
		Rate:          Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:   "*/30 * * * *",
		IsStoreSource: true,
		StoreType:     "VC",
	},
	{
		Key:            "sc_listing",
		Display:        "SC Listing",
		Summary:        "Seller Central Listing 原始清单，用于产品与 ASIN/MSKU 配对。",
		Path:           "/erp/sc/data/mws/listing",
		Method:         "POST",
		Table:          "ls_sc_listing",
		RecordIDs:      []string{"sid", "seller_sku"},
		Rate:           Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 1000, Dimension: "account+path"},
		DefaultCron:    "*/30 * * * *",
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
	},
	{
		Key:         "sc_products",
		Display:     "SC 产品列表",
		Summary:     "SC 本地库存产品主档原始列表。",
		Path:        "/erp/sc/routing/data/local_inventory/productList",
		Method:      "POST",
		Table:       "ls_sc_products",
		RecordIDs:   []string{"sku"},
		Rate:        Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron: "*/30 * * * *",
	},
	{
		Key:            "sc_sales_report",
		Display:        "SC 销量报表（ASIN 日报）",
		Summary:        "按 SC 店铺覆盖同步前两天的 ASIN 日销量。",
		Path:           "/erp/sc/data/sales_report/asinDailyLists",
		Method:         "POST",
		Table:          "ls_sc_sales_report",
		RecordIDs:      []string{"sid", "r_date", "asin"},
		Rate:           Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 1000, Dimension: "account+path"},
		DefaultCron:    "*/30 * * * *",
		DateField:      "event_date",
		DateOffsetDays: 2,
		ExtraParams:    map[string]any{"type": 2, "asin_type": 1},
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
	},
	{
		Key:            "sc_sales_revenue",
		Display:        "SC 销售额报表（ASIN 日报）",
		Summary:        "按 SC 店铺覆盖同步前两天的 ASIN 日销售额。",
		Path:           "/erp/sc/data/sales_report/asinDailyLists",
		Method:         "POST",
		Table:          "ls_sc_sales_revenue",
		RecordIDs:      []string{"sid", "r_date", "asin"},
		Rate:           Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 1000, Dimension: "account+path"},
		DefaultCron:    "*/30 * * * *",
		DateField:      "event_date",
		DateOffsetDays: 2,
		ExtraParams:    map[string]any{"type": 1, "asin_type": 1},
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
	},
	{
		Key:         "vc_orders",
		Display:     "VC PO 订单",
		Summary:     "同步近 7 天 VC PO 订单。",
		Path:        "/basicOpen/platformOrder/vcOrder/pageList",
		Method:      "POST",
		Table:       "ls_vc_orders",
		RecordIDs:   []string{"vc_store_id", "local_po_number"},
		Rate:        Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 1000, Dimension: "account+path"},
		DefaultCron: "*/30 * * * *",
		WindowDays:  7,
		ExtraParams: map[string]any{
			"search_field_time":   "3",
			"purchase_order_type": []string{"1"},
		},
	},
	{
		Key:               "vc_po_details",
		Display:           "VC PO 订单详情",
		Summary:           "从同账号 VC PO 列表读取近 7 天订单号，逐单保存详情头与 items 原始 JSON。",
		Path:              "/basicOpen/platformOrder/vcOrderPo/detail",
		Method:            "POST",
		Table:             "ls_vc_po_details",
		RecordIDs:         []string{"vc_store_id", "local_po_number"},
		Rate:              Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:       "*/30 * * * *",
		ResponseShape:     "object",
		WindowDays:        7,
		IterateByVCOrders: true,
		ForceInjectParams: []string{"vc_store_id", "local_po_number"},
	},
	{
		Key:              "vc_sales_report",
		Display:          "VC 销量报表",
		Summary:          "按 VC 店铺覆盖同步近 7 天 ASIN 日销量。",
		Path:             "/basicOpen/vc/report/sales/list",
		Method:           "POST",
		Table:            "ls_vc_sales_report",
		RecordIDs:        []string{"sid", "asin", "date"},
		Rate:             Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:      "*/30 * * * *",
		WindowDays:       7,
		WindowStartField: "startDate",
		WindowEndField:   "endDate",
		ExtraParams:      map[string]any{"view": "sourcing"},
		IterateByStore:   true,
		StoreParamName:   "sid",
		StoreType:        "VC",
		InjectParams:     []string{"sid"},
	},
	{
		Key:              "vc_realtime_sales",
		Display:          "VC 实时销量报表",
		Summary:          "按 VC 店铺覆盖同步最近一天的小时销量。",
		Path:             "/basicOpen/vc/report/realtimeSales/list",
		Method:           "POST",
		Table:            "ls_vc_realtime_sales",
		RecordIDs:        []string{"sid", "asin", "startTime"},
		Rate:             Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:      "*/30 * * * *",
		WindowDays:       1,
		WindowStartField: "startDate",
		WindowEndField:   "endDate",
		IterateByStore:   true,
		StoreParamName:   "sid",
		StoreType:        "VC",
		InjectParams:     []string{"sid"},
	},
	{
		Key:               "vc_traffic",
		Display:           "VC 流量报表",
		Summary:           "按 VC 店铺同步近 7 天 ASIN 浏览量。",
		Path:              "/basicOpen/vc/report/traffic/list",
		Method:            "POST",
		Table:             "ls_vc_traffic",
		RecordIDs:         []string{"sid", "asin", "date"},
		Rate:              Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:       "0 5 * * *",
		WindowDays:        7,
		WindowStartField:  "startDate",
		WindowEndField:    "endDate",
		IterateByStore:    true,
		StoreParamName:    "sid",
		StoreType:         "VC",
		ForceInjectParams: []string{"sid"},
	},
	{
		Key:               "vc_inventory",
		Display:           "VC 库存报表",
		Summary:           "按 VC 店铺同步日维 ASIN 库存快照。",
		Path:              "/basicOpen/vc/report/inventory/list",
		Method:            "POST",
		Table:             "ls_vc_inventory",
		RecordIDs:         []string{"sid", "date", "asin"},
		Rate:              Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:       "0 5 * * *",
		WindowDays:        1,
		SingleDayWindow:   true,
		DateOffsetDays:    1,
		WindowStartField:  "startDate",
		WindowEndField:    "endDate",
		ExtraParams:       map[string]any{"view": "sourcing"},
		IterateByStore:    true,
		StoreParamName:    "sid",
		StoreType:         "VC",
		ForceInjectParams: []string{"sid"},
	},
	{
		Key:             "sc_performance",
		Display:         "SC 产品表现（ASIN）",
		Summary:         "按 SC 店铺逐日补偿最近 7 天的 ASIN 日维产品表现。",
		Path:            "/bd/productPerformance/openApi/asinList",
		Method:          "POST",
		Table:           "ls_sc_performance_daily",
		RecordIDs:       []string{"sid", "asin", "business_date"},
		Rate:            Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 10000, Dimension: "account+path"},
		DefaultCron:     "0 5 * * *",
		WindowDays:      7,
		SingleDayWindow: true,
		RowDateField:    "business_date",
		ExtraParams:     map[string]any{"summary_field": "asin", "sort_field": "amount", "sort_type": "desc"},
		IterateByStore:  true,
		StoreParamName:  "sid",
		StoreType:       "SC",
		FieldPaths:      map[string]string{"asin": "asins[0].asin"},
		InjectParams:    []string{"sid"},
	},
	{
		Key:               "vc_margin",
		Display:           "VC 毛利日报",
		Summary:           "按 VC 店铺同步近 7 天 ASIN 毛利数据。",
		Path:              "/basicOpen/vc/report/nppm/list",
		Method:            "POST",
		Table:             "ls_vc_margin",
		RecordIDs:         []string{"sid", "asin", "date"},
		Rate:              Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:       "0 5 * * *",
		WindowDays:        7,
		WindowStartField:  "startDate",
		WindowEndField:    "endDate",
		IterateByStore:    true,
		StoreParamName:    "sid",
		StoreType:         "VC",
		ForceInjectParams: []string{"sid"},
	},
	{
		Key:            "sc_refunds",
		Display:        "SC FBA 退货订单",
		Summary:        "按 SC 店铺同步近 7 天 FBA 退货订单。",
		Path:           "/erp/sc/data/mws_report/refundOrders",
		Method:         "POST",
		Table:          "ls_sc_refunds",
		RecordIDs:      []string{"sid", "license_plate_number"},
		Rate:           Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:    "0 5 * * *",
		WindowDays:     7,
		ExtraParams:    map[string]any{"date_type": 1},
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
		InjectParams:   []string{"sid"},
	},
	{
		Key:            "sc_removal_orders",
		Display:        "SC 退仓订单",
		Summary:        "按 SC 店铺同步近 30 天退仓订单与退仓配送地址；报表按 seller_id 维度返回。",
		Path:           "/erp/sc/routing/data/order/removalOrderListNew",
		Method:         "POST",
		Table:          "ls_sc_removal_orders",
		RecordIDs:      []string{"seller_id", "order_id", "sku", "fnsku", "disposition"},
		Rate:           Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:    "0 5 * * *",
		WindowDays:     30,
		ExtraParams:    map[string]any{"search_field_time": "last_updated_date"},
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
	},
	{
		Key:            "sc_fba_order_addresses",
		Display:        "SC FBA 订单地址",
		Summary:        "按 SC 店铺同步近 30 天 FBA 订单的城市、州、邮编和国家。",
		Path:           "/erp/sc/data/mws_report/fbaOrders",
		Method:         "POST",
		Table:          "ls_sc_fba_order_addresses",
		RecordIDs:      []string{"sid", "shipment_id", "shipment_item_id"},
		Rate:           Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:    "0 5 * * *",
		WindowDays:     30,
		ExtraParams:    map[string]any{"date_type": 1},
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
		InjectParams:   []string{"sid"},
	},
	{
		Key:              "mp_fbm_orders",
		Display:          "多平台 FBM 订单",
		Summary:          "按更新时间同步多平台自发货订单；商品、地址和平台数组保留在原始订单 JSON 中。",
		Path:             "/pb/mp/order/v2/list",
		Method:           "POST",
		Table:            "ls_mp_fbm_orders",
		RecordIDs:        []string{"store_id", "global_order_no"},
		Rate:             Rate{Bucket: 10, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:      "*/30 * * * *",
		WindowDays:       30,
		WindowStartField: "start_time",
		WindowEndField:   "end_time",
		ExtraParams:      map[string]any{"date_type": "update_time"},
	},
	{
		Key:         "mp_store_mappings",
		Display:     "多平台店铺映射",
		Summary:     "同步多平台 store_id、店铺名、站点和可用 sid 映射。",
		Path:        "/pb/mp/shop/v2/getSellerList",
		Method:      "POST",
		Table:       "ls_mp_store_mappings",
		RecordIDs:   []string{"store_id"},
		Rate:        Rate{Bucket: 10, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron: "*/30 * * * *",
	},
	{
		Key:            "sc_sales_orders",
		Display:        "SC 亚马逊订单列表",
		Summary:        "按 SC 店铺和订单修改时间同步订单头；为订单详情提供候选订单号。",
		Path:           "/erp/sc/data/mws/orders",
		Method:         "POST",
		Table:          "ls_sales_orders",
		RecordIDs:      []string{"amazon_order_id"},
		Rate:           Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 1000, Dimension: "account+path"},
		DefaultCron:    "*/30 * * * *",
		WindowDays:     30,
		ExtraParams:    map[string]any{"date_type": 2},
		IterateByStore: true,
		StoreParamName: "sid",
		StoreType:      "SC",
		InjectParams:   []string{"sid"},
	},
	{
		Key:                  "sc_order_details",
		Display:              "SC 亚马逊订单详情",
		Summary:              "从同账号订单列表取候选订单号，批量同步订单及商品原始详情。",
		Path:                 "/erp/sc/data/mws/orderDetail",
		Method:               "POST",
		Table:                "ls_sc_order_details",
		RecordIDs:            []string{"sid", "amazon_order_id"},
		Rate:                 Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:          "*/30 * * * *",
		ResponseShape:        "list",
		WindowDays:           30,
		IterateBySalesOrders: true,
	},
	{
		Key:         "ad_accounts",
		Display:     "广告账号",
		Summary:     "广告报表的 profile_id 来源，须先同步。",
		Path:        "/basicOpen/baseData/account/list",
		Method:      "POST",
		Table:       "ls_ad_accounts",
		RecordIDs:   []string{"profile_id"},
		Rate:        Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron: "0 5 * * *",
		ExtraParams: map[string]any{"type": "seller"},
	},
	{
		Key:                "ad_sp_product",
		Display:            "SP 商品广告报表",
		Summary:            "按有效广告账号同步 SP 商品广告日报。",
		Path:               "/pb/openapi/newad/spProductAdReports",
		Method:             "POST",
		Table:              "ls_ad_sp_product",
		RecordIDs:          []string{"sid", "profile_id", "report_date", "ad_id"},
		Rate:               Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:        "0 5 * * *",
		DateField:          "report_date",
		DateOffsetDays:     2,
		ExtraParams:        map[string]any{"show_detail": 0},
		RequestHeaders:     map[string]string{"X-API-VERSION": "2"},
		IterateByAdAccount: true,
		AdAccountType:      "seller",
		ForceInjectParams:  []string{"sid", "profile_id"},
	},
	{
		Key:                "ad_sp_campaign",
		Display:            "SP 活动广告报表",
		Summary:            "按有效广告账号同步 SP 活动广告日报。",
		Path:               "/pb/openapi/newad/spCampaignReports",
		Method:             "POST",
		Table:              "ls_ad_sp_campaign",
		RecordIDs:          []string{"sid", "profile_id", "report_date", "campaign_id"},
		Rate:               Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:        "0 5 * * *",
		DateField:          "report_date",
		DateOffsetDays:     2,
		ExtraParams:        map[string]any{"show_detail": 0},
		IterateByAdAccount: true,
		AdAccountType:      "seller",
		ForceInjectParams:  []string{"sid", "profile_id"},
	},
	{
		Key:                "ad_sd_product",
		Display:            "SD 商品广告报表",
		Summary:            "按有效广告账号同步 SD 商品广告日报。",
		Path:               "/pb/openapi/newad/sdProductAdReports",
		Method:             "POST",
		Table:              "ls_ad_sd_product",
		RecordIDs:          []string{"sid", "profile_id", "report_date", "ad_id"},
		Rate:               Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:        "0 5 * * *",
		DateField:          "report_date",
		DateOffsetDays:     2,
		ExtraParams:        map[string]any{"show_detail": 0},
		RequestHeaders:     map[string]string{"X-API-VERSION": "2"},
		IterateByAdAccount: true,
		AdAccountType:      "seller",
		ForceInjectParams:  []string{"sid", "profile_id"},
	},
	{
		Key:                "ad_sd_campaign",
		Display:            "SD 活动广告报表",
		Summary:            "按有效 seller 广告账号同步 SD 活动广告日报。",
		Path:               "/pb/openapi/newad/sdCampaignReports",
		Method:             "POST",
		Table:              "ls_ad_sd_campaign",
		RecordIDs:          []string{"sid", "profile_id", "report_date", "campaign_id"},
		Rate:               Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:        "0 5 * * *",
		DateField:          "report_date",
		DateOffsetDays:     2,
		ExtraParams:        map[string]any{"show_detail": 0},
		IterateByAdAccount: true,
		AdAccountType:      "seller",
		ForceInjectParams:  []string{"sid", "profile_id"},
	},
	{
		Key:                "ad_hsa_campaign",
		Display:            "HSA 活动广告报表",
		Summary:            "按有效 seller 广告账号同步 HSA 活动广告日报。",
		Path:               "/pb/openapi/newad/hsaCampaignReports",
		Method:             "POST",
		Table:              "ls_ad_hsa_campaign",
		RecordIDs:          []string{"sid", "profile_id", "report_date", "campaign_id"},
		Rate:               Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:        "0 5 * * *",
		DateField:          "report_date",
		DateOffsetDays:     2,
		IterateByAdAccount: true,
		AdAccountType:      "seller",
		ForceInjectParams:  []string{"sid", "profile_id"},
	},
	// 「SC 广告日报」模板已移除，不是遗漏。原模板 path="/openapi/erp/sc/ads/daily"、
	// RecordIDs=["report_id"] 两者都不存在于领星 OpenAPI（凭空写的），谁点启用谁吃 404。
	// 本清单的契约是「路径和唯一键都已用真实账号验证过」，不能放没验证的条目。
	//
	// 广告数据按「广告账号 + 各报表」拆开：上方 ad_accounts 先同步 profile_id，
	// 其后各报表才按有效广告账号迭代。旧的单一广告日报 path/主键都是臆造的，
	// 不得重新加入清单。
}

// Catalog 返回内置清单的拷贝（防止调用方改动内部种子）。
func Catalog() []CatalogEntry {
	out := make([]CatalogEntry, len(catalogEntries))
	copy(out, catalogEntries)
	return out
}

// FindCatalogEntry 按 key 找模板，找不到返回 error（fail-loud，避免启用一个不存在的模板）。
func FindCatalogEntry(key string) (CatalogEntry, error) {
	for _, e := range catalogEntries {
		if e.Key == key {
			return e, nil
		}
	}
	return CatalogEntry{}, fmt.Errorf("接口模板不存在: %s", key)
}
