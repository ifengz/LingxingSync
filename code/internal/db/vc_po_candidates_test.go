package db

import (
	"os"
	"testing"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
)

func TestQueryRecentVCPOCandidatesRejectsReversedWindow(t *testing.T) {
	_, err := QueryRecentVCPOCandidates(nil, "sc_us_1", "2026-08-11", "2026-08-10")
	if err == nil {
		t.Fatal("reversed candidate window must fail before querying the database")
	}
}

func TestQueryRecentVCPOCandidatesAgainstLocalSchema(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the local VC PO candidate integration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("connect candidate test database: %v", err)
	}
	defer dbx.Close()

	const accountID = "vc_po_candidate_test"
	const otherAccountID = "vc_po_candidate_other"
	defer func() {
		_, _ = dbx.Exec("DELETE FROM ls_vc_orders WHERE account_id IN (?, ?)", accountID, otherAccountID)
	}()
	_, _ = dbx.Exec("DELETE FROM ls_vc_orders WHERE account_id IN (?, ?)", accountID, otherAccountID)

	const insert = `
INSERT INTO ls_vc_orders
    (account_id, local_po_number, vc_store_id, purchase_order_type, purchase_order_date, gmt_modified)
VALUES (?, ?, ?, ?, ?, ?)`
	rows := [][]any{
		{accountID, "po-in", "store-1", 1, "2026-08-09 12:00:00", "2026-08-09 13:00:00"},
		{accountID, "po-old-updated", "store-1", 1, "2026-07-01 12:00:00", "2026-08-09 13:00:00"},
		{accountID, "po-not-updated", "store-1", 1, "2026-08-09 12:00:00", "2026-08-01 13:00:00"},
		{accountID, "po-df", "store-1", 0, "2026-08-09 12:00:00", "2026-08-09 13:00:00"},
		{otherAccountID, "po-other", "store-2", 1, "2026-08-09 12:00:00", "2026-08-09 13:00:00"},
	}
	for _, row := range rows {
		if _, err := dbx.Exec(insert, row...); err != nil {
			t.Fatalf("insert candidate fixture: %v", err)
		}
	}

	candidates, err := QueryRecentVCPOCandidates(dbx, accountID, "2026-08-08", "2026-08-10")
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 || candidates[0].VCStoreID != "store-1" || candidates[0].LocalPONumber != "po-in" || candidates[1].LocalPONumber != "po-old-updated" {
		t.Fatalf("candidate scope = %#v", candidates)
	}

	if _, err := dbx.Exec(insert, accountID, "po-missing-store", nil, 1, "2026-08-09 12:00:00", "2026-08-09 13:00:00"); err != nil {
		t.Fatalf("insert missing-store fixture: %v", err)
	}
	if _, err := QueryRecentVCPOCandidates(dbx, accountID, "2026-08-08", "2026-08-10"); err == nil {
		t.Fatal("recent PO without vc_store_id must fail before detail calls")
	}
}
