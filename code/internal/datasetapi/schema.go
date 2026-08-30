package datasetapi

import (
	"fmt"
	"sort"
	"strings"
)

type Column struct {
	Name     string
	SQLType  string
	Nullable bool
}

type Schema struct {
	DatasetID  string
	TableName  string
	DataNote   string
	Columns    []Column
	PrimaryKey []string
}

var schemas = map[string]Schema{
	"listing-daily-v1": {
		DatasetID: "listing-daily-v1", TableName: "listing_daily_v1",
		DataNote: "日维数据；同一业务日期按店铺、渠道、ASIN、SKU 保留一行。",
		Columns: append(schemaColumns([]Column{
			{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "channel", SQLType: "VARCHAR(32)", Nullable: false},
			{Name: "asin", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "sku", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "business_date", SQLType: "DATE", Nullable: false},
			{Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}, {Name: "is_provisional", SQLType: "TINYINT(1)", Nullable: false}, {Name: "verification_status", SQLType: "VARCHAR(32)", Nullable: false},
		}), listingBusinessColumns()...),
		PrimaryKey: []string{"store", "channel", "asin", "sku", "business_date"},
	},
	"return-reason-detail-v1": {
		DatasetID: "return-reason-detail-v1", TableName: "return_reason_detail_v1", DataNote: "退货原因明细；按退货记录稳定键增量读取。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"return-reason-detail-v2": {
		DatasetID: "return-reason-detail-v2", TableName: "return_reason_detail_v2", DataNote: "退货原因明细 v2；补充站点日期和上游更新时间。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"order-shipping-address-detail-v1": {
		DatasetID: "order-shipping-address-detail-v1", TableName: "order_shipping_address_detail_v1", DataNote: "订单配送地址行明细；原始接口只提供城市、州、省、邮编和国家等字段。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"address-order-item-detail-v1": {
		DatasetID: "address-order-item-detail-v1", TableName: "address_order_item_detail_v1", DataNote: "FBA 订单配送商品行；marketplace 由店铺接口的 marketplace_id 映射为 US/AU/JP，fulfillment_channel 固定为 FBA。ship_lat/ship_lng 为历史兼容字段，保持 NULL。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"fbm-address-order-item-detail-v1": {
		DatasetID: "fbm-address-order-item-detail-v1", TableName: "fbm_address_order_item_detail_v1", DataNote: "FBM 订单商品配送明细；商品稳定键直接使用领星 global_item_no，地址来自订单级 address_info，坐标上游不提供保持 NULL。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"address-order-item-detail-v2": {
		DatasetID: "address-order-item-detail-v2", TableName: "address_order_item_detail_v2", DataNote: "订单配送商品行 v2；补充地址原始列。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"fba-inventory-snapshot-v1": {
		DatasetID: "fba-inventory-snapshot-v1", TableName: "fba_inventory_snapshot_v1", DataNote: "FBA 库存每日快照；历史从本版本部署后每次成功同步开始累计，不补造部署前日期。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"fba-inventory-snapshot-v2": {
		DatasetID: "fba-inventory-snapshot-v2", TableName: "fba_inventory_snapshot_v2", DataNote: "FBA 库存快照 v2；补充已验证原始库存列。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-po-detail-v1": {
		DatasetID: "vc-po-detail-v1", TableName: "vc_po_detail_v1", DataNote: "VC PO 逐单详情；一行一个店铺/本地 PO，record_date 使用 raw 详情同步时间的日期，items 保留上游 JSON，不拆造商品行。",
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-po-lines-v1": {
		DatasetID: "vc-po-lines-v1", TableName: "vc_po_lines_v1", DataNote: "VC PO 商品行；从已同步详情的 items 数组按已确认字段展开，一行一个商品，不改变原始 PO JSON。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"fba-links-v1": {
		DatasetID: "fba-links-v1", TableName: "fba_links_v1", DataNote: "FBA Links 页面固定行；销售/广告窗口来自 listing_daily_metrics，库存和 Listing 身份来自 SC 原始表。",
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-links-v1": {
		DatasetID: "vc-links-v1", TableName: "vc_links_v1", DataNote: "VC Links 页面固定行；按 VC 店铺和 ASIN 汇总已同步的 VC Listing、销售、实时销售、库存、流量和毛利事实。",
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-inventory-daily-v1": {
		DatasetID: "vc-inventory-daily-v1", TableName: "vc_inventory_daily_v1", DataNote: "VC 库存日维事实；只发布领星库存报表实际返回的数量、率、交付天数、成本和币种。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-ad-daily-v1": {
		DatasetID: "vc-ad-daily-v1", TableName: "vc_ad_daily_v1", DataNote: "VC 广告 profile 事实；store 查询参数代表 profile_id，HSA/SB 无 ASIN 时保留 profile_unattributed。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-traffic-daily-v1": {
		DatasetID: "vc-traffic-daily-v1", TableName: "vc_traffic_daily_v1", DataNote: "VC 流量日表；仅发布 raw ls_vc_traffic 提供的 glanceViews，桌面/移动流量不臆造。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"sc-account-ad-daily-v1": {
		DatasetID: "sc-account-ad-daily-v1", TableName: "sc_account_ad_daily_v1", DataNote: "SC 账户广告日表；按店铺、日期、广告类型汇总 SP/SD/HSA campaign raw。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"sc-account-ad-daily-v2": {
		DatasetID: "sc-account-ad-daily-v2", TableName: "sc_account_ad_daily_v2", DataNote: "SC 账户广告日表 v2；继承 v1 字段并允许下游读取已验证的 currency。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-realtime-v1": {
		DatasetID: "vc-realtime-v1", TableName: "vc_realtime_sales_v1", DataNote: "VC 实时销量；按原始小时窗口发布，业务日期取 localStartTime，缺失时保持 NULL。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"vc-listing-metrics-snapshot-v1": {
		DatasetID: "vc-listing-metrics-snapshot-v1", TableName: "vc_listing_metrics_snapshot_v1", DataNote: "VC Listing 当前态快照；日期为最近同步日期，raw 表不保留历史版本。",
		Columns:    detailSchemaColumns([]Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}),
		PrimaryKey: []string{"store", "stable_key"},
	},
	"operations-log-v1": {
		DatasetID: "operations-log-v1", TableName: "operations_log_v1", DataNote: "运营日志可由领星事实提供的日维指标；跟踪目标、备注和人工历史仍属于 polabel2 本地业务。",
		PrimaryKey: []string{"store", "stable_key"},
	},
	"operations-log-v2": {
		DatasetID: "operations-log-v2", TableName: "operations_log_v2", DataNote: "运营日志事实 v2；identity_scope 标识店铺级或已明确分摊的商品级事实，verified_fields 是报告实际核验字段的 JSON 对象。",
		PrimaryKey: []string{"store", "stable_key"},
	},
	"operations-log-v3": {
		DatasetID: "operations-log-v3", TableName: "operations_log_v3", DataNote: "运营日志事实 v3；保留 v2 身份和核验语义，并发布各广告类型的曝光和点击。",
		PrimaryKey: []string{"store", "stable_key"},
	},
}

func schemaColumns(columns []Column) []Column { return append([]Column(nil), columns...) }

func detailSchemaColumns(fixed []Column) []Column {
	return append([]Column(nil), fixed...)
}

func listingBusinessColumns() []Column {
	columns := make([]Column, 0, len(availableSchemaFields))
	for _, name := range availableSchemaFields {
		columns = append(columns, Column{Name: name, SQLType: schemaType(name), Nullable: true})
	}
	return columns
}

var availableSchemaFields = []string{
	"sales_units", "sales_amount", "returns_qty", "inventory_sellable", "inventory_inbound", "inventory_reserved", "inventory_unfulfillable", "inventory_local_warehouse", "inventory_unhealthy_units", "inventory_aged90_sellable_units", "inventory_sell_through_rate", "inventory_receive_fill_rate", "inventory_vendor_confirmation_rate", "inventory_avg_lead_time_days", "inventory_sellable_cost", "inventory_unfulfillable_cost", "inventory_aged90_cost", "inventory_unhealthy_cost", "inventory_inbound_cost", "inventory_currency", "inventory_inbound_receiving", "inventory_inbound_shipped", "inventory_inbound_working", "inventory_reserved_customer_orders", "inventory_reserved_fc_processing", "inventory_reserved_fc_transfers", "sessions_desktop", "sessions_mobile", "sessions_total", "review_count", "rating", "sp_spend", "sp_sales", "sp_orders", "sp_impressions", "sp_clicks", "sd_spend", "sd_sales", "sd_orders", "sd_impressions", "sd_clicks", "hsa_spend", "hsa_sales", "hsa_orders", "hsa_impressions", "hsa_clicks", "sb_spend", "sb_sales", "sb_orders", "sb_impressions", "sb_clicks",
}

func columnsForFields(fields []string) []Column {
	out := make([]Column, 0, len(fields))
	for _, name := range fields {
		out = append(out, Column{Name: name, SQLType: schemaType(name), Nullable: true})
	}
	return out
}

func schemaType(name string) string {
	switch {
	case name == "business_date", name == "snapshot_date":
		return "DATE"
	case name == "start_time", name == "end_time":
		return "DATETIME(6)"
	case name == "glance_views", name == "total_orders", name == "ordered_units":
		return "BIGINT"
	case name == "total_spend", name == "total_sales", name == "ordered_revenue":
		return "DECIMAL(20,6)"
	case name == "classification_rank", name == "display_group_rank":
		return "JSON"
	case name == "reviews_num", name == "stars":
		return "VARCHAR(64)"
	case name == "sellable", name == "inbound", name == "unfulfillable", name == "unhealthy_units", name == "aged90_sellable_units", name == "ad_orders", name == "clicks", name == "impressions":
		return "BIGINT"
	case name == "currency":
		return "VARCHAR(16)"
	case name == "return_date_locale", name == "purchase_date_locale":
		return "VARCHAR(32)"
	case name == "gmt_modified":
		return "VARCHAR(64)"
	case name == "afn_erp_real_shipped_quantity", name == "afn_researching_quantity", name == "inv_age_0_to_90_days", name == "inv_age_271_to_330_days", name == "inv_age_331_to_365_days", name == "reserved_customerorders", name == "total_fulfillable_quantity":
		return "INT"
	case name == "brand_id", name == "category_id":
		return "BIGINT"
	case name == "brand_name":
		return "VARCHAR(128)"
	case name == "category_name":
		return "VARCHAR(128)"
	case name == "cost", name == "estimated_excess_quantity", name == "estimated_storage_cost_next_month", name == "fba_minimum_inventory_level", name == "long_term_historical_days_of_supply", name == "short_term_historical_days_of_supply":
		return "DECIMAL(14,4)"
	case name == "fulfillment_channel_name", name == "low_inventory_level_fee_applied", name == "share_type":
		return "VARCHAR(32)"
	case name == "name":
		return "VARCHAR(256)"
	case name == "product_image":
		return "VARCHAR(512)"
	case name == "recommended_action":
		return "VARCHAR(64)"
	case name == "wname":
		return "VARCHAR(64)"
	case name == "payments_date", name == "shipment_date", name == "reporting_date", name == "estimated_arrival_date", name == "hide_time":
		return "VARCHAR(40)"
	case name == "items", name == "ad_spend_sparkline_7d", name == "verified_fields":
		return "JSON"
	case name == "identity_scope":
		return "VARCHAR(16)"
	case name == "ordered_quantity", name == "received_quantity":
		return "BIGINT"
	case name == "item_amount":
		return "DECIMAL(18,4)"
	case name == "ack_status", name == "purchase_order_type", name == "purchase_order_process_state", name == "shipment_confirm_status", name == "shipment_label_status":
		return "INT"
	case name == "ack_status_desc":
		return "VARCHAR(64)"
	case name == "seller_name":
		return "VARCHAR(128)"
	case name == "erp_warehouse_id":
		return "VARCHAR(32)"
	case name == "erp_warehouse_name":
		return "VARCHAR(128)"
	case name == "remark":
		return "VARCHAR(512)"
	case name == "total_price":
		return "VARCHAR(32)"
	case name == "currency_code":
		return "VARCHAR(8)"
	case name == "vc_store_id":
		return "VARCHAR(32)"
	case name == "local_po_number", name == "purchase_order_number", name == "customer_order_number":
		return "VARCHAR(64)"
	case name == "quantity_shipped":
		return "INT"
	case name == "item_tax", name == "shipping_price", name == "shipping_tax", name == "gift_wrap_price", name == "gift_wrap_tax", name == "item_promotion_discount", name == "ship_promotion_discount", name == "points_granted":
		return "VARCHAR(32)"
	case name == "ship_service_level":
		return "VARCHAR(64)"
	case name == "carrier":
		return "VARCHAR(64)"
	case name == "tracking_number":
		return "VARCHAR(128)"
	case name == "ship_lat", name == "ship_lng":
		return "DECIMAL(10,7)"
	case name == "marketplace", name == "fulfillment_channel":
		return "VARCHAR(8)"
	case name == "ship_country":
		return "CHAR(2)"
	case name == "ship_state", name == "ship_city":
		return "VARCHAR(128)"
	case name == "ship_postal_code":
		return "VARCHAR(32)"
	case strings.HasSuffix(name, "_units"), strings.HasSuffix(name, "_orders"), strings.HasSuffix(name, "_count"), strings.HasPrefix(name, "sessions_"), strings.HasPrefix(name, "inv_age_"), name == "quantity", name == "quantity_shipped", strings.HasSuffix(name, "_quantity"), name == "returns_qty":
		return "BIGINT"
	case strings.HasSuffix(name, "_date"), strings.HasSuffix(name, "_date_locale"):
		return "VARCHAR(64)"
	case name == "source_updated_at":
		return "DATETIME(6)"
	case name == "source_global_order_no", name == "source_global_item_no", name == "source_order_item_no", name == "amazon_order_id", name == "amazon_order_item_id":
		return "VARCHAR(128)"
	case name == "latest_sync_at":
		return "DATETIME(6)"
	case name == "quantity_7d", name == "quantity_30d", name == "returns_30d", name == "inventory", name == "inbound_inventory", name == "reserved_inventory", name == "unfulfillable_inventory", name == "reviews_count", name == "ad_orders_7d", name == "sales_quantity_7d", name == "sales_quantity_30d", name == "sellable_inventory", name == "aged90_sellable_inventory", name == "unhealthy_inventory", name == "realtime_ordered_units", name == "glance_views":
		return "BIGINT"
	case name == "revenue_30d", name == "ad_spend_30d", name == "sales_revenue_7d", name == "sales_revenue_30d", name == "realtime_ordered_revenue", name == "net_ppm":
		return "DECIMAL(20,6)"
	case name == "sales_units", name == "returns_qty", name == "inventory_sellable", name == "inventory_inbound", name == "inventory_reserved", name == "inventory_unfulfillable", name == "sessions_total", name == "sessions_desktop", name == "sessions_mobile", name == "review_count", name == "sp_orders", name == "sp_impressions", name == "sp_clicks", name == "sd_orders", name == "sd_impressions", name == "sd_clicks", name == "hsa_orders", name == "hsa_impressions", name == "hsa_clicks", name == "sb_orders", name == "sb_impressions", name == "sb_clicks":
		return "BIGINT"
	case name == "sales_amount", name == "sp_spend", name == "sp_sales", name == "sd_spend", name == "sd_sales", name == "hsa_spend", name == "hsa_sales", name == "sb_spend", name == "sb_sales", name == "rating":
		return "DECIMAL(20,6)"
	case name == "is_provisional", name == "is_verified":
		return "TINYINT(1)"
	case name == "inventory_currency", name == "currency", strings.HasSuffix(name, "_status"):
		return "VARCHAR(255)"
	case strings.Contains(name, "rate"), name == "rating", name == "sell_through", name == "historical_days_of_supply", name == "inventory_avg_lead_time_days":
		return "DECIMAL(20,6)"
	case strings.Contains(name, "amount"), strings.Contains(name, "cost"), strings.Contains(name, "spend"), strings.HasSuffix(name, "_sales"):
		return "DECIMAL(20,6)"
	case strings.HasPrefix(name, "inventory_"):
		return "BIGINT"
	default:
		return "VARCHAR(255)"
	}
}

func SchemaFor(id string) (Schema, bool) {
	schema, ok := schemas[id]
	if !ok {
		return Schema{}, false
	}
	definition, _ := DefinitionFor(id)
	if definition.Kind != DatasetKindDaily {
		fields := definition.Fields
		if definition.ParentID != "" && len(definition.CatalogFields) > 0 {
			fields = definition.CatalogFields
		}
		schema.Columns = append(fixedColumnsForDataset(), columnsForFields(fields)...)
	} else if definition.ID != DatasetID {
		fields := dailyDefinitionFields(definition)
		schema.Columns = append(fixedColumnsForDataset(), columnsForFields(fields)...)
	}
	schema.Columns = append([]Column(nil), schema.Columns...)
	schema.PrimaryKey = append([]string(nil), schema.PrimaryKey...)
	return schema, true
}

func (s Schema) CreateTableSQL(selected []string) (string, error) {
	allowed := make(map[string]Column, len(s.Columns))
	for _, column := range s.Columns {
		allowed[column.Name] = column
	}
	definition, _ := DefinitionFor(s.DatasetID)
	definitionFields := definition.Fields
	if definition.ParentID != "" && len(definition.CatalogFields) > 0 {
		definitionFields = definition.CatalogFields
	}
	if definition.Kind == DatasetKindDaily {
		definitionFields = dailyDefinitionFields(definition)
	}
	business := make(map[string]struct{}, len(definitionFields))
	for _, name := range definitionFields {
		business[name] = struct{}{}
	}
	wanted := make([]string, 0, len(s.Columns))
	for _, column := range s.Columns {
		if _, ok := business[column.Name]; !ok {
			wanted = append(wanted, column.Name)
		}
	}
	if len(selected) == 0 {
		for _, column := range s.Columns {
			if _, ok := business[column.Name]; ok {
				wanted = append(wanted, column.Name)
			}
		}
	} else {
		wanted = append(wanted, selected...)
	}
	seen := make(map[string]struct{}, len(wanted))
	columns := make([]Column, 0, len(wanted)+len(s.PrimaryKey))
	for _, name := range wanted {
		if _, ok := seen[name]; ok {
			continue
		}
		column, ok := allowed[name]
		if !ok {
			return "", fmt.Errorf("字段不可用于 %s: %s", s.DatasetID, name)
		}
		seen[name] = struct{}{}
		columns = append(columns, column)
	}
	for _, name := range s.PrimaryKey {
		if _, ok := seen[name]; !ok {
			return "", fmt.Errorf("建表字段缺少主键列: %s", name)
		}
	}
	sort.SliceStable(columns, func(i, j int) bool { return indexOf(wanted, columns[i].Name) < indexOf(wanted, columns[j].Name) })
	lines := make([]string, 0, len(columns)+2)
	for _, column := range columns {
		nullable := " NULL"
		if !column.Nullable {
			nullable = " NOT NULL"
		}
		lines = append(lines, "  `"+column.Name+"` "+column.SQLType+nullable)
	}
	keys := make([]string, 0, len(s.PrimaryKey))
	for _, key := range s.PrimaryKey {
		keys = append(keys, "`"+key+"`")
	}
	lines = append(lines, "  PRIMARY KEY ("+strings.Join(keys, ", ")+")", "  KEY `idx_updated_at` (`updated_at`)")
	return "CREATE TABLE `" + s.TableName + "` (\n" + strings.Join(lines, ",\n") + "\n) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;", nil
}

func dailyDefinitionFields(definition Definition) []string {
	if len(definition.Fields) > 0 {
		return definition.Fields
	}
	return availableSchemaFields
}

func fixedColumnsForDataset() []Column {
	return []Column{{Name: "store", SQLType: "VARCHAR(64)", Nullable: false}, {Name: "record_date", SQLType: "DATE", Nullable: true}, {Name: "stable_key", SQLType: "VARCHAR(255)", Nullable: false}, {Name: "updated_at", SQLType: "DATETIME(6)", Nullable: false}}
}

func indexOf(values []string, value string) int {
	for i, item := range values {
		if item == value {
			return i
		}
	}
	return len(values)
}
