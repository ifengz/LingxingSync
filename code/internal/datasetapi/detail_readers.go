package datasetapi

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// DetailSQLReader only reads one registered data product. Its source table,
// fields, date expression, and cursor key are all fixed in code.
type DetailSQLReader struct {
	queryer    SQLQueryer
	definition detailReaderDefinition
}

type detailReaderDefinition struct {
	sourceTable     string
	fromClause      string
	alias           string
	storeColumn     string
	baseColumns     []string
	dateColumn      string
	stableKeyColumn string
	updatedAtColumn string
	fields          map[string]string
	stableKeyParts  int
}

var returnReasonDetailDefinition = detailReaderDefinition{
	sourceTable:     "ls_sc_refunds",
	alias:           "r",
	baseColumns:     []string{"r.account_id", "r.sid", "r.asin", "COALESCE(NULLIF(r.sku, ''), NULLIF(r.local_sku, ''))", "r.return_date_locale", "r.synced_at", "CONCAT_WS('|', r.account_id, r.sid, r.license_plate_number)"},
	dateColumn:      "r.return_date_locale",
	stableKeyColumn: "CONCAT_WS('|', r.account_id, r.sid, r.license_plate_number)",
	updatedAtColumn: "r.synced_at",
	stableKeyParts:  3,
	fields: map[string]string{
		"license_plate_number":  "r.license_plate_number",
		"order_id":              "r.order_id",
		"asin":                  "r.asin",
		"sku":                   "COALESCE(NULLIF(r.sku, ''), NULLIF(r.local_sku, ''))",
		"fnsku":                 "r.fnsku",
		"local_sku":             "r.local_sku",
		"product_name":          "r.product_name",
		"quantity":              "r.quantity",
		"return_date":           "r.return_date",
		"purchase_date":         "r.purchase_date",
		"return_date_locale":    "r.return_date_locale",
		"purchase_date_locale":  "r.purchase_date_locale",
		"gmt_modified":          "r.gmt_modified",
		"reason":                "r.reason",
		"detailed_disposition":  "r.detailed_disposition",
		"status":                "r.status",
		"customer_comments":     "r.customer_comments",
		"remark":                "r.remark",
		"fulfillment_center_id": "r.fulfillment_center_id",
	},
}

var orderShippingAddressDetailDefinition = detailReaderDefinition{
	sourceTable:     "ls_sc_fba_order_addresses",
	alias:           "a",
	baseColumns:     []string{"a.account_id", "a.sid", "''", "a.sku", "DATE(a.purchase_date)", "a.synced_at", "CONCAT_WS('|', a.account_id, a.sid, a.shipment_id, a.shipment_item_id)"},
	dateColumn:      "DATE(a.purchase_date)",
	stableKeyColumn: "CONCAT_WS('|', a.account_id, a.sid, a.shipment_id, a.shipment_item_id)",
	updatedAtColumn: "a.synced_at",
	stableKeyParts:  4,
	fields: map[string]string{
		"amazon_order_id":         "a.amazon_order_id",
		"sku":                     "a.sku",
		"shipment_id":             "a.shipment_id",
		"shipment_item_id":        "a.shipment_item_id",
		"amazon_order_item_id":    "a.amazon_order_item_id",
		"purchase_date":           "a.purchase_date",
		"payments_date":           "a.payments_date",
		"shipment_date":           "a.shipment_date",
		"reporting_date":          "a.reporting_date",
		"estimated_arrival_date":  "a.estimated_arrival_date",
		"product_name":            "a.product_name",
		"quantity_shipped":        "a.quantity_shipped",
		"currency":                "a.currency",
		"item_price":              "a.item_price",
		"item_tax":                "a.item_tax",
		"shipping_price":          "a.shipping_price",
		"shipping_tax":            "a.shipping_tax",
		"gift_wrap_price":         "a.gift_wrap_price",
		"gift_wrap_tax":           "a.gift_wrap_tax",
		"ship_service_level":      "a.ship_service_level",
		"item_promotion_discount": "a.item_promotion_discount",
		"ship_promotion_discount": "a.ship_promotion_discount",
		"carrier":                 "a.carrier",
		"tracking_number":         "a.tracking_number",
		"fulfillment_channel":     "a.fulfillment_channel",
		"points_granted":          "a.points_granted",
		"hide_time":               "a.hide_time",
		"ship_city":               "a.ship_city",
		"ship_state":              "a.ship_state",
		"ship_postal_code":        "a.ship_postal_code",
		"ship_country":            "a.ship_country",
	},
}

const addressOrderItemJSONPath = "JSON_UNQUOTE(JSON_SEARCH(d.item_list, 'one', a.amazon_order_item_id, NULL, '$[*].order_item_id'))"

func addressOrderItemJSONValue(field string) string {
	return "JSON_UNQUOTE(JSON_EXTRACT(d.item_list, REPLACE(" + addressOrderItemJSONPath + ", '.order_item_id', '." + field + "')))"
}

var addressOrderItemDetailDefinition = detailReaderDefinition{
	fromClause: `ls_sc_fba_order_addresses a
LEFT JOIN ls_sales_orders o
  ON o.account_id = a.account_id AND o.sid = a.sid AND o.amazon_order_id = a.amazon_order_id
LEFT JOIN ls_sc_order_details d
  ON d.account_id = a.account_id AND d.sid = a.sid AND d.amazon_order_id = a.amazon_order_id
LEFT JOIN ls_stores s
	  ON s.account_id = a.account_id AND s.sid = a.sid AND s.store_type = 'SC'`,
	storeColumn:     "a.sid",
	baseColumns:     []string{"a.account_id", "a.sid", addressOrderItemJSONValue("asin"), "COALESCE(NULLIF(" + addressOrderItemJSONValue("seller_sku") + ", ''), NULLIF(" + addressOrderItemJSONValue("sku") + ", ''), a.sku)", "DATE(a.purchase_date)", "GREATEST(a.synced_at, COALESCE(o.synced_at, a.synced_at), COALESCE(d.synced_at, a.synced_at), COALESCE(s.synced_at, a.synced_at))", "CONCAT_WS('|', a.account_id, a.sid, a.shipment_id, a.shipment_item_id)"},
	dateColumn:      "DATE(a.purchase_date)",
	stableKeyColumn: "CONCAT_WS('|', a.account_id, a.sid, a.shipment_id, a.shipment_item_id)",
	updatedAtColumn: "GREATEST(a.synced_at, COALESCE(o.synced_at, a.synced_at), COALESCE(d.synced_at, a.synced_at), COALESCE(s.synced_at, a.synced_at))",
	stableKeyParts:  4,
	fields: map[string]string{
		"amazon_order_id":         "a.amazon_order_id",
		"store_name":              "o.seller_name",
		"marketplace":             "CASE s.marketplace_id WHEN 'ATVPDKIKX0DER' THEN 'US' WHEN 'A39IBJ37TRP1C6' THEN 'AU' WHEN 'A1VC38T7YXB528' THEN 'JP' END",
		"purchase_date":           "a.purchase_date",
		"last_updated_date":       "o.last_update_date",
		"order_status":            "COALESCE(NULLIF(o.order_status, ''), d.order_status)",
		"asin":                    addressOrderItemJSONValue("asin"),
		"sku":                     "COALESCE(NULLIF(" + addressOrderItemJSONValue("seller_sku") + ", ''), NULLIF(" + addressOrderItemJSONValue("sku") + ", ''), a.sku)",
		"product_name":            "COALESCE(NULLIF(" + addressOrderItemJSONValue("product_name") + ", ''), a.product_name)",
		"quantity":                "COALESCE(CAST(NULLIF(" + addressOrderItemJSONValue("quantity_shipped") + ", '') AS SIGNED), CAST(NULLIF(" + addressOrderItemJSONValue("quantity_ordered") + ", '') AS SIGNED), a.quantity_shipped)",
		"currency":                "a.currency",
		"item_price":              "a.item_price",
		"fulfillment_channel":     "CASE a.fulfillment_channel WHEN 'AFN' THEN 'FBA' WHEN 'MFN' THEN 'FBM' ELSE a.fulfillment_channel END",
		"ship_country":            "a.ship_country",
		"ship_state":              "a.ship_state",
		"ship_city":               "a.ship_city",
		"ship_postal_code":        "a.ship_postal_code",
		"ship_lat":                "CAST(NULL AS DECIMAL(10,7))",
		"ship_lng":                "CAST(NULL AS DECIMAL(10,7))",
		"source_store":            "a.sid",
		"shipment_id":             "a.shipment_id",
		"shipment_item_id":        "a.shipment_item_id",
		"amazon_order_item_id":    "a.amazon_order_item_id",
		"source_updated_at":       "GREATEST(a.synced_at, COALESCE(o.synced_at, a.synced_at), COALESCE(d.synced_at, a.synced_at), COALESCE(s.synced_at, a.synced_at))",
		"payments_date":           "a.payments_date",
		"shipment_date":           "a.shipment_date",
		"reporting_date":          "a.reporting_date",
		"estimated_arrival_date":  "a.estimated_arrival_date",
		"quantity_shipped":        "a.quantity_shipped",
		"item_tax":                "a.item_tax",
		"shipping_price":          "a.shipping_price",
		"shipping_tax":            "a.shipping_tax",
		"gift_wrap_price":         "a.gift_wrap_price",
		"gift_wrap_tax":           "a.gift_wrap_tax",
		"ship_service_level":      "a.ship_service_level",
		"item_promotion_discount": "a.item_promotion_discount",
		"ship_promotion_discount": "a.ship_promotion_discount",
		"carrier":                 "a.carrier",
		"tracking_number":         "a.tracking_number",
		"points_granted":          "a.points_granted",
		"hide_time":               "a.hide_time",
	},
}

var fbaInventorySnapshotDefinition = detailReaderDefinition{
	sourceTable:     "fba_inventory_daily_snapshots",
	alias:           "i",
	baseColumns:     []string{"i.account_id", "i.sid", "i.asin", "i.sku", "i.snapshot_date", "i.updated_at", "CONCAT_WS('|', i.account_id, i.sid, i.fnsku, i.snapshot_date)"},
	dateColumn:      "i.snapshot_date",
	stableKeyColumn: "CONCAT_WS('|', i.account_id, i.sid, i.fnsku, i.snapshot_date)",
	updatedAtColumn: "i.updated_at",
	stableKeyParts:  4,
	fields: map[string]string{
		"fnsku":                                "i.fnsku",
		"msku":                                 "i.msku",
		"asin":                                 "i.asin",
		"sku":                                  "i.sku",
		"product_name":                         "COALESCE(NULLIF(i.product_name, ''), i.name)",
		"fulfillable_quantity":                 "i.afn_fulfillable_quantity",
		"inbound_receiving_quantity":           "i.afn_inbound_receiving_quantity",
		"inbound_shipped_quantity":             "i.afn_inbound_shipped_quantity",
		"inbound_working_quantity":             "i.afn_inbound_working_quantity",
		"reserved_quantity":                    "i.afn_reserved_quantity",
		"unsellable_quantity":                  "i.afn_unsellable_quantity",
		"inv_age_0_to_30_days":                 "i.inv_age_0_to_30_days",
		"inv_age_31_to_60_days":                "i.inv_age_31_to_60_days",
		"inv_age_61_to_90_days":                "i.inv_age_61_to_90_days",
		"inv_age_91_to_180_days":               "i.inv_age_91_to_180_days",
		"inv_age_181_to_270_days":              "i.inv_age_181_to_270_days",
		"inv_age_271_to_365_days":              "i.inv_age_271_to_365_days",
		"inv_age_365_plus_days":                "i.inv_age_365_plus_days",
		"stock_cost_total":                     "i.stock_cost_total",
		"sell_through":                         "i.sell_through",
		"historical_days_of_supply":            "i.historical_days_of_supply",
		"fba_inventory_level_health_status":    "i.fba_inventory_level_health_status",
		"afn_erp_real_shipped_quantity":        "i.afn_erp_real_shipped_quantity",
		"afn_researching_quantity":             "i.afn_researching_quantity",
		"brand_id":                             "i.brand_id",
		"brand_name":                           "i.brand_name",
		"category_id":                          "i.category_id",
		"category_name":                        "i.category_name",
		"cost":                                 "i.cost",
		"estimated_excess_quantity":            "i.estimated_excess_quantity",
		"estimated_storage_cost_next_month":    "i.estimated_storage_cost_next_month",
		"fba_minimum_inventory_level":          "i.fba_minimum_inventory_level",
		"fulfillment_channel_name":             "i.fulfillment_channel_name",
		"inv_age_0_to_90_days":                 "i.inv_age_0_to_90_days",
		"inv_age_271_to_330_days":              "i.inv_age_271_to_330_days",
		"inv_age_331_to_365_days":              "i.inv_age_331_to_365_days",
		"long_term_historical_days_of_supply":  "i.long_term_historical_days_of_supply",
		"low_inventory_level_fee_applied":      "i.low_inventory_level_fee_applied",
		"name":                                 "i.name",
		"product_image":                        "i.product_image",
		"recommended_action":                   "i.recommended_action",
		"reserved_customerorders":              "i.reserved_customerorders",
		"share_type":                           "i.share_type",
		"short_term_historical_days_of_supply": "i.short_term_historical_days_of_supply",
		"total_fulfillable_quantity":           "i.total_fulfillable_quantity",
		"wname":                                "i.wname",
	},
}

var vcPODetailDefinition = detailReaderDefinition{
	fromClause: `ls_vc_po_details d
LEFT JOIN ls_vc_orders o
  ON o.account_id = d.account_id AND o.vc_store_id = d.vc_store_id AND o.local_po_number = d.local_po_number`,
	storeColumn:     "d.vc_store_id",
	baseColumns:     []string{"d.account_id", "d.vc_store_id", "''", "''", "DATE(d.synced_at)", "d.synced_at", "CONCAT_WS('|', d.account_id, d.vc_store_id, d.local_po_number)"},
	dateColumn:      "DATE(d.synced_at)",
	stableKeyColumn: "CONCAT_WS('|', d.account_id, d.vc_store_id, d.local_po_number)",
	updatedAtColumn: "d.synced_at",
	stableKeyParts:  3,
	fields: map[string]string{
		"vc_store_id":                  "d.vc_store_id",
		"local_po_number":              "d.local_po_number",
		"purchase_order_number":        "d.purchase_order_number",
		"purchase_order_date":          "d.purchase_order_date",
		"purchase_order_state":         "d.purchase_order_state",
		"payment_method":               "d.payment_method",
		"total_price":                  "d.total_price",
		"currency_code":                "d.currency_code",
		"item_amount":                  "d.item_amount",
		"ship_window_start":            "d.ship_window_start",
		"ship_window_end":              "d.ship_window_end",
		"delivery_window_start":        "d.delivery_window_start",
		"delivery_window_end":          "d.delivery_window_end",
		"items":                        "d.items",
		"seller_name":                  "o.seller_name",
		"purchase_order_type":          "o.purchase_order_type",
		"purchase_order_process_state": "o.purchase_order_process_state",
		"ack_status":                   "o.ack_status",
		"ack_status_desc":              "o.ack_status_desc",
		"shipment_confirm_status":      "o.shipment_confirm_status",
		"shipment_label_status":        "o.shipment_label_status",
		"customer_order_number":        "o.customer_order_number",
		"erp_warehouse_id":             "o.erp_warehouse_id",
		"erp_warehouse_name":           "o.erp_warehouse_name",
		"remark":                       "o.remark",
	},
}

var fbaLinksDefinition = detailReaderDefinition{
	fromClause: `ls_sc_listing l
JOIN ls_stores s
  ON s.account_id = l.account_id AND s.sid = l.sid AND s.store_type = 'SC'
LEFT JOIN (
  SELECT d.store_id, d.asin, d.sku,
         SUM(CASE WHEN m.business_date >= DATE_SUB(CURDATE(), INTERVAL 6 DAY) THEN m.sales_units END) AS quantity_7d,
         SUM(CASE WHEN m.business_date >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN m.sales_units END) AS quantity_30d,
         SUM(CASE WHEN m.business_date >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN m.sales_amount END) AS revenue_30d,
         SUM(CASE WHEN m.business_date >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN m.returns_qty END) AS returns_30d,
         SUM(CASE WHEN m.business_date >= DATE_SUB(CURDATE(), INTERVAL 6 DAY) THEN COALESCE(m.sp_orders, 0) + COALESCE(m.sd_orders, 0) + COALESCE(m.hsa_orders, 0) + COALESCE(m.sb_orders, 0) END) AS ad_orders_7d,
         SUM(CASE WHEN m.business_date >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN COALESCE(m.sp_spend, 0) + COALESCE(m.sd_spend, 0) + COALESCE(m.hsa_spend, 0) + COALESCE(m.sb_spend, 0) END) AS ad_spend_30d,
         MAX(m.business_date) AS latest_date,
         MAX(m.updated_at) AS latest_sync_at
    FROM listing_dimensions d
    JOIN listing_daily_metrics m ON m.listing_dimension_id = d.id
   WHERE d.channel = 'sc'
   GROUP BY d.store_id, d.asin, d.sku
) metrics
  ON metrics.store_id = l.sid AND metrics.asin = l.asin AND metrics.sku = l.seller_sku`,
	storeColumn:     "l.sid",
	baseColumns:     []string{"l.account_id", "l.sid", "l.asin", "l.seller_sku", "DATE(l.synced_at)", "COALESCE(metrics.latest_sync_at, l.synced_at)", "CONCAT_WS('|', l.account_id, l.sid, l.seller_sku)"},
	dateColumn:      "DATE(l.synced_at)",
	stableKeyColumn: "CONCAT_WS('|', l.account_id, l.sid, l.seller_sku)",
	updatedAtColumn: "COALESCE(metrics.latest_sync_at, l.synced_at)",
	stableKeyParts:  3,
	fields: map[string]string{
		"store_name":               "s.store_name",
		"country":                  "s.country",
		"channel_type":             "'sc'",
		"fulfillment_type":         "CASE WHEN l.fulfillment_channel_type LIKE '%AFN%' THEN 'fba' WHEN l.fulfillment_channel_type LIKE '%MFN%' THEN 'fbm' ELSE l.fulfillment_channel_type END",
		"asin":                     "l.asin",
		"parent_asin":              "l.parent_asin",
		"msku":                     "l.seller_sku",
		"title":                    "l.item_name",
		"brand":                    "l.seller_brand",
		"image_url":                "l.small_image_url",
		"quantity_7d":              "metrics.quantity_7d",
		"quantity_30d":             "metrics.quantity_30d",
		"revenue_30d":              "metrics.revenue_30d",
		"revenue_currency":         "l.currency_code",
		"returns_30d":              "metrics.returns_30d",
		"inventory":                "CASE WHEN l.fulfillment_channel_type LIKE '%AFN%' THEN l.afn_fulfillable_quantity END",
		"inbound_inventory":        "CASE WHEN l.fulfillment_channel_type LIKE '%AFN%' THEN l.afn_inbound_working_quantity + l.afn_inbound_shipped_quantity + l.afn_inbound_receiving_quantity END",
		"reserved_inventory":       "CASE WHEN l.fulfillment_channel_type LIKE '%AFN%' THEN l.afn_reserved_quantity END",
		"unfulfillable_inventory":  "CASE WHEN l.fulfillment_channel_type LIKE '%AFN%' THEN l.afn_unsellable_quantity END",
		"latest_inventory_date":    "DATE(l.synced_at)",
		"rating":                   "l.last_star",
		"reviews_count":            "l.review_num",
		"ad_orders_7d":             "metrics.ad_orders_7d",
		"ad_spend_30d":             "metrics.ad_spend_30d",
		"ad_spend_currency":        "l.currency_code",
		"account_ad_spend_30d":     "CAST(NULL AS DECIMAL(20,6))",
		"account_ad_breakdown_30d": "CAST(NULL AS JSON)",
		"sparkline_7d":             "CAST(NULL AS JSON)",
		"latest_date":              "COALESCE(metrics.latest_date, DATE(l.synced_at))",
		"latest_sync_at":           "COALESCE(metrics.latest_sync_at, l.synced_at)",
		"revenue_rate_to_usd":      "CAST(NULL AS DECIMAL(20,6))",
		"revenue_rate_date":        "CAST(NULL AS DATE)",
		"sibling_count":            "1",
		"sibling_skus":             "l.seller_sku",
		"visibility_status":        "CASE WHEN COALESCE(l.status, 0) = 1 THEN 'active' ELSE CAST(l.status AS CHAR) END",
		"tag_names":                "CAST(NULL AS JSON)",
	},
}

var vcLinksDefinition = detailReaderDefinition{
	fromClause: `ls_vc_listing v
JOIN ls_stores s
  ON s.account_id = v.account_id AND s.sid = v.vc_store_id AND s.store_type = 'VC'
LEFT JOIN (
  SELECT account_id, sid, asin,
         SUM(CASE WHEN ` + "`date`" + ` >= DATE_SUB(CURDATE(), INTERVAL 6 DAY) THEN shippedUnits END) AS sales_quantity_7d,
         SUM(CASE WHEN ` + "`date`" + ` >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN shippedUnits END) AS sales_quantity_30d,
         SUM(CASE WHEN ` + "`date`" + ` >= DATE_SUB(CURDATE(), INTERVAL 6 DAY) THEN shippedRevenueAmount END) AS sales_revenue_7d,
         SUM(CASE WHEN ` + "`date`" + ` >= DATE_SUB(CURDATE(), INTERVAL 29 DAY) THEN shippedRevenueAmount END) AS sales_revenue_30d,
         MAX(` + "`date`" + `) AS latest_date, MAX(synced_at) AS latest_sync_at
    FROM ls_vc_sales_report
   GROUP BY account_id, sid, asin
) sales ON sales.account_id = v.account_id AND sales.sid = v.vc_store_id AND sales.asin = v.asin
LEFT JOIN (
  SELECT account_id, sid, asin, MAX(LEFT(` + "`date`" + `, 10)) AS latest_date,
         SUBSTRING_INDEX(GROUP_CONCAT(sellableOnHandInventoryUnits ORDER BY LEFT(` + "`date`" + `, 10) DESC), ',', 1) AS sellable_inventory,
         SUBSTRING_INDEX(GROUP_CONCAT(netReceivedInventoryUnits ORDER BY LEFT(` + "`date`" + `, 10) DESC), ',', 1) AS inbound_inventory,
         SUBSTRING_INDEX(GROUP_CONCAT(unsellableOnHandInventoryUnits ORDER BY LEFT(` + "`date`" + `, 10) DESC), ',', 1) AS unfulfillable_inventory,
         SUBSTRING_INDEX(GROUP_CONCAT(aged90PlusDaysSellableInventoryUnits ORDER BY LEFT(` + "`date`" + `, 10) DESC), ',', 1) AS aged90_sellable_inventory,
         SUBSTRING_INDEX(GROUP_CONCAT(unhealthyInventoryUnits ORDER BY LEFT(` + "`date`" + `, 10) DESC), ',', 1) AS unhealthy_inventory,
         MAX(synced_at) AS latest_sync_at
    FROM ls_vc_inventory
   GROUP BY account_id, sid, asin
) inventory ON inventory.account_id = v.account_id AND inventory.sid = v.vc_store_id AND inventory.asin = v.asin
LEFT JOIN (
  SELECT account_id, sid, asin, MAX(LEFT(` + "`date`" + `, 10)) AS latest_date,
         SUBSTRING_INDEX(GROUP_CONCAT(glanceViews ORDER BY LEFT(` + "`date`" + `, 10) DESC), ',', 1) AS glance_views,
         MAX(synced_at) AS latest_sync_at
    FROM ls_vc_traffic GROUP BY account_id, sid, asin
) traffic ON traffic.account_id = v.account_id AND traffic.sid = v.vc_store_id AND traffic.asin = v.asin
LEFT JOIN (
  SELECT account_id, sid, asin, MAX(` + "`date`" + `) AS latest_date, SUBSTRING_INDEX(GROUP_CONCAT(netPureProductMargin ORDER BY ` + "`date`" + ` DESC), ',', 1) AS net_ppm, MAX(synced_at) AS latest_sync_at
    FROM ls_vc_margin GROUP BY account_id, sid, asin
) margin ON margin.account_id = v.account_id AND margin.sid = v.vc_store_id AND margin.asin = v.asin
LEFT JOIN (
  SELECT account_id, sid, asin, MAX(localEndTime) AS latest_realtime_end_at,
         SUBSTRING_INDEX(GROUP_CONCAT(orderedUnits ORDER BY localEndTime DESC), ',', 1) AS realtime_ordered_units,
         SUBSTRING_INDEX(GROUP_CONCAT(orderedRevenue ORDER BY localEndTime DESC), ',', 1) AS realtime_ordered_revenue,
         MAX(synced_at) AS latest_sync_at
    FROM ls_vc_realtime_sales GROUP BY account_id, sid, asin
) realtime ON realtime.account_id = v.account_id AND realtime.sid = v.vc_store_id AND realtime.asin = v.asin`,
	storeColumn:     "v.vc_store_id",
	baseColumns:     []string{"v.account_id", "v.vc_store_id", "v.asin", "''", "DATE(v.synced_at)", "GREATEST(v.synced_at, COALESCE(sales.latest_sync_at, v.synced_at), COALESCE(inventory.latest_sync_at, v.synced_at), COALESCE(traffic.latest_sync_at, v.synced_at), COALESCE(margin.latest_sync_at, v.synced_at), COALESCE(realtime.latest_sync_at, v.synced_at))", "CONCAT_WS('|', v.account_id, v.vc_store_id, v.asin)"},
	dateColumn:      "DATE(v.synced_at)",
	stableKeyColumn: "CONCAT_WS('|', v.account_id, v.vc_store_id, v.asin)",
	updatedAtColumn: "GREATEST(v.synced_at, COALESCE(sales.latest_sync_at, v.synced_at), COALESCE(inventory.latest_sync_at, v.synced_at), COALESCE(traffic.latest_sync_at, v.synced_at), COALESCE(margin.latest_sync_at, v.synced_at), COALESCE(realtime.latest_sync_at, v.synced_at))",
	stableKeyParts:  3,
	fields: map[string]string{
		"store_name":                    "s.store_name",
		"country":                       "s.country",
		"channel_type":                  "'vc'",
		"asin":                          "v.asin",
		"parent_asin":                   "v.parent_asin",
		"msku":                          "v.msku",
		"title":                         "v.item_name",
		"brand":                         "v.brand_id",
		"image_url":                     "v.small_min_image_url",
		"quantity_7d":                   "sales.sales_quantity_7d",
		"quantity_30d":                  "sales.sales_quantity_30d",
		"revenue_30d":                   "sales.sales_revenue_30d",
		"revenue_currency":              "CAST(NULL AS CHAR)",
		"returns_30d":                   "CAST(NULL AS SIGNED)",
		"inventory":                     "inventory.sellable_inventory",
		"rating":                        "v.stars",
		"reviews_count":                 "v.reviews_num",
		"ad_orders_7d":                  "CAST(NULL AS SIGNED)",
		"ad_spend_30d":                  "CAST(NULL AS DECIMAL(20,6))",
		"ad_sales_30d":                  "CAST(NULL AS DECIMAL(20,6))",
		"ad_spend_currency":             "CAST(NULL AS CHAR)",
		"sales_quantity_7d":             "sales.sales_quantity_7d",
		"sales_quantity_30d":            "sales.sales_quantity_30d",
		"sales_revenue_7d":              "sales.sales_revenue_7d",
		"sales_revenue_30d":             "sales.sales_revenue_30d",
		"sales_sparkline_7d":            "CAST(NULL AS JSON)",
		"sales_revenue_sparkline_7d":    "CAST(NULL AS JSON)",
		"realtime_revenue_sparkline_7d": "CAST(NULL AS JSON)",
		"ad_spend_sparkline_7d":         "CAST(NULL AS JSON)",
		"sellable_inventory":            "inventory.sellable_inventory",
		"inbound_inventory":             "inventory.inbound_inventory",
		"unfulfillable_inventory":       "inventory.unfulfillable_inventory",
		"aged90_sellable_inventory":     "inventory.aged90_sellable_inventory",
		"unhealthy_inventory":           "inventory.unhealthy_inventory",
		"latest_inventory_date":         "inventory.latest_date",
		"realtime_ordered_units":        "realtime.realtime_ordered_units",
		"realtime_ordered_revenue":      "realtime.realtime_ordered_revenue",
		"latest_realtime_end_at":        "realtime.latest_realtime_end_at",
		"glance_views":                  "traffic.glance_views",
		"net_ppm":                       "margin.net_ppm",
		"latest_margin_date":            "margin.latest_date",
		"latest_date":                   "COALESCE(sales.latest_date, inventory.latest_date, DATE(v.synced_at))",
		"latest_sync_at":                "GREATEST(v.synced_at, COALESCE(sales.latest_sync_at, v.synced_at), COALESCE(inventory.latest_sync_at, v.synced_at), COALESCE(traffic.latest_sync_at, v.synced_at), COALESCE(margin.latest_sync_at, v.synced_at), COALESCE(realtime.latest_sync_at, v.synced_at))",
		"sales_latest_date":             "sales.latest_date",
		"sales_latest_sync_at":          "sales.latest_sync_at",
		"vc_sales_covered_dates":        "CAST(NULL AS CHAR)",
		"visibility_status":             "CASE WHEN COALESCE(v.status, 0) = 1 THEN 'active' ELSE CAST(v.status AS CHAR) END",
	},
}

var operationsLogDefinition = detailReaderDefinition{
	fromClause: `listing_daily_metrics m
JOIN listing_dimensions d ON d.id = m.listing_dimension_id`,
	storeColumn:     "d.store_id",
	baseColumns:     []string{"''", "d.store_id", "d.asin", "d.sku", "m.business_date", "m.updated_at", "CONCAT_WS('|', d.store_id, d.channel, d.asin, COALESCE(NULLIF(d.sku, ''), '-'), DATE_FORMAT(m.business_date, '%Y-%m-%d'))"},
	dateColumn:      "m.business_date",
	stableKeyColumn: "CONCAT_WS('|', d.store_id, d.channel, d.asin, COALESCE(NULLIF(d.sku, ''), '-'), DATE_FORMAT(m.business_date, '%Y-%m-%d'))",
	updatedAtColumn: "m.updated_at",
	stableKeyParts:  5,
	fields: map[string]string{
		"channel_type":            "d.channel",
		"asin":                    "d.asin",
		"listing_sku":             "d.sku",
		"sales_units":             "m.sales_units",
		"sales_amount":            "m.sales_amount",
		"returns_qty":             "m.returns_qty",
		"inventory_sellable":      "m.inventory_sellable",
		"inventory_inbound":       "m.inventory_inbound",
		"inventory_reserved":      "m.inventory_reserved",
		"inventory_unfulfillable": "m.inventory_unfulfillable",
		"sessions_total":          "m.sessions_total",
		"sessions_desktop":        "m.sessions_desktop",
		"sessions_mobile":         "m.sessions_mobile",
		"rating":                  "m.rating",
		"review_count":            "m.review_count",
		"sp_spend":                "m.sp_spend",
		"sp_sales":                "m.sp_sales",
		"sp_orders":               "m.sp_orders",
		"sd_spend":                "m.sd_spend",
		"sd_sales":                "m.sd_sales",
		"sd_orders":               "m.sd_orders",
		"hsa_spend":               "m.hsa_spend",
		"hsa_sales":               "m.hsa_sales",
		"hsa_orders":              "m.hsa_orders",
		"sb_spend":                "m.sb_spend",
		"sb_sales":                "m.sb_sales",
		"sb_orders":               "m.sb_orders",
		"is_provisional":          "m.is_provisional",
		"is_verified":             "m.is_verified",
	},
}

func NewReturnReasonDetailReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, returnReasonDetailDefinition)
}

func NewOrderShippingAddressDetailReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, orderShippingAddressDetailDefinition)
}

func NewAddressOrderItemDetailReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, addressOrderItemDetailDefinition)
}

func NewFBAInventorySnapshotReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, fbaInventorySnapshotDefinition)
}

func NewVCPODetailReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, vcPODetailDefinition)
}

func NewFBALinksReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, fbaLinksDefinition)
}

func NewVCLinksReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, vcLinksDefinition)
}

func NewOperationsLogReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, operationsLogDefinition)
}

func newDetailSQLReader(db *sqlx.DB, definition detailReaderDefinition) *DetailSQLReader {
	if db == nil {
		return &DetailSQLReader{definition: definition}
	}
	return &DetailSQLReader{queryer: sqlxQueryer{db: db}, definition: definition}
}

func (r *DetailSQLReader) Snapshot(ctx context.Context, query Query) (Page, error) {
	if query.DateFrom == "" || query.DateTo == "" {
		return Page{}, fmt.Errorf("snapshot date range is required")
	}
	return r.read(ctx, query, true)
}

func (r *DetailSQLReader) Changes(ctx context.Context, query Query) (Page, error) {
	if query.Cursor == nil {
		return Page{}, fmt.Errorf("changes cursor is required")
	}
	return r.read(ctx, query, false)
}

func (r *DetailSQLReader) read(ctx context.Context, query Query, snapshot bool) (Page, error) {
	if r == nil || r.queryer == nil {
		return Page{}, fmt.Errorf("registered detail SQL reader is not configured")
	}
	if query.PageSize < 1 {
		return Page{}, fmt.Errorf("positive page size is required")
	}
	fields, err := r.selectedFields(query.Fields)
	if err != nil {
		return Page{}, err
	}
	if query.Cursor != nil && !r.validStableKey(query.Cursor.StableKey) {
		return Page{}, fmt.Errorf("detail cursor stable key is invalid")
	}
	selectColumns := append([]string(nil), r.definition.baseColumns...)
	selectColumns = append(selectColumns, fields...)
	fromClause := r.definition.fromClause
	if fromClause == "" {
		fromClause = r.definition.sourceTable + " " + r.definition.alias
	}
	storeColumn := r.definition.storeColumn
	if storeColumn == "" {
		storeColumn = r.definition.alias + ".sid"
	}
	queryText := "SELECT " + strings.Join(selectColumns, ", ") + " FROM " + fromClause
	where := make([]string, 0, 3)
	args := make([]any, 0, 6)
	where, args = appendStoreFilter(where, args, storeColumn, query)
	if snapshot {
		where = append(where, r.definition.dateColumn+" BETWEEN ? AND ?")
		args = append(args, query.DateFrom, query.DateTo)
	}
	if query.Cursor != nil {
		where = append(where, "("+r.definition.updatedAtColumn+" > ? OR ("+r.definition.updatedAtColumn+" = ? AND "+r.definition.stableKeyColumn+" > ?))")
		args = append(args, query.Cursor.UpdatedAt, query.Cursor.UpdatedAt, query.Cursor.StableKey)
	}
	queryText += " WHERE " + strings.Join(where, " AND ")
	queryText += " ORDER BY " + r.definition.updatedAtColumn + " ASC, " + r.definition.stableKeyColumn + " ASC LIMIT ?"
	args = append(args, query.PageSize+1)
	rows, err := r.queryer.Query(ctx, queryText, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{}
	for rows.Next() {
		row, err := scanDetailRow(rows, query.Fields)
		if err != nil {
			return Page{}, err
		}
		page.Rows = append(page.Rows, row)
		if len(page.Rows) > query.PageSize {
			page.HasMore = true
			page.Rows = page.Rows[:query.PageSize]
			break
		}
	}
	if err := rows.Err(); err != nil {
		return Page{}, err
	}
	if len(page.Rows) > 0 && page.HasMore {
		last := page.Rows[len(page.Rows)-1]
		page.Next = &CursorKey{UpdatedAt: last.UpdatedAt, StableKey: last.StableKey}
	}
	return page, nil
}

func (r *DetailSQLReader) selectedFields(fields []string) ([]string, error) {
	selected := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		column, ok := r.definition.fields[field]
		if !ok {
			return nil, fmt.Errorf("dataset field %q is not supported by registered detail SQL reader", field)
		}
		if _, exists := seen[field]; exists {
			return nil, fmt.Errorf("dataset field %q is duplicated", field)
		}
		seen[field] = struct{}{}
		selected = append(selected, column+" AS `"+field+"`")
	}
	return selected, nil
}

func (r *DetailSQLReader) validStableKey(stableKey string) bool {
	parts := strings.Split(stableKey, "|")
	if len(parts) != r.definition.stableKeyParts {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
	}
	return true
}

func scanDetailRow(rows SQLRows, fields []string) (Row, error) {
	var accountID, store, asin, sku, businessDate, stableKey sql.NullString
	var updatedAt sql.NullTime
	dest := []any{&accountID, &store, &asin, &sku, &businessDate, &updatedAt, &stableKey}
	values := make([]any, len(fields))
	for i := range fields {
		dest = append(dest, &values[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return Row{}, err
	}
	if !updatedAt.Valid || !stableKey.Valid || stableKey.String == "" {
		return Row{}, fmt.Errorf("detail row has invalid synced_at/stable key")
	}
	row := Row{
		AccountID:    accountID.String,
		Store:        store.String,
		ASIN:         asin.String,
		SKU:          sku.String,
		BusinessDate: businessDate.String,
		UpdatedAt:    updatedAt.Time.UTC(),
		StableKey:    stableKey.String,
		FixedValues: map[string]any{
			"store":       store.String,
			"record_date": businessDate.String,
			"stable_key":  stableKey.String,
			"updated_at":  updatedAt.Time.UTC().Format(time.RFC3339Nano),
		},
		Values: make(map[string]any, len(fields)),
	}
	for i, field := range fields {
		row.Values[field] = normalizeSQLValue(values[i])
	}
	return row, nil
}
