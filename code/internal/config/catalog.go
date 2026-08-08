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

import "fmt"

// CatalogEntry 是一个「接口模板」：一条领星接口的完整接入合同，
// 除了「账号」这一个变量外，其余字段都已定死。字段语义与 Endpoint 对应字段一致。
type CatalogEntry struct {
	Key         string // 模板唯一标识（英文小写下划线），启用时拼进 Endpoint.Name
	Display     string // UI 展示名（中文）
	Summary     string // 一句话说明这个接口拉的是什么，给用户在清单里看
	Path        string // 领星 API Path
	Method      string // GET / POST
	Table       string // 目标表（必须已由 migrations 建好）
	RecordIDs   []string
	Rate        Rate
	DefaultCron string

	// 参数形态（三选一或组合，与 Endpoint 语义一致）
	WindowDays     int            // >0：注入 start_date/end_date 范围
	DateField      string         // 非空：注入单日期（如 event_date）
	DateOffsetDays int            // 单日期往前几天（0=今天，1=昨天）
	ExtraParams    map[string]any // 固定业务参数（如 {"type":1}）

	// 多店铺
	IsStoreSource  bool
	IterateByStore bool
	StoreParamName string
}

// ToEndpoint 用模板 + 账号 ID 生成一个可直接写入 config 的 Endpoint。
// 纯函数，不触碰任何共享状态。生成的 Name = "<key>_<accountID>"，保证同模板可用于
// 多个账号而不撞名；Enabled 默认 true（用户点了启用就是要它跑）。
//
// 重试无需在此设置：它是 worker 层的固定策略（网络/429/5xx 指数退避），不入 Endpoint 配置。
func (e CatalogEntry) ToEndpoint(accountID string) Endpoint {
	return Endpoint{
		Name:           e.Key + "_" + accountID,
		Display:        e.Display,
		Account:        accountID,
		Path:           e.Path,
		Method:         e.Method,
		Table:          e.Table,
		RecordIDFields: append([]string(nil), e.RecordIDs...),
		Rate:           e.Rate,
		Cron:           e.DefaultCron,
		Enabled:        true,
		WindowDays:     e.WindowDays,
		DateField:      e.DateField,
		DateOffsetDays: e.DateOffsetDays,
		ExtraParams:    e.ExtraParams,
		IsStoreSource:  e.IsStoreSource,
		IterateByStore: e.IterateByStore,
		StoreParamName: e.StoreParamName,
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
		Rate:          Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 0, Dimension: "account+path"},
		DefaultCron:   "0 */6 * * *",
		IsStoreSource: true,
	},
	{
		Key:     "sc_sales_orders",
		Display: "SC 销售订单",
		Summary: "亚马逊 FBA/FBM 销售订单，滚动近 7 天。",
		// path 已用真实账号跑通（probe 200 行）。注意不带 /openapi 前缀：
		// baseURL 本身就是 https://openapi.lingxing.com，早期误写成
		// "/openapi/erp/sc/orders/list" 会拼成 /openapi/openapi/... → 领星回 404。
		Path:      "/erp/sc/data/mws/orders",
		Method:    "POST",
		Table:     "ls_sales_orders",
		RecordIDs: []string{"amazon_order_id"}, // 领星返回的是 amazon_order_id，没有 order_id
		Rate:      Rate{Bucket: 5, IntervalMs: 200, MultiIntervalMs: 1000, Dimension: "account+path"},

		DefaultCron: "*/10 * * * *",
		WindowDays:  7, // → start_date/end_date，本接口必填
		// date_type=1 按订购时间【站点时间】筛选。早期误写 type=1，该接口无此参数。
		ExtraParams: map[string]any{"date_type": 1},
	},
	{
		Key:     "sc_inventory",
		Display: "SC FBA 库存",
		Summary: "亚马逊 FBA 在库/在途库存快照，全量。",
		// 与 006 迁移建的 ls_inventory 同源的真实路径（生产已跑通，单次 5079 行）。
		// 早期误写 "/openapi/erp/sc/inventory/list"（不存在），同 sc_sales_orders 的坑。
		Path:      "/erp/sc/routing/fba/fbaStock/fbaList",
		Method:    "GET",
		Table:     "ls_inventory",
		RecordIDs: []string{"sid", "fnsku"}, // 同一 fnsku 在多店铺各一行，必须带 sid
		Rate:      Rate{Bucket: 1, IntervalMs: 1000, MultiIntervalMs: 10000, Dimension: "account+path"},

		DefaultCron:    "0 */2 * * *",
		IterateByStore: true,
		StoreParamName: "sid",
	},
	// 「SC 广告日报」模板已移除，不是遗漏。原模板 path="/openapi/erp/sc/ads/daily"、
	// RecordIDs=["report_id"] 两者都不存在于领星 OpenAPI（凭空写的），谁点启用谁吃 404。
	// 本清单的契约是「路径和唯一键都已用真实账号验证过」，不能放没验证的条目。
	//
	// 领星真实的广告报表不是一个「日报」接口，而是一族（doc/core/08-api-reference.md §6.5）：
	//   POST /pb/openapi/newad/spProductAdReports      SP 商品报表（归因 7 天）
	//   POST /pb/openapi/newad/listHsaProductAdReport  SB 商品报表（归因 14 天）
	//   POST /pb/openapi/newad/sdProductAdReports      SD 商品报表（归因 14 天）
	// 还要先用 POST /basicOpen/baseData/account/list 取 profile_id 作参数。
	// 要接的话按 doc/core/07-add-endpoint.md 走：probe 摸字段 → 各建一张表 → 各加一条模板；
	// 现有 ls_ads_daily 表的列名（report_id/spend/sales/orders…）同样是臆造的，需一并重建。
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
