package db

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestFBAInventorySnapshotSQLRebuildsOnlyTheRequestedDay(t *testing.T) {
	for _, want := range []string{"account_id = ?", "sid = ?", "snapshot_date = ?"} {
		if !strings.Contains(fbaInventorySnapshotDeleteSQL, want) {
			t.Fatalf("snapshot delete SQL missing %q: %s", want, fbaInventorySnapshotDeleteSQL)
		}
	}
	for _, want := range []string{
		"INSERT INTO fba_inventory_daily_snapshots",
		"FROM ls_fba_inventory i",
		"i.synced_at >= ?",
		"i.synced_at",
	} {
		if !strings.Contains(fbaInventorySnapshotInsertSQL, want) {
			t.Fatalf("snapshot insert SQL missing %q: %s", want, fbaInventorySnapshotInsertSQL)
		}
	}
}

func TestCaptureFBAInventorySnapshotsIsDatedAndIdempotent(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the FBA snapshot integration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("connect migration test database: %v", err)
	}
	defer dbx.Close()
	if err := RunMigrations(dbx, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	const accountID = "__snapshot_test__"
	const storeID = "__store_test__"
	cleanup := func() {
		_, _ = dbx.Exec("DELETE FROM fba_inventory_daily_snapshots WHERE account_id = ?", accountID)
		_, _ = dbx.Exec("DELETE FROM ls_fba_inventory WHERE account_id = ?", accountID)
	}
	cleanup()
	defer cleanup()
	dayOne := time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)
	if _, err := dbx.Exec(`INSERT INTO ls_fba_inventory (account_id, sid, fnsku, afn_fulfillable_quantity, synced_at) VALUES
(?, ?, ?, ?, ?), (?, ?, ?, ?, ?)`, accountID, storeID, "FNSKU-1", 3, dayOne, accountID, storeID, "FNSKU-2", 5, dayOne); err != nil {
		t.Fatalf("seed raw inventory: %v", err)
	}
	if err := CaptureFBAInventorySnapshots(context.Background(), dbx, accountID, []FBAInventorySnapshotTarget{{Store: storeID, Date: dayOne, StartedAt: dayOne}}); err != nil {
		t.Fatalf("capture first day: %v", err)
	}
	if _, err := dbx.Exec("UPDATE ls_fba_inventory SET afn_fulfillable_quantity = ?, synced_at = ? WHERE account_id = ? AND sid = ? AND fnsku = ?", 7, dayOne, accountID, storeID, "FNSKU-1"); err != nil {
		t.Fatalf("update same-day raw row: %v", err)
	}
	if err := CaptureFBAInventorySnapshots(context.Background(), dbx, accountID, []FBAInventorySnapshotTarget{{Store: storeID, Date: dayOne, StartedAt: dayOne}}); err != nil {
		t.Fatalf("recapture first day: %v", err)
	}
	dayTwo := dayOne.AddDate(0, 0, 1)
	if _, err := dbx.Exec("UPDATE ls_fba_inventory SET synced_at = ? WHERE account_id = ? AND sid = ? AND fnsku = ?", dayTwo, accountID, storeID, "FNSKU-1"); err != nil {
		t.Fatalf("touch next-day raw row: %v", err)
	}
	if err := CaptureFBAInventorySnapshots(context.Background(), dbx, accountID, []FBAInventorySnapshotTarget{{Store: storeID, Date: dayTwo, StartedAt: dayTwo}}); err != nil {
		t.Fatalf("capture second day: %v", err)
	}
	var rows int
	if err := dbx.Get(&rows, "SELECT COUNT(*) FROM fba_inventory_daily_snapshots WHERE account_id = ? AND sid = ?", accountID, storeID); err != nil {
		t.Fatalf("count history: %v", err)
	}
	if rows != 3 {
		t.Fatalf("history rows=%d, want two first-day rows plus one second-day row", rows)
	}
	var quantity int
	if err := dbx.Get(&quantity, "SELECT afn_fulfillable_quantity FROM fba_inventory_daily_snapshots WHERE account_id = ? AND sid = ? AND fnsku = ? AND snapshot_date = ?", accountID, storeID, "FNSKU-1", dayOne.Format("2006-01-02")); err != nil {
		t.Fatalf("read same-day replacement: %v", err)
	}
	if quantity != 7 {
		t.Fatalf("same-day quantity=%d, want 7", quantity)
	}
	boundaryStart := time.Date(2026, 8, 17, 23, 59, 59, 0, time.UTC)
	afterMidnight := boundaryStart.Add(2 * time.Second)
	if _, err := dbx.Exec("INSERT INTO ls_fba_inventory (account_id, sid, fnsku, synced_at) VALUES (?, ?, ?, ?)", accountID, "boundary-store", "FNSKU-3", afterMidnight); err != nil {
		t.Fatalf("seed date-boundary raw row: %v", err)
	}
	if err := CaptureFBAInventorySnapshots(context.Background(), dbx, accountID, []FBAInventorySnapshotTarget{{Store: "boundary-store", Date: boundaryStart, StartedAt: boundaryStart}}); err != nil {
		t.Fatalf("capture date-boundary task: %v", err)
	}
	if err := dbx.Get(&rows, "SELECT COUNT(*) FROM fba_inventory_daily_snapshots WHERE account_id = ? AND sid = ? AND snapshot_date = ?", accountID, "boundary-store", boundaryStart.Format("2006-01-02")); err != nil {
		t.Fatalf("count date-boundary history: %v", err)
	}
	if rows != 1 {
		t.Fatalf("date-boundary history rows=%d, want row returned after midnight in the task-start snapshot", rows)
	}
}
