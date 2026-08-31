package datasetapi

import (
	"context"
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
)

// SQLRows and SQLQueryer keep the reader testable without exposing a generic
// SQL endpoint. Production uses the sqlx adapter below; tests provide fixed
// rows and can assert the generated statement and arguments.
type SQLRows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close() error
}

type SQLQueryer interface {
	Query(context.Context, string, ...any) (SQLRows, error)
}

type SQLReader struct {
	queryer SQLQueryer
}

type sqlxQueryer struct{ db *sqlx.DB }

func (q sqlxQueryer) Query(ctx context.Context, query string, args ...any) (SQLRows, error) {
	return q.db.QueryxContext(ctx, query, args...)
}

func NewSQLReader(db *sqlx.DB) *SQLReader {
	if db == nil {
		return &SQLReader{}
	}
	return &SQLReader{queryer: sqlxQueryer{db: db}}
}

var metricColumns = map[string]string{
	"sales_units": "m.sales_units", "sales_units_source": "m.sales_units_source",
	"sales_amount": "m.sales_amount", "sales_amount_source": "m.sales_amount_source",
	"returns_qty": "m.returns_qty", "returns_qty_source": "m.returns_qty_source",
	"inventory_sellable": "m.inventory_sellable", "inventory_sellable_source": "m.inventory_sellable_source",
	"inventory_inbound": "m.inventory_inbound", "inventory_inbound_source": "m.inventory_inbound_source",
	"inventory_reserved": "m.inventory_reserved", "inventory_reserved_source": "m.inventory_reserved_source",
	"inventory_unfulfillable": "m.inventory_unfulfillable", "inventory_unfulfillable_source": "m.inventory_unfulfillable_source",
	"inventory_local_warehouse": "m.inventory_local_warehouse", "inventory_local_warehouse_source": "m.inventory_local_warehouse_source",
	"inventory_unhealthy_units": "m.inventory_unhealthy_units", "inventory_unhealthy_units_source": "m.inventory_unhealthy_units_source",
	"inventory_aged90_sellable_units": "m.inventory_aged90_sellable_units", "inventory_aged90_sellable_units_source": "m.inventory_aged90_sellable_units_source",
	"inventory_sell_through_rate": "m.inventory_sell_through_rate", "inventory_sell_through_rate_source": "m.inventory_sell_through_rate_source",
	"inventory_receive_fill_rate": "m.inventory_receive_fill_rate", "inventory_receive_fill_rate_source": "m.inventory_receive_fill_rate_source",
	"inventory_vendor_confirmation_rate": "m.inventory_vendor_confirmation_rate", "inventory_vendor_confirmation_rate_source": "m.inventory_vendor_confirmation_rate_source",
	"inventory_avg_lead_time_days": "m.inventory_avg_lead_time_days", "inventory_avg_lead_time_days_source": "m.inventory_avg_lead_time_days_source",
	"inventory_sellable_cost": "m.inventory_sellable_cost", "inventory_sellable_cost_source": "m.inventory_sellable_cost_source",
	"inventory_unfulfillable_cost": "m.inventory_unfulfillable_cost", "inventory_unfulfillable_cost_source": "m.inventory_unfulfillable_cost_source",
	"inventory_aged90_cost": "m.inventory_aged90_cost", "inventory_aged90_cost_source": "m.inventory_aged90_cost_source",
	"inventory_unhealthy_cost": "m.inventory_unhealthy_cost", "inventory_unhealthy_cost_source": "m.inventory_unhealthy_cost_source",
	"inventory_inbound_cost": "m.inventory_inbound_cost", "inventory_inbound_cost_source": "m.inventory_inbound_cost_source",
	"inventory_currency": "m.inventory_currency", "inventory_currency_source": "m.inventory_currency_source",
	"inventory_inbound_receiving": "m.inventory_inbound_receiving", "inventory_inbound_receiving_source": "m.inventory_inbound_receiving_source",
	"inventory_inbound_shipped": "m.inventory_inbound_shipped", "inventory_inbound_shipped_source": "m.inventory_inbound_shipped_source",
	"inventory_inbound_working": "m.inventory_inbound_working", "inventory_inbound_working_source": "m.inventory_inbound_working_source",
	"inventory_reserved_customer_orders": "m.inventory_reserved_customer_orders", "inventory_reserved_customer_orders_source": "m.inventory_reserved_customer_orders_source",
	"inventory_reserved_fc_processing": "m.inventory_reserved_fc_processing", "inventory_reserved_fc_processing_source": "m.inventory_reserved_fc_processing_source",
	"inventory_reserved_fc_transfers": "m.inventory_reserved_fc_transfers", "inventory_reserved_fc_transfers_source": "m.inventory_reserved_fc_transfers_source",
	"sessions_desktop": "m.sessions_desktop", "sessions_desktop_source": "m.sessions_desktop_source",
	"sessions_mobile": "m.sessions_mobile", "sessions_mobile_source": "m.sessions_mobile_source",
	"sessions_total": "m.sessions_total", "sessions_total_source": "m.sessions_total_source",
	"review_count": "m.review_count", "review_count_source": "m.review_count_source",
	"rating": "m.rating", "rating_source": "m.rating_source",
	"sp_spend": "m.sp_spend", "sp_spend_source": "m.sp_spend_source", "sp_sales": "m.sp_sales", "sp_sales_source": "m.sp_sales_source", "sp_orders": "m.sp_orders", "sp_orders_source": "m.sp_orders_source",
	"sd_spend": "m.sd_spend", "sd_spend_source": "m.sd_spend_source", "sd_sales": "m.sd_sales", "sd_sales_source": "m.sd_sales_source", "sd_orders": "m.sd_orders", "sd_orders_source": "m.sd_orders_source",
	"hsa_spend": "m.hsa_spend", "hsa_spend_source": "m.hsa_spend_source", "hsa_sales": "m.hsa_sales", "hsa_sales_source": "m.hsa_sales_source", "hsa_orders": "m.hsa_orders", "hsa_orders_source": "m.hsa_orders_source",
	"sb_spend": "m.sb_spend", "sb_spend_source": "m.sb_spend_source", "sb_sales": "m.sb_sales", "sb_sales_source": "m.sb_sales_source", "sb_orders": "m.sb_orders", "sb_orders_source": "m.sb_orders_source",
	"is_provisional": "m.is_provisional", "is_verified": "m.is_verified", "verified_fields": "m.verified_fields", "report_verified_at": "m.report_verified_at",
}

func (r *SQLReader) Snapshot(ctx context.Context, query Query) (Page, error) {
	if query.DateFrom == "" || query.DateTo == "" {
		return Page{}, fmt.Errorf("snapshot date range is required")
	}
	return r.read(ctx, query, true)
}

func (r *SQLReader) Changes(ctx context.Context, query Query) (Page, error) {
	if query.Cursor == nil {
		return Page{}, fmt.Errorf("changes cursor is required")
	}
	return r.read(ctx, query, false)
}

func (r *SQLReader) read(ctx context.Context, query Query, snapshot bool) (Page, error) {
	if r == nil || r.queryer == nil {
		return Page{}, fmt.Errorf("listing daily SQL reader is not configured")
	}
	if query.PageSize < 1 {
		return Page{}, fmt.Errorf("positive page size is required")
	}
	fieldNames := append([]string(nil), query.Fields...)
	fields, err := fixedMetricFields(fieldNames)
	if err != nil {
		return Page{}, err
	}
	selectColumns := "d.store_id, d.channel, d.identity_scope, d.asin, d.sku, m.business_date, m.updated_at, m.is_provisional, m.is_verified, m.verified_fields, m.report_verified_at, m.listing_dimension_id"
	if len(fields) > 0 {
		selectColumns += ", " + strings.Join(fields, ", ")
	}
	queryText := "SELECT " + selectColumns + " FROM listing_daily_metrics m JOIN listing_dimensions d ON d.id = m.listing_dimension_id"
	where := make([]string, 0, 3)
	args := make([]any, 0, 6)
	where, args = appendStoreFilter(where, args, "d.store_id", query)
	if snapshot {
		where = append(where, "m.business_date BETWEEN ? AND ?")
		args = append(args, query.DateFrom, query.DateTo)
	}
	if query.Cursor != nil {
		dimensionID, date, err := parseStableKey(query.Cursor.StableKey)
		if err != nil {
			return Page{}, err
		}
		where = append(where, "(m.updated_at > ? OR (m.updated_at = ? AND (m.listing_dimension_id > ? OR (m.listing_dimension_id = ? AND m.business_date > ?))))")
		args = append(args, query.Cursor.UpdatedAt, query.Cursor.UpdatedAt, dimensionID, dimensionID, date)
	}
	queryText += " WHERE " + strings.Join(where, " AND ")
	queryText += " ORDER BY m.updated_at ASC, m.listing_dimension_id ASC, m.business_date ASC LIMIT ?"
	args = append(args, query.PageSize+1)
	rows, err := r.queryer.Query(ctx, queryText, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	page := Page{}
	for rows.Next() {
		row, err := scanSQLRow(rows, fieldNames)
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

func appendStoreFilter(where []string, args []any, column string, query Query) ([]string, []any) {
	stores := queryStores(query)
	if len(stores) == 0 {
		return where, args
	}
	where = append(where, column+" IN ("+placeholders(len(stores))+")")
	for _, store := range stores {
		args = append(args, store)
	}
	return where, args
}

func queryStores(query Query) []string {
	stores := make([]string, 0, len(query.Stores)+1)
	seen := make(map[string]struct{}, len(query.Stores)+1)
	for _, raw := range append(append([]string(nil), query.Stores...), query.Store) {
		store := strings.TrimSpace(raw)
		if store == "" {
			continue
		}
		if _, ok := seen[store]; ok {
			continue
		}
		seen[store] = struct{}{}
		stores = append(stores, store)
	}
	return stores
}

func placeholders(count int) string {
	return strings.TrimRight(strings.Repeat("?,", count), ",")
}

func fixedMetricFields(fields []string) ([]string, error) {
	selected := make([]string, 0, len(fields))
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		column, ok := metricColumns[field]
		if !ok {
			return nil, fmt.Errorf("dataset field %q is not supported by listing SQL reader", field)
		}
		if _, exists := seen[field]; exists {
			return nil, fmt.Errorf("dataset field %q is duplicated", field)
		}
		seen[field] = struct{}{}
		selected = append(selected, column+" AS `"+field+"`")
	}
	return selected, nil
}

func parseStableKey(raw string) (uint64, string, error) {
	parts := strings.SplitN(raw, "|", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return 0, "", fmt.Errorf("listing cursor stable key is invalid")
	}
	id, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("listing cursor stable key is invalid")
	}
	if _, err := time.Parse("2006-01-02", parts[1]); err != nil {
		return 0, "", fmt.Errorf("listing cursor stable key date is invalid")
	}
	return id, parts[1], nil
}

func scanSQLRow(rows SQLRows, fields []string) (Row, error) {
	var storeID, channel, scope, asin, sku sql.NullString
	var businessDate, updatedAt sql.NullTime
	var provisional, verified sql.NullBool
	var verifiedFields, reportVerified any
	var dimensionID uint64
	dest := []any{&storeID, &channel, &scope, &asin, &sku, &businessDate, &updatedAt, &provisional, &verified, &verifiedFields, &reportVerified, &dimensionID}
	values := make([]any, len(fields))
	for i := range fields {
		dest = append(dest, &values[i])
	}
	if err := rows.Scan(dest...); err != nil {
		return Row{}, err
	}
	if !businessDate.Valid || !updatedAt.Valid {
		return Row{}, fmt.Errorf("listing row has invalid business_date/updated_at")
	}
	businessDateText := businessDate.Time.Format("2006-01-02")
	row := Row{Store: storeID.String, Channel: channel.String, ASIN: asin.String, SKU: sku.String, BusinessDate: businessDateText, UpdatedAt: updatedAt.Time.UTC(), StableKey: fmt.Sprintf("%d|%s", dimensionID, businessDateText), IsProvisional: provisional.Bool, VerificationStatus: verificationStatus(provisional, verified), Values: make(map[string]any, len(fields))}
	for i, field := range fields {
		row.Values[field] = normalizeSQLValue(values[i])
	}
	// 033 has no deletion column yet. The explicit nil keeps the response
	// contract honest; tombstones require a later schema contract, not a guess.
	return row, nil
}

func verificationStatus(provisional, verified sql.NullBool) string {
	if verified.Valid && verified.Bool {
		return "verified"
	}
	if provisional.Valid && provisional.Bool {
		return "provisional"
	}
	return "unverified"
}

func normalizeSQLValue(value any) any {
	if raw, ok := value.([]byte); ok {
		return string(raw)
	}
	return value
}
