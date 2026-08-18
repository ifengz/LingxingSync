package db

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"lingxing-sync/internal/reportexport"
)

func TestIsDuplicateKeyErrorOnlyAcceptsMySQL1062(t *testing.T) {
	if !isDuplicateKeyError(&mysql.MySQLError{Number: 1062, Message: "duplicate"}) {
		t.Fatal("MySQL 1062 was not recognized as duplicate key")
	}
	if isDuplicateKeyError(&mysql.MySQLError{Number: 1048, Message: "not null"}) || isDuplicateKeyError(errors.New("duplicate text")) {
		t.Fatal("non-1062 error was recognized as duplicate key")
	}
}

func TestSaveFixedReportRowsRejectsPlaceholderMismatchBeforeDatabase(t *testing.T) {
	store := &DBReportStore{}
	err := store.saveFixedReportRows(context.Background(), 1, "test report", "INSERT INTO t VALUES (?, ?)", [][]string{{"a"}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "placeholders=2, want 7") {
		t.Fatalf("error = %v", err)
	}
}

func TestSaveFixedReportRowsIgnoresQuestionMarkInQuotedColumnName(t *testing.T) {
	store := &DBReportStore{}
	err := store.saveFixedReportRows(context.Background(), 1, "test report", "INSERT INTO t (`fee?`, value) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", [][]string{{"a", "b"}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want placeholder validation to ignore quoted column question mark", err)
	}
}

func TestSaveAllOrdersAcceptsCanonicalThirtyFourColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveAllOrders(context.Background(), 1, []reportexport.AllOrder{{Values: make([]string, 34)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBAInventoryAcceptsProductionTwentyFourColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFBAInventory(context.Background(), 1, []reportexport.FBAInventory{{AFNFCTransferQuantity: "1", AFNOnhandBuyableQuantity: "2", Store: "store", AFNFulfillableQuantityRaw: "3"}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want 24-column placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBAStrandedInventoryAcceptsProductionNineteenColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFBAStrandedInventory(context.Background(), 1, []reportexport.FBAStrandedInventory{{Values: make([]string, 19)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want 19-column placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBAAllInventoryAcceptsProductionTwentyFourColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFBAAllInventory(context.Background(), 1, []reportexport.FBAAllInventory{{AFNFCTransferQuantity: "1", AFNOnhandBuyableQuantity: "2", Store: "store", AFNFulfillableQuantityRaw: "3"}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want 24-column placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBAEstimatedFeesAcceptsCanonicalFortyColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFBAEstimatedFees(context.Background(), 1, []reportexport.FBAEstimatedFees{{Values: make([]string, 40)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want 40-column placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBAInventoryPlanningAcceptsProductionNinetyNineColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFBAInventoryPlanning(context.Background(), 1, []reportexport.FBAInventoryPlanning{{Values: make([]string, 99)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want placeholder validation to pass before nil database", err)
	}
}

func TestSaveFulfilledShipmentsAcceptsOfficialFortyEightColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFulfilledShipments(context.Background(), 1, []reportexport.FulfilledShipment{{Values: make([]string, 48)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBAStorageFeeChargesAcceptsCanonicalThirtySixColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveFBAStorageFeeCharges(context.Background(), 1, []reportexport.FBAStorageFeeCharges{{Values: make([]string, 36)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want 36-column placeholder validation to pass before nil database", err)
	}
}

func TestSaveFBARemovalOrderMapsObservedColumnsThroughPublicEntry(t *testing.T) {
	log := &removalOrderSaveLog{}
	driverName := fmt.Sprintf("removal_order_save_%d", time.Now().UnixNano())
	sql.Register(driverName, removalOrderSaveDriver{log: log})
	db, err := sqlx.Open(driverName, "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	values := []string{"request", "order", "source", "type", "status", "updated", "sku", "fnsku", "disposition", "requested", "cancelled", "disposed", "shipped", "in-process", "fee", "USD"}
	err = NewReportStore(db).SaveFBARemovalOrder(context.Background(), 1, []reportexport.FBARemovalOrder{{Values: values}}, "sha", "doc")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.query, "`order-id`, `order-source`, `order-type`, `order-status`") {
		t.Fatalf("Removal Order INSERT does not map observed columns in order: %s", log.query)
	}
	if strings.Contains(log.query, "service-speed") {
		t.Fatalf("Removal Order INSERT still writes historical service-speed: %s", log.query)
	}
	if strings.Count(log.query, "`order-source`") != 3 {
		t.Fatalf("Removal Order INSERT/upsert order-source occurrences=%d, want 3", strings.Count(log.query, "`order-source`"))
	}
	if got, want := log.args[8:10], []driver.Value{"source", "type"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("Removal Order source/type args=%v, want %v", got, want)
	}
}

type removalOrderSaveLog struct {
	query string
	args  []driver.Value
}

type removalOrderSaveDriver struct{ log *removalOrderSaveLog }

func (d removalOrderSaveDriver) Open(string) (driver.Conn, error) {
	return &removalOrderSaveConn{log: d.log}, nil
}

type removalOrderSaveConn struct{ log *removalOrderSaveLog }

func (c *removalOrderSaveConn) Prepare(query string) (driver.Stmt, error) {
	if strings.Contains(query, "INSERT INTO ls_fba_removal_order_details") {
		c.log.query = query
	}
	return removalOrderSaveStmt{query: query, log: c.log}, nil
}

func (*removalOrderSaveConn) Close() error              { return nil }
func (*removalOrderSaveConn) Begin() (driver.Tx, error) { return removalOrderSaveTx{}, nil }

func (*removalOrderSaveConn) BeginTx(context.Context, driver.TxOptions) (driver.Tx, error) {
	return removalOrderSaveTx{}, nil
}

func (*removalOrderSaveConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &removalOrderSaveRows{values: []driver.Value{"acct", "seller", "store", "task"}}, nil
}

func (*removalOrderSaveConn) ExecContext(context.Context, string, []driver.NamedValue) (driver.Result, error) {
	return removalOrderSaveResult{}, nil
}

type removalOrderSaveStmt struct {
	query string
	log   *removalOrderSaveLog
}

func (removalOrderSaveStmt) Close() error  { return nil }
func (removalOrderSaveStmt) NumInput() int { return -1 }
func (s removalOrderSaveStmt) Exec(args []driver.Value) (driver.Result, error) {
	if strings.Contains(s.query, "INSERT INTO ls_fba_removal_order_details") {
		s.log.args = append([]driver.Value(nil), args...)
	}
	return removalOrderSaveResult{}, nil
}
func (removalOrderSaveStmt) Query([]driver.Value) (driver.Rows, error) {
	return nil, driver.ErrSkip
}

type removalOrderSaveRows struct {
	values []driver.Value
	done   bool
}

func (*removalOrderSaveRows) Columns() []string {
	return []string{"account_id", "seller_id", "store_id", "report_task_id"}
}
func (*removalOrderSaveRows) Close() error { return nil }
func (r *removalOrderSaveRows) Next(dest []driver.Value) error {
	if r.done {
		return io.EOF
	}
	copy(dest, r.values)
	r.done = true
	return nil
}

type removalOrderSaveTx struct{}

func (removalOrderSaveTx) Commit() error   { return nil }
func (removalOrderSaveTx) Rollback() error { return nil }

type removalOrderSaveResult struct{}

func (removalOrderSaveResult) LastInsertId() (int64, error) { return 1, nil }
func (removalOrderSaveResult) RowsAffected() (int64, error) { return 1, nil }

func TestMarkReportProgressRejectsUnknownStatusBeforeDatabase(t *testing.T) {
	store := &DBReportStore{}
	if err := store.MarkReportProgress(context.Background(), 1, "new_status", "", "", ""); err == nil || !strings.Contains(err.Error(), "invalid progress status") {
		t.Fatalf("error = %v, want invalid progress status", err)
	}
}

func TestMarkReportCreatedRejectsBlankTaskID(t *testing.T) {
	store := &DBReportStore{db: &sqlx.DB{}}
	if err := store.MarkReportCreated(context.Background(), 1, " "); err == nil || !strings.Contains(err.Error(), "task id is required") {
		t.Fatalf("error = %v, want task id validation", err)
	}
}

func TestEnsureReportDeduplicatesConcurrentActiveScope(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the active report dedupe integration test")
	}
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("connect migration test database: %v", err)
	}
	defer db.Close()
	if err := RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	store := NewReportStore(db)
	dateSeed := time.Now().UTC().Format("20060102150405.000000000")
	req := reportexport.Request{
		AccountID: "dedupe-acct", SellerID: "dedupe-seller", StoreID: "dedupe-store", Region: "na",
		MarketplaceIDs: []string{"B", "A"}, DateFrom: dateSeed, DateTo: dateSeed,
	}
	const callers = 8
	ids := make(chan int64, callers)
	errs := make(chan error, callers)
	var wg sync.WaitGroup
	for i := 0; i < callers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			audit, callErr := store.EnsureReport(context.Background(), req)
			if callErr != nil {
				errs <- callErr
				return
			}
			ids <- audit.ID
		}()
	}
	wg.Wait()
	close(ids)
	close(errs)
	for err := range errs {
		t.Fatalf("EnsureReport returned error: %v", err)
	}
	var first int64
	count := 0
	for id := range ids {
		if count == 0 {
			first = id
		} else if id != first {
			t.Fatalf("active scope created duplicate audit ids: first=%d current=%d", first, id)
		}
		count++
	}
	if count != callers {
		t.Fatalf("got %d audit results, want %d", count, callers)
	}

	if err := store.MarkReportProgress(context.Background(), first, "UNKNOWN", "", "", ""); err != nil {
		t.Fatalf("mark active task UNKNOWN: %v", err)
	}
	var unknownActiveKey *string
	if err := db.Get(&unknownActiveKey, `SELECT active_scope_key FROM ls_report_export_tasks WHERE id = ?`, first); err != nil {
		t.Fatalf("load UNKNOWN active key: %v", err)
	}
	if unknownActiveKey == nil {
		t.Fatal("transient UNKNOWN task released its active scope key")
	}
	reusedUnknown, err := store.EnsureReport(context.Background(), req)
	if err != nil {
		t.Fatalf("EnsureReport during UNKNOWN: %v", err)
	}
	if reusedUnknown.ID != first {
		t.Fatalf("UNKNOWN task was not reused: got id=%d want id=%d", reusedUnknown.ID, first)
	}

	if err := store.MarkReportError(context.Background(), first, "ERROR", fmt.Errorf("test terminal state")); err != nil {
		t.Fatalf("mark active task terminal: %v", err)
	}
	next, err := store.EnsureReport(context.Background(), req)
	if err != nil {
		t.Fatalf("EnsureReport after terminal state: %v", err)
	}
	if next.ID == first {
		t.Fatalf("terminal ERROR task was reused: id=%d", next.ID)
	}
	if err := store.MarkReportProgress(context.Background(), next.ID, "FATAL", "", "", ""); err != nil {
		t.Fatalf("mark upstream terminal state: %v", err)
	}
	var activeKey *string
	if err := db.Get(&activeKey, `SELECT active_scope_key FROM ls_report_export_tasks WHERE id = ?`, next.ID); err != nil {
		t.Fatalf("load terminal active key: %v", err)
	}
	if activeKey != nil {
		t.Fatalf("terminal FATAL active key = %q, want NULL", *activeKey)
	}
	afterFatal, err := store.EnsureReport(context.Background(), req)
	if err != nil {
		t.Fatalf("EnsureReport after FATAL: %v", err)
	}
	if afterFatal.ID == next.ID {
		t.Fatalf("terminal FATAL task was reused: id=%d", afterFatal.ID)
	}
}

func TestEnsureReportReusesLegacyActiveRowWithoutDigest(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the legacy active-row integration test")
	}
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("connect migration test database: %v", err)
	}
	defer db.Close()
	if err := RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	seed := time.Now().UTC().Format("20060102150405.000000000")
	request := reportexport.Request{
		AccountID: "legacy-acct", SellerID: "legacy-seller", StoreID: "legacy-store", Region: "na",
		MarketplaceIDs: []string{"B", "A"}, DateFrom: seed, DateTo: seed,
	}
	result, err := db.Exec(`INSERT INTO ls_report_export_tasks
(account_id, seller_id, store_id, report_type, region, marketplace_ids, date_from, date_to, status, active_scope_key)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'PENDING', NULL)`, request.AccountID, request.SellerID, request.StoreID, reportexport.CustomerReturnsReportType, request.Region, `["A","B"]`, request.DateFrom, request.DateTo)
	if err != nil {
		t.Fatalf("insert legacy active row: %v", err)
	}
	legacyID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("legacy row id: %v", err)
	}
	audit, err := NewReportStore(db).EnsureReport(context.Background(), request)
	if err != nil {
		t.Fatalf("EnsureReport legacy row: %v", err)
	}
	if audit.ID != legacyID || !audit.CreateClaimed || audit.Status != "CREATING" {
		t.Fatalf("legacy audit = %#v, want claimed CREATING id=%d", audit, legacyID)
	}
}
