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
