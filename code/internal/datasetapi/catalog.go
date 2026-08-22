package datasetapi

import "sort"

type DatasetKind string

const (
	DatasetKindDaily    DatasetKind = "daily"
	DatasetKindDetail   DatasetKind = "detail"
	DatasetKindSnapshot DatasetKind = "snapshot"
)

// Definition is a system-owned data product. It is deliberately static: a
// browser can select an entry, but cannot introduce a table, join, or SQL.
type Definition struct {
	ID            string
	Name          string
	Kind          DatasetKind
	Source        string
	Grain         string
	InitialCursor string
	FixedFields   []string
	Fields        []string
	CatalogFields []string
	ParentID      string
	NextVersionID string
}

var definitions = map[string]Definition{
	"listing-daily-v1": {
		ID: "listing-daily-v1", Name: "Listing 日维指标表", Kind: DatasetKindDaily,
		Source: "listing_dimensions + listing_daily_metrics", Grain: "store + channel + asin + sku + business_date",
		InitialCursor: "0|1000-01-01",
		FixedFields:   append([]string(nil), FixedFields...),
	},
	"return-reason-detail-v1": {
		ID: "return-reason-detail-v1", Name: "退货原因明细表", Kind: DatasetKindDetail,
		Source: "ls_sc_refunds", Grain: "store + license_plate_number",
		InitialCursor: "0|0|0",
		FixedFields:   []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"license_plate_number", "order_id", "asin", "sku", "fnsku", "local_sku", "product_name", "quantity", "return_date", "reason", "customer_comments", "remark", "detailed_disposition", "status", "fulfillment_center_id", "purchase_date"},
		CatalogFields: []string{"license_plate_number", "order_id", "asin", "sku", "fnsku", "local_sku", "product_name", "quantity", "return_date", "reason", "customer_comments", "remark", "detailed_disposition", "status", "fulfillment_center_id", "purchase_date", "return_date_locale", "purchase_date_locale", "gmt_modified"},
		NextVersionID: "return-reason-detail-v2",
	},
	"return-reason-detail-v2": {
		ID: "return-reason-detail-v2", Name: "退货原因明细表 v2", Kind: DatasetKindDetail,
		Source: "ls_sc_refunds", Grain: "store + license_plate_number", ParentID: "return-reason-detail-v1",
		InitialCursor: "0|0|0", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"license_plate_number", "order_id", "asin", "sku", "fnsku", "local_sku", "product_name", "quantity", "return_date", "reason", "customer_comments", "remark", "detailed_disposition", "status", "fulfillment_center_id", "purchase_date"},
		CatalogFields: []string{"license_plate_number", "order_id", "asin", "sku", "fnsku", "local_sku", "product_name", "quantity", "return_date", "reason", "customer_comments", "remark", "detailed_disposition", "status", "fulfillment_center_id", "purchase_date", "return_date_locale", "purchase_date_locale", "gmt_modified"},
	},
	"fba-inventory-snapshot-v1": {
		ID: "fba-inventory-snapshot-v1", Name: "FBA 库存快照表", Kind: DatasetKindSnapshot,
		Source: "fba_inventory_daily_snapshots", Grain: "store + fnsku + snapshot_date",
		InitialCursor: "0|0|0|1000-01-01",
		FixedFields:   []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"fnsku", "msku", "asin", "sku", "product_name", "fulfillable_quantity", "unsellable_quantity", "reserved_quantity", "inbound_receiving_quantity", "inbound_shipped_quantity", "inbound_working_quantity", "inv_age_0_to_30_days", "inv_age_31_to_60_days", "inv_age_61_to_90_days", "inv_age_91_to_180_days", "inv_age_181_to_270_days", "inv_age_271_to_365_days", "inv_age_365_plus_days", "stock_cost_total", "sell_through", "historical_days_of_supply", "fba_inventory_level_health_status"},
		CatalogFields: []string{"fnsku", "msku", "asin", "sku", "product_name", "fulfillable_quantity", "unsellable_quantity", "reserved_quantity", "inbound_receiving_quantity", "inbound_shipped_quantity", "inbound_working_quantity", "inv_age_0_to_30_days", "inv_age_31_to_60_days", "inv_age_61_to_90_days", "inv_age_91_to_180_days", "inv_age_181_to_270_days", "inv_age_271_to_365_days", "inv_age_365_plus_days", "stock_cost_total", "sell_through", "historical_days_of_supply", "fba_inventory_level_health_status", "afn_erp_real_shipped_quantity", "afn_researching_quantity", "brand_id", "brand_name", "category_id", "category_name", "cost", "estimated_excess_quantity", "estimated_storage_cost_next_month", "fba_minimum_inventory_level", "fulfillment_channel_name", "inv_age_0_to_90_days", "inv_age_271_to_330_days", "inv_age_331_to_365_days", "long_term_historical_days_of_supply", "low_inventory_level_fee_applied", "name", "product_image", "recommended_action", "reserved_customerorders", "share_type", "short_term_historical_days_of_supply", "total_fulfillable_quantity", "wname"},
		NextVersionID: "fba-inventory-snapshot-v2",
	},
	"fba-inventory-snapshot-v2": {
		ID: "fba-inventory-snapshot-v2", Name: "FBA 库存快照表 v2", Kind: DatasetKindSnapshot,
		Source: "fba_inventory_daily_snapshots", Grain: "store + fnsku + snapshot_date", ParentID: "fba-inventory-snapshot-v1",
		InitialCursor: "0|0|0|1000-01-01", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"fnsku", "msku", "asin", "sku", "product_name", "fulfillable_quantity", "unsellable_quantity", "reserved_quantity", "inbound_receiving_quantity", "inbound_shipped_quantity", "inbound_working_quantity", "inv_age_0_to_30_days", "inv_age_31_to_60_days", "inv_age_61_to_90_days", "inv_age_91_to_180_days", "inv_age_181_to_270_days", "inv_age_271_to_365_days", "inv_age_365_plus_days", "stock_cost_total", "sell_through", "historical_days_of_supply", "fba_inventory_level_health_status"},
		CatalogFields: []string{"fnsku", "msku", "asin", "sku", "product_name", "fulfillable_quantity", "unsellable_quantity", "reserved_quantity", "inbound_receiving_quantity", "inbound_shipped_quantity", "inbound_working_quantity", "inv_age_0_to_30_days", "inv_age_31_to_60_days", "inv_age_61_to_90_days", "inv_age_91_to_180_days", "inv_age_181_to_270_days", "inv_age_271_to_365_days", "inv_age_365_plus_days", "stock_cost_total", "sell_through", "historical_days_of_supply", "fba_inventory_level_health_status", "afn_erp_real_shipped_quantity", "afn_researching_quantity", "brand_id", "brand_name", "category_id", "category_name", "cost", "estimated_excess_quantity", "estimated_storage_cost_next_month", "fba_minimum_inventory_level", "fulfillment_channel_name", "inv_age_0_to_90_days", "inv_age_271_to_330_days", "inv_age_331_to_365_days", "long_term_historical_days_of_supply", "low_inventory_level_fee_applied", "name", "product_image", "recommended_action", "reserved_customerorders", "share_type", "short_term_historical_days_of_supply", "total_fulfillable_quantity", "wname"},
	},
	"order-shipping-address-detail-v1": {
		ID: "order-shipping-address-detail-v1", Name: "订单配送地址明细表", Kind: DatasetKindDetail,
		Source: "ls_sc_fba_order_addresses", Grain: "store + shipment_id + shipment_item_id",
		InitialCursor: "0|0|0|0",
		FixedFields:   []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"shipment_id", "shipment_item_id", "amazon_order_id", "amazon_order_item_id", "purchase_date", "payments_date", "shipment_date", "reporting_date", "estimated_arrival_date", "sku", "product_name", "quantity_shipped", "currency", "item_price", "item_tax", "shipping_price", "shipping_tax", "gift_wrap_price", "gift_wrap_tax", "ship_service_level", "item_promotion_discount", "ship_promotion_discount", "carrier", "tracking_number", "fulfillment_channel", "points_granted", "hide_time", "ship_city", "ship_state", "ship_postal_code", "ship_country"},
	},
	"address-order-item-detail-v1": {
		ID: "address-order-item-detail-v1", Name: "Address 订单商品配送明细表", Kind: DatasetKindDetail,
		Source: "ls_sc_fba_order_addresses + ls_sales_orders + ls_sc_order_details + ls_stores", Grain: "store + shipment_id + shipment_item_id",
		InitialCursor: "0|0|0|0",
		FixedFields:   []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"amazon_order_id", "store_name", "marketplace", "purchase_date", "last_updated_date", "order_status", "asin", "sku", "product_name", "quantity", "currency", "item_price", "fulfillment_channel", "ship_country", "ship_state", "ship_city", "ship_postal_code", "ship_lat", "ship_lng", "source_store", "shipment_id", "shipment_item_id", "amazon_order_item_id", "source_updated_at"},
		CatalogFields: []string{"amazon_order_id", "store_name", "marketplace", "purchase_date", "last_updated_date", "order_status", "asin", "sku", "product_name", "quantity", "currency", "item_price", "fulfillment_channel", "ship_country", "ship_state", "ship_city", "ship_postal_code", "ship_lat", "ship_lng", "source_store", "shipment_id", "shipment_item_id", "amazon_order_item_id", "source_updated_at", "payments_date", "shipment_date", "reporting_date", "estimated_arrival_date", "quantity_shipped", "item_tax", "shipping_price", "shipping_tax", "gift_wrap_price", "gift_wrap_tax", "ship_service_level", "item_promotion_discount", "ship_promotion_discount", "carrier", "tracking_number", "points_granted", "hide_time"},
		NextVersionID: "address-order-item-detail-v2",
	},
	"address-order-item-detail-v2": {
		ID: "address-order-item-detail-v2", Name: "Address 订单商品配送明细表 v2", Kind: DatasetKindDetail,
		Source: "ls_sc_fba_order_addresses + ls_sales_orders + ls_sc_order_details + ls_stores", Grain: "store + shipment_id + shipment_item_id", ParentID: "address-order-item-detail-v1",
		InitialCursor: "0|0|0|0", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"amazon_order_id", "store_name", "marketplace", "purchase_date", "last_updated_date", "order_status", "asin", "sku", "product_name", "quantity", "currency", "item_price", "fulfillment_channel", "ship_country", "ship_state", "ship_city", "ship_postal_code", "ship_lat", "ship_lng", "source_store", "shipment_id", "shipment_item_id", "amazon_order_item_id", "source_updated_at"},
		CatalogFields: []string{"amazon_order_id", "store_name", "marketplace", "purchase_date", "last_updated_date", "order_status", "asin", "sku", "product_name", "quantity", "currency", "item_price", "fulfillment_channel", "ship_country", "ship_state", "ship_city", "ship_postal_code", "ship_lat", "ship_lng", "source_store", "shipment_id", "shipment_item_id", "amazon_order_item_id", "source_updated_at", "payments_date", "shipment_date", "reporting_date", "estimated_arrival_date", "quantity_shipped", "item_tax", "shipping_price", "shipping_tax", "gift_wrap_price", "gift_wrap_tax", "ship_service_level", "item_promotion_discount", "ship_promotion_discount", "carrier", "tracking_number", "points_granted", "hide_time"},
	},
	"vc-po-detail-v1": {
		ID: "vc-po-detail-v1", Name: "VC PO 订单明细表", Kind: DatasetKindDetail,
		Source: "ls_vc_orders + ls_vc_po_details", Grain: "store + local_po_number",
		InitialCursor: "0|0|0", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields: []string{"vc_store_id", "local_po_number", "purchase_order_number", "purchase_order_date", "purchase_order_state", "payment_method", "total_price", "currency_code", "item_amount", "ship_window_start", "ship_window_end", "delivery_window_start", "delivery_window_end", "items", "seller_name", "purchase_order_type", "purchase_order_process_state", "ack_status", "ack_status_desc", "shipment_confirm_status", "shipment_label_status", "customer_order_number", "erp_warehouse_id", "erp_warehouse_name", "remark"},
	},
	"vc-po-lines-v1": {
		ID: "vc-po-lines-v1", Name: "VC PO 商品行表", Kind: DatasetKindDetail,
		Source: "ls_vc_po_details.items + ls_vc_orders", Grain: "store + local_po_number + asin + msku",
		InitialCursor: "0|0|0|0|0", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields: []string{"vc_store_id", "local_po_number", "purchase_order_number", "asin", "msku", "sku", "item_name", "ordered_quantity", "received_quantity", "unit_price", "image_url"},
	},
	"fba-links-v1": {
		ID: "fba-links-v1", Name: "FBA Links 页面数据表", Kind: DatasetKindDetail,
		Source: "ls_sc_listing + listing_daily_metrics", Grain: "store + ASIN + listing_sku",
		InitialCursor: "0|0|0", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields: []string{"store_name", "country", "channel_type", "fulfillment_type", "asin", "parent_asin", "msku", "title", "brand", "image_url", "quantity_7d", "quantity_30d", "revenue_30d", "revenue_currency", "returns_30d", "inventory", "inbound_inventory", "reserved_inventory", "unfulfillable_inventory", "latest_inventory_date", "rating", "reviews_count", "ad_orders_7d", "ad_spend_30d", "ad_spend_currency", "account_ad_spend_30d", "account_ad_breakdown_30d", "sparkline_7d", "latest_date", "latest_sync_at", "revenue_rate_to_usd", "revenue_rate_date", "sibling_count", "sibling_skus", "visibility_status", "tag_names"},
	},
	"vc-links-v1": {
		ID: "vc-links-v1", Name: "VC Links 页面数据表", Kind: DatasetKindDetail,
		Source: "ls_vc_listing + ls_vc_sales_report + ls_vc_inventory + ls_vc_traffic + ls_vc_margin + ls_vc_realtime_sales + ls_ad_sp_product + ls_ad_sd_product", Grain: "store + ASIN",
		InitialCursor: "0|0|0", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields: []string{"store_name", "country", "channel_type", "asin", "parent_asin", "msku", "title", "brand", "image_url", "quantity_7d", "quantity_30d", "revenue_30d", "revenue_currency", "returns_30d", "inventory", "rating", "reviews_count", "ad_orders_7d", "ad_spend_30d", "ad_sales_30d", "ad_spend_currency", "sales_quantity_7d", "sales_quantity_30d", "sales_revenue_7d", "sales_revenue_30d", "sales_sparkline_7d", "sales_revenue_sparkline_7d", "realtime_revenue_sparkline_7d", "ad_spend_sparkline_7d", "sellable_inventory", "inbound_inventory", "unfulfillable_inventory", "aged90_sellable_inventory", "unhealthy_inventory", "latest_inventory_date", "realtime_ordered_units", "realtime_ordered_revenue", "latest_realtime_end_at", "glance_views", "net_ppm", "latest_margin_date", "latest_date", "latest_sync_at", "sales_latest_date", "sales_latest_sync_at", "vc_sales_covered_dates", "visibility_status"},
	},
	"operations-log-v1": {
		ID: "operations-log-v1", Name: "运营日志事实数据表", Kind: DatasetKindDetail,
		Source: "listing_daily_metrics", Grain: "store + channel + ASIN + listing_sku + business_date",
		InitialCursor: "0|0|0|0|1000-01-01", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"channel_type", "asin", "listing_sku", "sales_units", "sales_amount", "returns_qty", "inventory_sellable", "inventory_inbound", "inventory_reserved", "inventory_unfulfillable", "sessions_total", "sessions_desktop", "sessions_mobile", "rating", "review_count", "sp_spend", "sp_sales", "sp_orders", "sd_spend", "sd_sales", "sd_orders", "hsa_spend", "hsa_sales", "hsa_orders", "sb_spend", "sb_sales", "sb_orders", "is_provisional", "is_verified"},
		NextVersionID: "operations-log-v2",
	},
	"operations-log-v2": {
		ID: "operations-log-v2", Name: "运营日志事实数据表 v2", Kind: DatasetKindDetail,
		Source: "listing_daily_metrics", Grain: "store + channel + identity_scope + optional ASIN + optional listing_sku + business_date", ParentID: "operations-log-v1",
		InitialCursor: "0|0|0|0|1000-01-01", FixedFields: []string{"store", "record_date", "stable_key", "updated_at"},
		Fields: []string{"channel_type", "identity_scope", "asin", "listing_sku", "sales_units", "sales_amount", "returns_qty", "inventory_sellable", "inventory_inbound", "inventory_reserved", "inventory_unfulfillable", "sessions_total", "sessions_desktop", "sessions_mobile", "rating", "review_count", "sp_spend", "sp_sales", "sp_orders", "sd_spend", "sd_sales", "sd_orders", "hsa_spend", "hsa_sales", "hsa_orders", "sb_spend", "sb_sales", "sb_orders", "is_provisional", "is_verified", "verified_fields"},
	},
}

func Definitions() []Definition {
	ids := make([]string, 0, len(definitions))
	for id := range definitions {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]Definition, 0, len(ids))
	for _, id := range ids {
		out = append(out, copyDefinition(definitions[id]))
	}
	return out
}

func DefinitionFor(id string) (Definition, bool) {
	definition, ok := definitions[id]
	return copyDefinition(definition), ok
}

func copyDefinition(definition Definition) Definition {
	definition.FixedFields = append([]string(nil), definition.FixedFields...)
	definition.Fields = append([]string(nil), definition.Fields...)
	definition.CatalogFields = append([]string(nil), definition.CatalogFields...)
	return definition
}
