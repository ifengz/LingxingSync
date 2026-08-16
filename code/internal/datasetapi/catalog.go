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
	},
	"fba-inventory-snapshot-v1": {
		ID: "fba-inventory-snapshot-v1", Name: "FBA 库存快照表", Kind: DatasetKindSnapshot,
		Source: "ls_fba_inventory", Grain: "store + fnsku",
		InitialCursor: "0|0|0",
		FixedFields:   []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"fnsku", "msku", "asin", "sku", "product_name", "fulfillable_quantity", "unsellable_quantity", "reserved_quantity", "inbound_receiving_quantity", "inbound_shipped_quantity", "inbound_working_quantity", "inv_age_0_to_30_days", "inv_age_31_to_60_days", "inv_age_61_to_90_days", "inv_age_91_to_180_days", "inv_age_181_to_270_days", "inv_age_271_to_365_days", "inv_age_365_plus_days", "stock_cost_total", "sell_through", "historical_days_of_supply", "fba_inventory_level_health_status"},
	},
	"order-shipping-address-detail-v1": {
		ID: "order-shipping-address-detail-v1", Name: "订单配送地址明细表", Kind: DatasetKindDetail,
		Source: "ls_sc_fba_order_addresses", Grain: "store + shipment_id + shipment_item_id",
		InitialCursor: "0|0|0|0",
		FixedFields:   []string{"store", "record_date", "stable_key", "updated_at"},
		Fields:        []string{"shipment_id", "shipment_item_id", "amazon_order_id", "amazon_order_item_id", "purchase_date", "payments_date", "shipment_date", "reporting_date", "estimated_arrival_date", "sku", "product_name", "quantity_shipped", "currency", "item_price", "item_tax", "shipping_price", "shipping_tax", "gift_wrap_price", "gift_wrap_tax", "ship_service_level", "item_promotion_discount", "ship_promotion_discount", "carrier", "tracking_number", "fulfillment_channel", "points_granted", "hide_time", "ship_city", "ship_state", "ship_postal_code", "ship_country"},
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
	return definition
}
