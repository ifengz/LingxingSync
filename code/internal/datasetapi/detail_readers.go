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
	alias           string
	baseColumns     []string
	dateColumn      string
	stableKeyColumn string
	fields          map[string]string
	stableKeyParts  int
}

var returnReasonDetailDefinition = detailReaderDefinition{
	sourceTable:     "ls_sc_refunds",
	alias:           "r",
	baseColumns:     []string{"r.account_id", "r.sid", "r.asin", "COALESCE(NULLIF(r.sku, ''), NULLIF(r.local_sku, ''))", "r.return_date_locale", "r.synced_at", "CONCAT_WS('|', r.account_id, r.sid, r.license_plate_number)"},
	dateColumn:      "r.return_date_locale",
	stableKeyColumn: "CONCAT_WS('|', r.account_id, r.sid, r.license_plate_number)",
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

var fbaInventorySnapshotDefinition = detailReaderDefinition{
	sourceTable:     "ls_fba_inventory",
	alias:           "i",
	baseColumns:     []string{"i.account_id", "i.sid", "i.asin", "i.sku", "DATE(i.synced_at)", "i.synced_at", "CONCAT_WS('|', i.account_id, i.sid, i.fnsku)"},
	dateColumn:      "DATE(i.synced_at)",
	stableKeyColumn: "CONCAT_WS('|', i.account_id, i.sid, i.fnsku)",
	stableKeyParts:  3,
	fields: map[string]string{
		"fnsku":                             "i.fnsku",
		"msku":                              "i.msku",
		"asin":                              "i.asin",
		"sku":                               "i.sku",
		"product_name":                      "COALESCE(NULLIF(i.product_name, ''), i.name)",
		"fulfillable_quantity":              "i.afn_fulfillable_quantity",
		"inbound_receiving_quantity":        "i.afn_inbound_receiving_quantity",
		"inbound_shipped_quantity":          "i.afn_inbound_shipped_quantity",
		"inbound_working_quantity":          "i.afn_inbound_working_quantity",
		"reserved_quantity":                 "i.afn_reserved_quantity",
		"unsellable_quantity":               "i.afn_unsellable_quantity",
		"inv_age_0_to_30_days":              "i.inv_age_0_to_30_days",
		"inv_age_31_to_60_days":             "i.inv_age_31_to_60_days",
		"inv_age_61_to_90_days":             "i.inv_age_61_to_90_days",
		"inv_age_91_to_180_days":            "i.inv_age_91_to_180_days",
		"inv_age_181_to_270_days":           "i.inv_age_181_to_270_days",
		"inv_age_271_to_365_days":           "i.inv_age_271_to_365_days",
		"inv_age_365_plus_days":             "i.inv_age_365_plus_days",
		"stock_cost_total":                  "i.stock_cost_total",
		"sell_through":                      "i.sell_through",
		"historical_days_of_supply":         "i.historical_days_of_supply",
		"fba_inventory_level_health_status": "i.fba_inventory_level_health_status",
	},
}

func NewReturnReasonDetailReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, returnReasonDetailDefinition)
}

func NewOrderShippingAddressDetailReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, orderShippingAddressDetailDefinition)
}

func NewFBAInventorySnapshotReader(db *sqlx.DB) *DetailSQLReader {
	return newDetailSQLReader(db, fbaInventorySnapshotDefinition)
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
	if query.Store == "" || query.PageSize < 1 {
		return Page{}, fmt.Errorf("store and positive page size are required")
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
	queryText := "SELECT " + strings.Join(selectColumns, ", ") + " FROM " + r.definition.sourceTable + " " + r.definition.alias + " WHERE " + r.definition.alias + ".sid = ?"
	args := []any{query.Store}
	if snapshot {
		queryText += " AND " + r.definition.dateColumn + " BETWEEN ? AND ?"
		args = append(args, query.DateFrom, query.DateTo)
	}
	if query.Cursor != nil {
		queryText += " AND (" + r.definition.alias + ".synced_at > ? OR (" + r.definition.alias + ".synced_at = ? AND " + r.definition.stableKeyColumn + " > ?))"
		args = append(args, query.Cursor.UpdatedAt, query.Cursor.UpdatedAt, query.Cursor.StableKey)
	}
	queryText += " ORDER BY " + r.definition.alias + ".synced_at ASC, " + r.definition.stableKeyColumn + " ASC LIMIT ?"
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
