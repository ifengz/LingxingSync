package datasetapi

import "testing"

func TestCatalogExposesOnlyRegisteredDataProducts(t *testing.T) {
	definitions := Definitions()
	if len(definitions) != 16 {
		t.Fatalf("definitions=%d, want 16", len(definitions))
	}

	po, ok := DefinitionFor("vc-po-detail-v1")
	if !ok || po.Kind != DatasetKindDetail || po.Source != "ls_vc_orders + ls_vc_po_details" || po.Grain != "store + local_po_number" {
		t.Fatalf("VC PO definition=%+v found=%t", po, ok)
	}
	for _, field := range []string{"vc_store_id", "local_po_number", "purchase_order_number", "purchase_order_state", "items", "seller_name", "purchase_order_type"} {
		if !containsField(po.Fields, field) {
			t.Fatalf("VC PO field %q is missing", field)
		}
	}
	poLines, ok := DefinitionFor("vc-po-lines-v1")
	if !ok || poLines.Kind != DatasetKindDetail || poLines.Source != "ls_vc_po_details.items + ls_vc_orders" || poLines.Grain != "store + local_po_number + asin + msku" {
		t.Fatalf("VC PO lines definition=%+v found=%t", poLines, ok)
	}
	for _, field := range []string{"vc_store_id", "local_po_number", "purchase_order_number", "asin", "msku", "sku", "item_name", "ordered_quantity", "received_quantity", "unit_price", "image_url"} {
		if !containsField(poLines.Fields, field) {
			t.Fatalf("VC PO lines field %q is missing", field)
		}
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
	for _, field := range []string{"store_name", "marketplace", "last_updated_date", "order_status", "asin", "quantity", "fulfillment_channel", "ship_country", "ship_state", "ship_city", "ship_postal_code", "source_updated_at"} {
		if !containsField(address.Fields, field) {
			t.Fatalf("address order item field %q is missing", field)
		}
	}
	fbm, ok := DefinitionFor("fbm-address-order-item-detail-v1")
	if !ok || fbm.Source != "ls_mp_fbm_orders + item_info + address_info + platform_info" || fbm.Grain != "store_id + global_item_no" {
		t.Fatalf("FBM address definition=%+v found=%t", fbm, ok)
	}
	for _, field := range []string{"marketplace", "fulfillment_channel", "ship_country", "ship_state", "ship_city", "ship_postal_code", "ship_lat", "ship_lng", "source_global_item_no"} {
		if !containsField(fbm.Fields, field) {
			t.Fatalf("FBM address field %q is missing", field)
		}
	}
	if _, ok := DefinitionFor("user_entered_table"); ok {
		t.Fatal("unregistered dataset must not be exposed")
	}
}

func TestPageDatasetDefinitionsExposeSeparateContracts(t *testing.T) {
	cases := []struct {
		id, grain, source, field string
	}{
		{"fba-links-v1", "store + ASIN + listing_sku", "ls_sc_listing + listing_daily_metrics", "quantity_30d"},
		{"vc-links-v1", "store + ASIN", "ls_vc_listing + ls_vc_sales_report + ls_vc_inventory + ls_vc_traffic + ls_vc_margin + ls_vc_realtime_sales + ls_ad_sp_product + ls_ad_sd_product", "sales_revenue_30d"},
		{"operations-log-v1", "store + channel + ASIN + listing_sku + business_date", "listing_daily_metrics", "sales_units"},
	}
	for _, tc := range cases {
		definition, ok := DefinitionFor(tc.id)
		if !ok || definition.Grain != tc.grain || definition.Source != tc.source || !containsField(definition.Fields, tc.field) {
			t.Fatalf("page definition %s=%+v found=%t", tc.id, definition, ok)
		}
	}
}

func TestOperationsLogV2PublishesVerificationAndIdentityContract(t *testing.T) {
	v1, ok := DefinitionFor("operations-log-v1")
	if !ok || v1.NextVersionID != "operations-log-v2" {
		t.Fatalf("operations log v1 version metadata=%+v found=%t", v1, ok)
	}
	for _, v2OnlyField := range []string{"identity_scope", "verified_fields"} {
		if containsField(v1.Fields, v2OnlyField) {
			t.Fatalf("locked operations log v1 unexpectedly contains %q", v2OnlyField)
		}
	}
	v2, ok := DefinitionFor("operations-log-v2")
	if !ok {
		t.Fatal("operations log v2 definition is missing")
	}
	if v2.ParentID != "operations-log-v1" || v2.Grain != "store + channel + identity_scope + optional ASIN + optional listing_sku + business_date" {
		t.Fatalf("operations log v2 contract=%+v", v2)
	}
	for _, field := range []string{"identity_scope", "verified_fields", "sp_impressions", "sp_clicks", "sd_impressions", "sd_clicks", "hsa_impressions", "hsa_clicks", "sb_impressions", "sb_clicks"} {
		if !containsField(v2.Fields, field) {
			t.Fatalf("operations log v2 field %q is missing", field)
		}
	}
	v3, ok := DefinitionFor("operations-log-v3")
	if !ok || v2.NextVersionID != "operations-log-v3" || v3.ParentID != "operations-log-v2" {
		t.Fatalf("operations log v3 contract v2=%+v v3=%+v", v2, v3)
	}
}

func TestVersionedDatasetDefinitionsExposeFixedDraftContracts(t *testing.T) {
	cases := []struct {
		id, parent, next string
		candidate        string
		candidateCount   int
	}{
		{id: "return-reason-detail-v2", parent: "return-reason-detail-v1", candidate: "return_date_locale", candidateCount: 3},
		{id: "fba-inventory-snapshot-v2", parent: "fba-inventory-snapshot-v1", candidate: "total_fulfillable_quantity", candidateCount: 24},
		{id: "address-order-item-detail-v2", parent: "address-order-item-detail-v1", candidate: "tracking_number", candidateCount: 17},
	}
	for _, tc := range cases {
		definition, ok := DefinitionFor(tc.id)
		if !ok || definition.ParentID != tc.parent || definition.NextVersionID != "" || !containsField(definition.CatalogFields, tc.candidate) {
			t.Fatalf("draft definition %s=%+v found=%t", tc.id, definition, ok)
		}
		if !containsField(definition.Fields, tc.candidate) {
			if len(definition.Fields) >= len(definition.CatalogFields) {
				t.Fatalf("draft %s must expose candidate catalog beyond inherited fields: %+v", tc.id, definition)
			}
		}
		if got := len(definition.CatalogFields) - len(definition.Fields); got != tc.candidateCount {
			t.Fatalf("draft %s candidate count=%d want %d", tc.id, got, tc.candidateCount)
		}
	}
	v1, _ := DefinitionFor("return-reason-detail-v1")
	if v1.NextVersionID != "return-reason-detail-v2" || len(v1.CatalogFields) <= len(v1.Fields) {
		t.Fatalf("v1 version metadata/candidate catalog=%+v", v1)
	}
}

func containsField(fields []string, wanted string) bool {
	for _, field := range fields {
		if field == wanted {
			return true
		}
	}
	return false
}
