package listingdaily

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestMetricsForPersistenceUseOneNormalizedKeyOrder(t *testing.T) {
	date := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	earlier := Metric{
		Key:   Key{Store: " store-a ", Channel: "SC_FBA", ASIN: "B01", SKU: "SKU-1", BusinessDate: date},
		Scope: ScopeListing,
	}
	later := Metric{
		Key:   Key{Store: "store-b", Channel: "sc_fba", ASIN: "B02", SKU: "SKU-2", BusinessDate: date},
		Scope: ScopeListing,
	}
	input := []Metric{later, earlier}
	inputBefore := append([]Metric(nil), input...)

	got := metricsForPersistence(input)
	reverseGot := metricsForPersistence([]Metric{earlier, later})

	if gotIDs := metricIDs(got); !reflect.DeepEqual(gotIDs, metricIDs(reverseGot)) {
		t.Fatalf("unordered inputs produced different key order: %v vs %v", gotIDs, metricIDs(reverseGot))
	}
	wantIDs := []string{keyID(earlier.Key, earlier.Scope), keyID(later.Key, later.Scope)}
	if gotIDs := metricIDs(got); !reflect.DeepEqual(gotIDs, wantIDs) {
		t.Fatalf("normalized key order = %v, want %v", gotIDs, wantIDs)
	}
	if !reflect.DeepEqual(input, inputBefore) {
		t.Fatalf("input slice was mutated: %#v", input)
	}
}

func TestRetryableLocalTxErrorOnlyMatchesLockErrors(t *testing.T) {
	if !retryableLocalTxError(&mysql.MySQLError{Number: 1205}) || !retryableLocalTxError(&mysql.MySQLError{Number: 1213}) {
		t.Fatal("lock wait and deadlock errors must be retried")
	}
	if retryableLocalTxError(&mysql.MySQLError{Number: 1062}) {
		t.Fatal("duplicate key must not be retried")
	}
}

func metricIDs(rows []Metric) []string {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, keyID(row.Key, row.Scope))
	}
	return ids
}

func TestSQLStorePublicPersistersUseNormalizedDimensionOrder(t *testing.T) {
	log := &dimensionOrderLog{}
	driverName := fmt.Sprintf("listingdaily_order_%d", time.Now().UnixNano())
	sql.Register(driverName, dimensionOrderDriver{log: log})
	db, err := sqlx.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	date := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	earlier := Metric{Key: Key{Store: "store-a", Channel: "sc_fba", ASIN: "B01", SKU: "SKU-1", BusinessDate: date}, Scope: ScopeListing}
	later := Metric{Key: Key{Store: "store-b", Channel: "sc_fba", ASIN: "B02", SKU: "SKU-2", BusinessDate: date}, Scope: ScopeListing}
	rows := []Metric{later, earlier}
	want := []string{"B01\x00SKU-1", "B02\x00SKU-2"}
	store := SQLStore{DB: db}

	for _, persist := range []func() error{
		func() error { return store.Persist(context.Background(), rows) },
		func() error { return store.PersistReportBatch(context.Background(), rows, nil) },
	} {
		log.ids = nil
		if err := persist(); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(log.ids, want) {
			t.Fatalf("dimension write order = %v, want %v", log.ids, want)
		}
	}
}

type dimensionOrderLog struct{ ids []string }

type dimensionOrderDriver struct{ log *dimensionOrderLog }

func (d dimensionOrderDriver) Open(string) (driver.Conn, error) {
	return &dimensionOrderConn{log: d.log}, nil
}

type dimensionOrderConn struct{ log *dimensionOrderLog }

func (c *dimensionOrderConn) Prepare(string) (driver.Stmt, error) { return nil, driver.ErrSkip }
func (c *dimensionOrderConn) Close() error                        { return nil }
func (c *dimensionOrderConn) Begin() (driver.Tx, error)           { return dimensionOrderTx{}, nil }

func (c *dimensionOrderConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return dimensionOrderTx{}, nil
}

func (c *dimensionOrderConn) ExecContext(_ context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if strings.Contains(query, "INSERT INTO listing_dimensions") {
		if len(args) < 4 {
			return nil, errors.New("listing daily test driver: missing identity key")
		}
		identityKey, ok := args[3].Value.(string)
		if !ok {
			return nil, fmt.Errorf("listing daily test driver: identity key type %T", args[3].Value)
		}
		c.log.ids = append(c.log.ids, identityKey)
	}
	return dimensionOrderResult{lastInsertID: int64(len(c.log.ids))}, nil
}

type dimensionOrderTx struct{}

func (dimensionOrderTx) Commit() error   { return nil }
func (dimensionOrderTx) Rollback() error { return nil }

type dimensionOrderResult struct{ lastInsertID int64 }

func (r dimensionOrderResult) LastInsertId() (int64, error) { return r.lastInsertID, nil }
func (r dimensionOrderResult) RowsAffected() (int64, error) { return 1, nil }
