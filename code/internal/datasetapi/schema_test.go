package datasetapi

import (
	"strings"
	"testing"
)

func TestRegisteredDatasetSchemasCoverDefinitions(t *testing.T) {
	for _, definition := range Definitions() {
		schema, ok := SchemaFor(definition.ID)
		if !ok {
			t.Fatalf("dataset %s has no downstream schema", definition.ID)
		}
		if schema.DatasetID != definition.ID || schema.TableName == "" || len(schema.Columns) == 0 || len(schema.PrimaryKey) == 0 {
			t.Fatalf("dataset %s schema is incomplete: %+v", definition.ID, schema)
		}
		columnNames := make(map[string]struct{}, len(schema.Columns))
		for _, column := range schema.Columns {
			if column.Name == "" || column.SQLType == "" {
				t.Fatalf("dataset %s has incomplete column: %+v", definition.ID, column)
			}
			columnNames[column.Name] = struct{}{}
		}
		for _, key := range schema.PrimaryKey {
			if _, ok := columnNames[key]; !ok {
				t.Fatalf("dataset %s primary key column %q is missing", definition.ID, key)
			}
		}
		ddl, err := schema.CreateTableSQL(nil)
		if err != nil || !strings.Contains(ddl, "CREATE TABLE") || !strings.Contains(ddl, "PRIMARY KEY") {
			t.Fatalf("dataset %s invalid create SQL: err=%v sql=%s", definition.ID, err, ddl)
		}
	}
}

func TestVCPOSchemaUsesStableHeaderKeyAndRawItems(t *testing.T) {
	schema, ok := SchemaFor("vc-po-detail-v1")
	if !ok {
		t.Fatal("VC PO schema missing")
	}
	if strings.Join(schema.PrimaryKey, ",") != "store,stable_key" {
		t.Fatalf("VC PO primary key=%v", schema.PrimaryKey)
	}
	ddl, err := schema.CreateTableSQL(nil)
	if err != nil {
		t.Fatalf("create SQL: %v", err)
	}
	for _, want := range []string{"`items` JSON", "`local_po_number` VARCHAR(64)", "PRIMARY KEY (`store`, `stable_key`)"} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("VC PO DDL missing %q: %s", want, ddl)
		}
	}
}

func TestVCPOLinesSchemaUsesCompositeLineKey(t *testing.T) {
	schema, ok := SchemaFor("vc-po-lines-v1")
	if !ok {
		t.Fatal("VC PO lines schema missing")
	}
	if strings.Join(schema.PrimaryKey, ",") != "store,stable_key" {
		t.Fatalf("VC PO lines primary key=%v", schema.PrimaryKey)
	}
	ddl, err := schema.CreateTableSQL(nil)
	if err != nil {
		t.Fatalf("create SQL: %v", err)
	}
	for _, want := range []string{"`asin` VARCHAR(255)", "`msku` VARCHAR(255)", "`ordered_quantity` BIGINT", "`received_quantity` BIGINT", "PRIMARY KEY (`store`, `stable_key`)"} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("VC PO lines DDL missing %q: %s", want, ddl)
		}
	}
}

func TestPageDatasetSchemasHaveStableKeys(t *testing.T) {
	for _, id := range []string{"fba-links-v1", "vc-links-v1", "operations-log-v1"} {
		schema, ok := SchemaFor(id)
		if !ok {
			t.Fatalf("schema %s missing", id)
		}
		if strings.Join(schema.PrimaryKey, ",") != "store,stable_key" {
			t.Fatalf("schema %s primary key=%v", id, schema.PrimaryKey)
		}
	}
}

func TestVCFactSchemasUseTheirOwnPublishedFields(t *testing.T) {
	cases := map[string]map[string]string{
		"vc-inventory-daily-v1": {"sellable": "BIGINT", "sellable_cost": "DECIMAL(20,6)", "currency": "VARCHAR(16)"},
		"vc-ad-daily-v1":        {"profile_id": "VARCHAR(255)", "campaign_type": "VARCHAR(255)", "impressions": "BIGINT"},
	}
	for datasetID, expected := range cases {
		schema, ok := SchemaFor(datasetID)
		if !ok {
			t.Fatalf("schema %s missing", datasetID)
		}
		columns := make(map[string]struct{}, len(schema.Columns))
		for _, column := range schema.Columns {
			columns[column.Name] = struct{}{}
		}
		for field, sqlType := range expected {
			_, ok := columns[field]
			if !ok {
				t.Fatalf("schema %s missing published field %s", datasetID, field)
			}
			for _, candidate := range schema.Columns {
				if candidate.Name == field && candidate.SQLType != sqlType {
					t.Fatalf("schema %s field %s type=%q want %q", datasetID, field, candidate.SQLType, sqlType)
				}
			}
		}
		if _, ok := columns["sales_units"]; ok {
			t.Fatalf("schema %s leaked unrelated daily field sales_units", datasetID)
		}
	}
}

func TestRequestedDownstreamSchemasPublishOwnFields(t *testing.T) {
	cases := map[string][]string{
		"vc-traffic-daily-v1":            {"glance_views", "business_date"},
		"sc-account-ad-daily-v1":         {"campaign_type", "total_spend", "total_sales", "total_orders", "currency"},
		"vc-realtime-v1":                 {"start_time", "end_time", "ordered_units", "ordered_revenue"},
		"vc-listing-metrics-snapshot-v1": {"snapshot_date", "classification_rank", "display_group_rank", "reviews_num", "stars"},
	}
	for id, fields := range cases {
		schema, ok := SchemaFor(id)
		if !ok {
			t.Fatalf("schema %s missing", id)
		}
		columns := make(map[string]bool, len(schema.Columns))
		for _, column := range schema.Columns {
			columns[column.Name] = true
		}
		for _, field := range fields {
			if !columns[field] {
				t.Fatalf("schema %s missing field %s", id, field)
			}
		}
	}
}

func TestSCAccountAdDailyV2HasIndependentSchema(t *testing.T) {
	schema, ok := SchemaFor("sc-account-ad-daily-v2")
	if !ok || schema.TableName != "sc_account_ad_daily_v2" {
		t.Fatalf("v2 schema=%+v found=%t", schema, ok)
	}
	columns := make(map[string]bool, len(schema.Columns))
	for _, column := range schema.Columns {
		columns[column.Name] = true
	}
	for _, field := range []string{"campaign_type", "business_date", "total_spend", "total_sales", "total_orders", "currency"} {
		if !columns[field] {
			t.Fatalf("v2 schema missing field %s", field)
		}
	}
}

func TestVCLinksSchemaKeepsAdSparklineAsJSON(t *testing.T) {
	schema, ok := SchemaFor("vc-links-v1")
	if !ok {
		t.Fatal("VC Links schema missing")
	}
	for _, column := range schema.Columns {
		if column.Name == "ad_spend_sparkline_7d" && column.SQLType != "JSON" {
			t.Fatalf("ad sparkline type=%q want JSON", column.SQLType)
		}
	}
}

func TestOperationsLogV2SchemaKeepsVerificationAsJSON(t *testing.T) {
	schema, ok := SchemaFor("operations-log-v2")
	if !ok {
		t.Fatal("operations log v2 schema missing")
	}
	wantTypes := map[string]string{
		"identity_scope": "VARCHAR(16)", "verified_fields": "JSON",
		"sp_impressions": "BIGINT", "sp_clicks": "BIGINT",
		"sd_impressions": "BIGINT", "sd_clicks": "BIGINT",
		"hsa_impressions": "BIGINT", "hsa_clicks": "BIGINT",
		"sb_impressions": "BIGINT", "sb_clicks": "BIGINT",
	}
	for _, column := range schema.Columns {
		if want, exists := wantTypes[column.Name]; exists {
			if column.SQLType != want {
				t.Fatalf("operations log v2 field %s type=%q want %q", column.Name, column.SQLType, want)
			}
			delete(wantTypes, column.Name)
		}
	}
	if len(wantTypes) != 0 {
		t.Fatalf("operations log v2 schema missing columns: %v", wantTypes)
	}
}

func TestOperationsLogV3SchemaUsesV2Contract(t *testing.T) {
	schema, ok := SchemaFor("operations-log-v3")
	if !ok || schema.TableName != "operations_log_v3" {
		t.Fatalf("operations log v3 schema=%+v ok=%v", schema, ok)
	}
}

func TestDatasetSchemaCreateTableCanRestrictPublishedFields(t *testing.T) {
	schema, ok := SchemaFor(DatasetID)
	if !ok {
		t.Fatal("listing schema missing")
	}
	ddl, err := schema.CreateTableSQL([]string{"sales_units"})
	if err != nil {
		t.Fatalf("create restricted SQL: %v", err)
	}
	if !strings.Contains(ddl, "`sales_units`") || strings.Contains(ddl, "`sp_sales`") {
		t.Fatalf("restricted SQL columns are wrong: %s", ddl)
	}
}

func TestAddressOrderItemSchemaKeepsCompositeDownstreamKey(t *testing.T) {
	schema, ok := SchemaFor("address-order-item-detail-v1")
	if !ok {
		t.Fatal("address order item schema missing")
	}
	ddl, err := schema.CreateTableSQL(nil)
	if err != nil {
		t.Fatalf("create SQL: %v", err)
	}
	for _, want := range []string{"`marketplace` VARCHAR(8)", "`fulfillment_channel` VARCHAR(8)", "`ship_country` CHAR(2)", "`ship_state` VARCHAR(128)", "`ship_city` VARCHAR(128)", "`ship_postal_code` VARCHAR(32)", "`ship_lat` DECIMAL(10,7)", "`ship_lng` DECIMAL(10,7)", "`source_updated_at` DATETIME(6)", "PRIMARY KEY (`store`, `stable_key`)", "KEY `idx_updated_at` (`updated_at`)"} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("address order item DDL missing %q: %s", want, ddl)
		}
	}
}

func TestFBMAddressOrderItemSchemaKeepsUpstreamItemKey(t *testing.T) {
	schema, ok := SchemaFor("fbm-address-order-item-detail-v1")
	if !ok {
		t.Fatal("FBM address order item schema missing")
	}
	ddl, err := schema.CreateTableSQL(nil)
	if err != nil {
		t.Fatalf("create SQL: %v", err)
	}
	for _, want := range []string{"`source_global_item_no` VARCHAR(128)", "`ship_country` CHAR(2)", "`ship_postal_code` VARCHAR(32)", "`ship_lat` DECIMAL(10,7)", "PRIMARY KEY (`store`, `stable_key`)", "KEY `idx_updated_at` (`updated_at`)"} {
		if !strings.Contains(ddl, want) {
			t.Fatalf("FBM address DDL missing %q: %s", want, ddl)
		}
	}
}

func TestVersionedSchemasContainCandidateColumnsWithFixedTypes(t *testing.T) {
	cases := []struct {
		datasetID, field, sqlType string
	}{
		{"return-reason-detail-v2", "return_date_locale", "VARCHAR(32)"},
		{"fba-inventory-snapshot-v2", "total_fulfillable_quantity", "INT"},
		{"fba-inventory-snapshot-v2", "cost", "DECIMAL(14,4)"},
		{"address-order-item-detail-v2", "tracking_number", "VARCHAR(128)"},
	}
	for _, tc := range cases {
		schema, ok := SchemaFor(tc.datasetID)
		if !ok {
			t.Fatalf("schema %s missing", tc.datasetID)
		}
		found := false
		for _, column := range schema.Columns {
			if column.Name == tc.field {
				found = true
				if column.SQLType != tc.sqlType {
					t.Fatalf("schema %s field %s type=%q want %q", tc.datasetID, tc.field, column.SQLType, tc.sqlType)
				}
			}
		}
		if !found {
			t.Fatalf("schema %s missing candidate %s", tc.datasetID, tc.field)
		}
	}
}

func TestV1SchemasDoNotSilentlyIncludeV2CandidateColumns(t *testing.T) {
	for _, tc := range []struct{ datasetID, candidate string }{
		{"return-reason-detail-v1", "return_date_locale"},
		{"fba-inventory-snapshot-v1", "total_fulfillable_quantity"},
		{"address-order-item-detail-v1", "tracking_number"},
	} {
		schema, _ := SchemaFor(tc.datasetID)
		for _, column := range schema.Columns {
			if column.Name == tc.candidate {
				t.Fatalf("locked %s unexpectedly contains v2 candidate %s", tc.datasetID, tc.candidate)
			}
		}
	}
}

func TestVCFactSchemasUseNumericAndBoundedTypes(t *testing.T) {
	cases := []struct {
		datasetID string
		field     string
		sqlType   string
	}{
		{"vc-inventory-daily-v1", "sellable", "BIGINT"},
		{"vc-inventory-daily-v1", "sell_through_rate", "DECIMAL(20,6)"},
		{"vc-inventory-daily-v1", "currency", "VARCHAR(16)"},
		{"vc-ad-daily-v1", "ad_orders", "BIGINT"},
		{"vc-ad-daily-v1", "impressions", "BIGINT"},
		{"vc-ad-daily-v1", "currency", "VARCHAR(16)"},
	}
	for _, tc := range cases {
		schema, ok := SchemaFor(tc.datasetID)
		if !ok {
			t.Fatalf("schema %s missing", tc.datasetID)
		}
		found := false
		for _, column := range schema.Columns {
			if column.Name == tc.field {
				found = true
				if column.SQLType != tc.sqlType {
					t.Fatalf("schema %s field %s type=%q want %q", tc.datasetID, tc.field, column.SQLType, tc.sqlType)
				}
			}
		}
		if !found {
			t.Fatalf("schema %s missing field %s", tc.datasetID, tc.field)
		}
	}
}
