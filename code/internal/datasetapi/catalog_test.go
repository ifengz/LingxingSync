package datasetapi

import "testing"

func TestCatalogExposesOnlyRegisteredDataProducts(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 4 {
		t.Fatalf("definitions=%d, want 4", len(definitions))
	}

	returns, ok := DefinitionFor("return-reason-detail-v1")
	if !ok {
		t.Fatal("return detail definition is missing")
	}
	if returns.Kind != DatasetKindDetail || returns.Source != "ls_sc_refunds" || returns.Grain != "store + license_plate_number" {
		t.Fatalf("return detail definition=%+v", returns)
	}

	inventory, ok := DefinitionFor("fba-inventory-snapshot-v1")
	if !ok || inventory.Kind != DatasetKindSnapshot || inventory.Source != "ls_fba_inventory" {
		t.Fatalf("inventory snapshot definition=%+v found=%t", inventory, ok)
	}
	if _, ok := DefinitionFor("user_entered_table"); ok {
		t.Fatal("unregistered dataset must not be exposed")
	}
}
