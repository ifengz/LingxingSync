package datasetapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fixedRows struct {
	values []any
	used   bool
}

func (r *fixedRows) Next() bool {
	if r.used {
		return false
	}
	r.used = true
	return true
}

func (r *fixedRows) Scan(dest ...any) error {
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan destinations=%d values=%d", len(dest), len(r.values))
	}
	for i, value := range r.values {
		if err := assignFixedScan(dest[i], value); err != nil {
			return err
		}
	}
	return nil
}

func (*fixedRows) Err() error   { return nil }
func (*fixedRows) Close() error { return nil }

func assignFixedScan(dest any, value any) error {
	switch target := dest.(type) {
	case *sql.NullString:
		if value == nil {
			*target = sql.NullString{}
			return nil
		}
		switch value := value.(type) {
		case string:
			*target = sql.NullString{String: value, Valid: true}
		case []byte:
			*target = sql.NullString{String: string(value), Valid: true}
		default:
			return fmt.Errorf("cannot scan %T into NullString", value)
		}
	case *sql.NullTime:
		if value == nil {
			*target = sql.NullTime{}
			return nil
		}
		parsed, ok := value.(time.Time)
		if !ok {
			return fmt.Errorf("cannot scan %T into NullTime", value)
		}
		*target = sql.NullTime{Time: parsed, Valid: true}
	case *sql.NullBool:
		if value == nil {
			*target = sql.NullBool{}
			return nil
		}
		parsed, ok := value.(bool)
		if !ok {
			return fmt.Errorf("cannot scan %T into NullBool", value)
		}
		*target = sql.NullBool{Bool: parsed, Valid: true}
	case *uint64:
		parsed := reflect.ValueOf(value)
		if !parsed.IsValid() || !parsed.Type().ConvertibleTo(reflect.TypeOf(uint64(0))) {
			return fmt.Errorf("cannot scan %T into uint64", value)
		}
		*target = parsed.Convert(reflect.TypeOf(uint64(0))).Uint()
	case *any:
		*target = value
	default:
		return fmt.Errorf("unsupported scan destination %T", dest)
	}
	return nil
}

type fixedQueryer struct {
	query string
	args  []any
	rows  SQLRows
}

func (q *fixedQueryer) Query(_ context.Context, query string, args ...any) (SQLRows, error) {
	q.query = query
	q.args = append([]any(nil), args...)
	return q.rows, nil
}

func TestSQLReaderSnapshotUsesFixedSchemaAndKeysetPage(t *testing.T) {
	rows := &fixedRows{values: []any{
		"store-a", "SC", "listing", "ASIN1", "SKU1", time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 1, 1, 0, 0, 0, time.UTC), true, false, "[]", nil, uint64(7), int64(3),
	}}
	queryer := &fixedQueryer{rows: rows}
	reader := &SQLReader{queryer: queryer}
	page, err := reader.Snapshot(context.Background(), Query{Store: "store-a", DateFrom: "2026-08-01", DateTo: "2026-08-01", Fields: []string{"sales_units"}, PageSize: 1})
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if !strings.Contains(queryer.query, "FROM listing_daily_metrics m JOIN listing_dimensions d") || !strings.Contains(queryer.query, "m.sales_units AS `sales_units`") {
		t.Fatalf("reader query is outside fixed schema: %s", queryer.query)
	}
	if strings.Contains(queryer.query, "internal_secret") || len(page.Rows) != 1 || page.Rows[0].DeletedAt != nil {
		t.Fatalf("unexpected page: query=%s page=%+v", queryer.query, page)
	}
	if page.Rows[0].StableKey != "7|2026-08-01" || page.Rows[0].Values["sales_units"] != int64(3) {
		t.Fatalf("row identity/value mismatch: %+v", page.Rows[0])
	}
}

func TestSQLReaderChangesBindsUpdatedAtAndStableKey(t *testing.T) {
	updated := time.Date(2026, 8, 1, 3, 4, 5, 0, time.UTC)
	queryer := &fixedQueryer{rows: &fixedRows{values: []any{
		"store-a", "SC", "listing", "ASIN1", "SKU1", time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), updated, false, true, "[\"sales_units\"]", updated, uint64(8), int64(5),
	}}}
	reader := &SQLReader{queryer: queryer}
	page, err := reader.Changes(context.Background(), Query{Store: "store-a", Fields: []string{"sales_units"}, PageSize: 10, Cursor: &CursorKey{UpdatedAt: updated, StableKey: "7|2026-08-01"}})
	if err != nil {
		t.Fatalf("Changes: %v", err)
	}
	if len(page.Rows) != 1 || page.Rows[0].StableKey != "8|2026-08-02" || page.Rows[0].VerificationStatus != "verified" {
		t.Fatalf("unexpected changes page: %+v", page)
	}
	if !strings.Contains(queryer.query, "m.updated_at > ? OR (m.updated_at = ?") || !strings.Contains(queryer.query, "ORDER BY m.updated_at ASC, m.listing_dimension_id ASC, m.business_date ASC") {
		t.Fatalf("changes query is not keyset ordered: %s", queryer.query)
	}
	if len(queryer.args) != 7 || queryer.args[1] != updated || queryer.args[2] != updated || queryer.args[3] != uint64(7) || queryer.args[4] != uint64(7) || queryer.args[5] != "2026-08-01" {
		t.Fatalf("cursor args mismatch: %#v", queryer.args)
	}
}

func TestSQLReaderRejectsUnknownFieldBeforeQuery(t *testing.T) {
	queryer := &fixedQueryer{rows: &fixedRows{}}
	reader := &SQLReader{queryer: queryer}
	if _, err := reader.Snapshot(context.Background(), Query{Store: "store-a", DateFrom: "2026-08-01", DateTo: "2026-08-01", Fields: []string{"internal_secret"}, PageSize: 1}); err == nil {
		t.Fatal("unknown field was accepted")
	}
	if queryer.query != "" {
		t.Fatal("unknown field reached SQL query")
	}
}

func TestChangesHTTPThroughSQLReaderReturnsExplicitNilDeletedAt(t *testing.T) {
	updated := time.Date(2026, 8, 1, 3, 4, 5, 0, time.UTC)
	queryer := &fixedQueryer{rows: &fixedRows{values: []any{
		"store-a", "SC", "listing", "ASIN1", "SKU1", time.Date(2026, 8, 2, 0, 0, 0, 0, time.UTC), updated, false, false, "[]", nil, uint64(8), int64(5),
	}}}
	rawToken := "sql-reader-project-token"
	handler, err := New(Config{
		FieldAllowlist: []string{"sales_units"},
		CursorSecret:   []byte("cursor-secret-for-tests"),
		Tokens: []Token{{
			ID: "token-a", ProjectID: "project-a", Hash: HashToken(rawToken), DatasetScopes: []string{DatasetID}, StoreScopes: []string{"store-a"}, Fields: []string{"sales_units"},
		}},
	}, &SQLReader{queryer: queryer})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	cursor, err := handler.encodeCursor(cursorEnvelope{Version: 1, Dataset: DatasetID, Kind: "changes", TokenID: "token-a", Store: "store-a", Key: CursorKey{UpdatedAt: updated, StableKey: "7|2026-08-01"}})
	if err != nil {
		t.Fatalf("encode cursor: %v", err)
	}
	rec := requestJSON(t, handler, "POST", ChangesPath, rawToken, `{"store":"store-a","cursor":"`+cursor+`"}`)
	if rec.Code != 200 {
		t.Fatalf("changes status=%d body=%s", rec.Code, rec.Body.String())
	}
	var response Response
	if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Rows) != 1 || response.Data.Rows[0]["deleted_at"] != nil || response.Data.Rows[0]["sales_units"] != float64(5) {
		t.Fatalf("changes response mismatch: %+v", response.Data)
	}
	if !strings.Contains(queryer.query, "m.updated_at > ?") {
		t.Fatalf("changes did not use keyset SQL: %s", queryer.query)
	}
}
