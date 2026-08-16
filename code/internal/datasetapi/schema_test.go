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
