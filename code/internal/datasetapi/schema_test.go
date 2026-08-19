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
