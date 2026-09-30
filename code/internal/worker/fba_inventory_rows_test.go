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

	filtered, dropped, err := filterFBAInventoryRows("ls_fba_inventory", rows)
	if err != nil {
		t.Fatalf("filterFBAInventoryRows returned error: %v", err)
	}
	if dropped != 2 {
		t.Fatalf("dropped=%d, want 2", dropped)
	}
	if len(filtered) != 1 || filtered[0]["fnsku"] != "X-FBA" {
		t.Fatalf("filtered=%v, want only FBA row", filtered)
	}
}

func TestFilterFBAInventoryRowsRejectsUnknownChannel(t *testing.T) {
	rows := []map[string]any{{"fnsku": "X-1"}}
	_, _, err := filterFBAInventoryRows("ls_fba_inventory", rows)
	if err == nil || !strings.Contains(err.Error(), "fulfillment channel") {
		t.Fatalf("error=%v, want unknown fulfillment channel error", err)
	}
}

func TestFilterFBAInventoryRowsDoesNotAffectOtherTables(t *testing.T) {
	rows := []map[string]any{{"fnsku": "X-1"}}
	filtered, dropped, err := filterFBAInventoryRows("other_table", rows)
	if err != nil {
		t.Fatalf("filterFBAInventoryRows returned error: %v", err)
	}
	if dropped != 0 || len(filtered) != 1 || filtered[0]["fnsku"] != "X-1" {
		t.Fatalf("filtered=%v dropped=%d, want unchanged", filtered, dropped)
	}
}
