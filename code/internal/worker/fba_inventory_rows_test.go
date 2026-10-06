package worker

import (
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
