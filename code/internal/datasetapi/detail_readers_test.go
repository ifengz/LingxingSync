package datasetapi

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestReturnReasonDetailReaderSnapshotUsesFixedRefundSchema(t *testing.T) {
	updated := time.Date(2026, 8, 15, 3, 4, 5, 0, time.UTC)
	queryer := &fixedQueryer{rows: &fixedRows{values: []any{
		"sc-us-1", "store-a", "ASIN1", "SKU1", "2026-08-14", updated, "sc-us-1|store-a|refund-1", "refund-1", int64(2), "damaged",
	}}}
	reader := &DetailSQLReader{queryer: queryer, definition: returnReasonDetailDefinition}
	page, err := reader.Snapshot(context.Background(), Query{
		Store: "store-a", DateFrom: "2026-08-01", DateTo: "2026-08-15", Fields: []string{"license_plate_number", "quantity", "reason"}, PageSize: 1,
	})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.Contains(queryer.query, "FROM ls_sc_refunds r") || !strings.Contains(queryer.query, "r.return_date_locale BETWEEN ? AND ?") {
		t.Fatalf("refund query must use the fixed raw source and date filter: %s", queryer.query)
	}
	if strings.Contains(queryer.query, "internal_secret") || len(queryer.args) != 4 {
		t.Fatalf("refund query leaked arbitrary SQL or unexpected args: query=%s args=%#v", queryer.query, queryer.args)
	}
	if len(page.Rows) != 1 || page.Rows[0].StableKey != "sc-us-1|store-a|refund-1" || page.Rows[0].Values["reason"] != "damaged" {
		t.Fatalf("refund row mismatch: %+v", page)
	}
	if page.Rows[0].FixedValues["record_date"] != "2026-08-14" || page.Rows[0].FixedValues["stable_key"] != "sc-us-1|store-a|refund-1" {
		t.Fatalf("refund fixed fields mismatch: %+v", page.Rows[0].FixedValues)
	}
}

func TestOrderShippingAddressReaderChangesUsesStaticKeyset(t *testing.T) {
	updated := time.Date(2026, 8, 15, 3, 4, 5, 0, time.UTC)
	queryer := &fixedQueryer{rows: &fixedRows{values: []any{
		"sc-us-1", "store-a", "", "SKU1", "2026-08-14", updated, "sc-us-1|store-a|shipment-1|item-1", "ORDER-1", "City", "US",
	}}}
	reader := &DetailSQLReader{queryer: queryer, definition: orderShippingAddressDetailDefinition}
	page, err := reader.Changes(context.Background(), Query{
		Store: "store-a", Fields: []string{"amazon_order_id", "ship_city", "ship_country"}, PageSize: 10,
		Cursor: &CursorKey{UpdatedAt: updated, StableKey: "sc-us-1|store-a|shipment-0|item-0"},
	})
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if !strings.Contains(queryer.query, "FROM ls_sc_fba_order_addresses a") || !strings.Contains(queryer.query, "a.synced_at > ? OR (a.synced_at = ?") || !strings.Contains(queryer.query, "ORDER BY a.synced_at ASC") {
		t.Fatalf("address changes must use a fixed keyset query: %s", queryer.query)
	}
	if len(queryer.args) != 5 || queryer.args[1] != updated || queryer.args[2] != updated || queryer.args[3] != "sc-us-1|store-a|shipment-0|item-0" {
		t.Fatalf("address keyset args mismatch: %#v", queryer.args)
	}
	if len(page.Rows) != 1 || page.Rows[0].Values["ship_country"] != "US" {
		t.Fatalf("address row mismatch: %+v", page)
	}
}

func TestFBAInventorySnapshotReaderUsesCurrentStateDate(t *testing.T) {
	updated := time.Date(2026, 8, 15, 3, 4, 5, 0, time.UTC)
	queryer := &fixedQueryer{rows: &fixedRows{values: []any{
		"sc-us-1", "store-a", "ASIN1", "SKU1", "2026-08-15", updated, "sc-us-1|store-a|FNSKU1", "FNSKU1", int64(7), int64(3),
	}}}
	reader := &DetailSQLReader{queryer: queryer, definition: fbaInventorySnapshotDefinition}
	page, err := reader.Snapshot(context.Background(), Query{
		Store: "store-a", DateFrom: "2026-08-15", DateTo: "2026-08-15", Fields: []string{"fnsku", "fulfillable_quantity", "inv_age_0_to_30_days"}, PageSize: 1,
	})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.Contains(queryer.query, "FROM ls_fba_inventory i") || !strings.Contains(queryer.query, "DATE(i.synced_at) BETWEEN ? AND ?") {
		t.Fatalf("inventory query must expose only the current state snapshot: %s", queryer.query)
	}
	if len(page.Rows) != 1 || page.Rows[0].Values["fulfillable_quantity"] != int64(7) {
		t.Fatalf("inventory row mismatch: %+v", page)
	}
}

func TestDetailReaderRejectsUnknownFieldBeforeQuery(t *testing.T) {
	queryer := &fixedQueryer{rows: &fixedRows{}}
	reader := &DetailSQLReader{queryer: queryer, definition: returnReasonDetailDefinition}
	_, err := reader.Snapshot(context.Background(), Query{
		Store: "store-a", DateFrom: "2026-08-01", DateTo: "2026-08-01", Fields: []string{"internal_secret"}, PageSize: 1,
	})
	if err == nil {
		t.Fatal("unknown field was accepted")
	}
	if queryer.query != "" {
		t.Fatalf("unknown field reached SQL query: %s", queryer.query)
	}
}
