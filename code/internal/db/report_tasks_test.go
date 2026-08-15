package db

import (
	"context"
	"errors"
	"fmt"
	"os"
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

func TestSaveAllOrdersAcceptsCanonicalThirtyThreeColumnRow(t *testing.T) {
	store := &DBReportStore{}
	err := store.SaveAllOrders(context.Background(), 1, []reportexport.AllOrder{{Values: make([]string, 33)}}, "sha", "doc")
	if err == nil || !strings.Contains(err.Error(), "nil database") {
		t.Fatalf("error = %v, want placeholder validation to pass before nil database", err)
	}
}

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
