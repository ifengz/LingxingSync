package worker

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFilterFBAInventoryRowsKeepsFBAAndDropsFBM(t *testing.T) {
	rows := []map[string]any{
		{"fulfillment_channel_name": "FBA", "fnsku": "X-FBA"},
		{"fulfillment_channel_name": "FBM", "fnsku": ""},
		{"fulfillment_channel_name": "FBM", "fnsku": "X-FBM"},
	}

	filtered, dropped, unkeyed, err := filterFBAInventoryRows("ls_fba_inventory", rows)
	if err != nil {
		t.Fatalf("filterFBAInventoryRows returned error: %v", err)
	}
	if dropped != 2 {
		t.Fatalf("dropped=%d, want 2", dropped)
	}
	if len(unkeyed) != 0 {
		t.Fatalf("unkeyed=%v, want none", unkeyed)
	}
	if len(filtered) != 1 || filtered[0]["fnsku"] != "X-FBA" {
		t.Fatalf("filtered=%v, want only FBA row", filtered)
	}
}

func TestFilterFBAInventoryRowsSkipsFBAWithoutFNSKU(t *testing.T) {
	rows := []map[string]any{
		{"fulfillment_channel_name": "FBA", "fnsku": "X-FBA"},
		{"fulfillment_channel_name": "FBA", "fnsku": "", "asin": "B0DEAD"},
		{"fulfillment_channel_name": "FBA", "asin": "B0MISSING"},
		{"fulfillment_channel_name": "FBM", "fnsku": ""},
	}

	filtered, dropped, unkeyed, err := filterFBAInventoryRows("ls_fba_inventory", rows)
	if err != nil {
		t.Fatalf("a blank FNSKU must skip one row, not fail the batch: %v", err)
	}
	if len(filtered) != 1 || filtered[0]["fnsku"] != "X-FBA" {
		t.Fatalf("filtered=%v, want only the keyed FBA row", filtered)
	}
	if dropped != 1 {
		t.Fatalf("dropped=%d, want 1 FBM row", dropped)
	}
	if len(unkeyed) != 2 {
		t.Fatalf("unkeyed=%v, want both blank and missing FNSKU rows", unkeyed)
	}
	if unkeyed[0]["asin"] != "B0DEAD" || unkeyed[1]["asin"] != "B0MISSING" {
		t.Fatalf("unkeyed must retain row content for task_logs: %v", unkeyed)
	}
}

func TestFilterFBAInventoryRowsRejectsUnknownChannel(t *testing.T) {
	rows := []map[string]any{{"fnsku": "X-1"}}
	_, _, _, err := filterFBAInventoryRows("ls_fba_inventory", rows)
	if err == nil || !strings.Contains(err.Error(), "fulfillment channel") {
		t.Fatalf("error=%v, want unknown fulfillment channel error", err)
	}
}

func TestFilterFBAInventoryRowsDoesNotAffectOtherTables(t *testing.T) {
	rows := []map[string]any{{"fnsku": "X-1"}}
	filtered, dropped, unkeyed, err := filterFBAInventoryRows("other_table", rows)
	if err != nil {
		t.Fatalf("filterFBAInventoryRows returned error: %v", err)
	}
	if dropped != 0 || unkeyed != nil || len(filtered) != 1 || filtered[0]["fnsku"] != "X-1" {
		t.Fatalf("filtered=%v dropped=%d unkeyed=%v, want unchanged", filtered, dropped, unkeyed)
	}
}

func unkeyedRowFixture(index int) map[string]any {
	return map[string]any{
		"sid": "3785", "asin": "B0HJCL5YB8", "sku": "", "msku": "", "fnsku": "",
		"fulfillment_channel_name":          "FBA",
		"afn_fulfillable_quantity":          0,
		"afn_reserved_quantity":             0,
		"afn_unsellable_quantity":           0,
		"afn_inbound_shipped_quantity":      0,
		"afn_inbound_working_quantity":      0,
		"afn_inbound_receiving_quantity":    0,
		"afn_researching_quantity":          0,
		"afn_erp_real_shipped_quantity":     0,
		"product_name":                      strings.Repeat("风扇配件", 12),
		"product_image":                     "https://m.media-amazon.com/images/I/" + strings.Repeat("x", 64) + ".jpg",
		"category_name":                     strings.Repeat("Home Improvement", 6),
		"brand_name":                        strings.Repeat("FLOWBREEZE", 6),
		"dimension_info":                    strings.Repeat(`{"length":10,"width":8}`, 8),
		"afn_fulfillable_quantity_multi":    []any{},
		"inv_age_0_to_30_days":              0,
		"estimated_storage_cost_next_month": 0,
		"row_index":                         index,
	}
}

func TestSummarizeUnkeyedRowsKeepsIdentityAndQuantities(t *testing.T) {
	projected := summarizeUnkeyedRows([]map[string]any{unkeyedRowFixture(1)})
	if len(projected) != 1 {
		t.Fatalf("projected=%v, want 1 row", projected)
	}
	for _, field := range []string{"sid", "asin", "fnsku", "fulfillment_channel_name", "afn_fulfillable_quantity"} {
		if _, ok := projected[0][field]; !ok {
			t.Fatalf("field %s must survive the projection, got %v", field, projected[0])
		}
	}
	for _, bulky := range []string{"product_name", "product_image", "dimension_info", "row_index"} {
		if _, ok := projected[0][bulky]; ok {
			t.Fatalf("bulky field %s must be dropped, got %v", bulky, projected[0])
		}
	}
}

func TestSummarizeUnkeyedRowsFitWorstProductionPage(t *testing.T) {
	// 生产实测最坏一页跳过 94 行（sc_us_2 / sid 3785 / page 3）。
	rows := make([]map[string]any, 0, 94)
	for i := 0; i < 94; i++ {
		rows = append(rows, unkeyedRowFixture(i))
	}
	sample, err := json.Marshal(summarizeUnkeyedRows(rows))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if len(sample) >= probeSampleMaxBytes {
		t.Fatalf("94 projected rows need %d bytes, want < %d so no page loses its evidence", len(sample), probeSampleMaxBytes)
	}
	full, err := json.Marshal(rows)
	if err != nil {
		t.Fatalf("marshal full rows: %v", err)
	}
	if len(full) <= len(sample) {
		t.Fatalf("projection must shrink the payload: full=%d projected=%d", len(full), len(sample))
	}
	t.Logf("94 rows: full=%d bytes (over the %d cap => truncated), projected=%d bytes", len(full), probeSampleMaxBytes, len(sample))
}
