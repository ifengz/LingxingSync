package datasetapi

import "testing"

func TestCatalogExposesOnlyRegisteredDataProducts(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 5 {
		t.Fatalf("definitions=%d, want 5", len(definitions))
	}

	returns, ok := DefinitionFor("return-reason-detail-v1")
	if !ok {
		t.Fatal("return detail definition is missing")
	}
	if returns.Kind != DatasetKindDetail || returns.Source != "ls_sc_refunds" || returns.Grain != "store + license_plate_number" {
		t.Fatalf("return detail definition=%+v", returns)
	}

	inventory, ok := DefinitionFor("fba-inventory-snapshot-v1")
	if !ok || inventory.Kind != DatasetKindSnapshot || inventory.Source != "fba_inventory_daily_snapshots" {
		t.Fatalf("inventory snapshot definition=%+v found=%t", inventory, ok)
	}
	if inventory.Grain != "store + fnsku + snapshot_date" || inventory.InitialCursor != "0|0|0|1000-01-01" {
		t.Fatalf("inventory history contract mismatch: %+v", inventory)
	}
	address, ok := DefinitionFor("address-order-item-detail-v1")
	if !ok || address.Kind != DatasetKindDetail || address.Grain != "store + shipment_id + shipment_item_id" {
		t.Fatalf("address order item definition=%+v found=%t", address, ok)
	}
	for _, field := range []string{"store_name", "last_updated_date", "order_status", "asin", "quantity", "source_updated_at"} {
		found := false
		for _, actual := range address.Fields {
			found = found || actual == field
		}
		if !found {
			t.Fatalf("address order item field %q is missing", field)
		}
	}
	if _, ok := DefinitionFor("user_entered_table"); ok {
		t.Fatal("unregistered dataset must not be exposed")
	}
}
