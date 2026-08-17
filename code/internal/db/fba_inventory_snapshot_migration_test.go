package db

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func TestFBAInventoryDailySnapshotMigrationDefinesDatedHistory(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/063_add_fba_inventory_daily_snapshots.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS FBA_INVENTORY_DAILY_SNAPSHOTS",
		"SNAPSHOT_DATE DATE NOT NULL",
		"SOURCE_SYNCED_AT DATETIME",
		"UPDATED_AT DATETIME(6) NOT NULL",
		"PRIMARY KEY (ACCOUNT_ID, SID, FNSKU, SNAPSHOT_DATE)",
		"INDEX IDX_FBA_INVENTORY_SNAPSHOT_DATE (SID, SNAPSHOT_DATE, ACCOUNT_ID)",
		"INDEX IDX_FBA_INVENTORY_SNAPSHOT_CHANGES (SID, UPDATED_AT, ACCOUNT_ID, FNSKU, SNAPSHOT_DATE)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("FBA inventory history migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE LS_FBA_INVENTORY", "TRUNCATE LS_FBA_INVENTORY", "DELETE FROM LS_FBA_INVENTORY"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("FBA history migration modifies raw current-state table with %q", forbidden)
		}
	}
}

func TestFBAInventoryDailySnapshotMigrationCoversEveryRawColumn(t *testing.T) {
	rawMigration, err := os.ReadFile("../../migrations/006_rebuild_ls_inventory.sql")
	if err != nil {
		t.Fatal(err)
	}
	historyMigration, err := os.ReadFile("../../migrations/063_add_fba_inventory_daily_snapshots.sql")
	if err != nil {
		t.Fatal(err)
	}
	rawColumns := createTableColumns(t, string(rawMigration), "ls_fba_inventory")
	historyColumns := createTableColumns(t, string(historyMigration), "fba_inventory_daily_snapshots")
	delete(rawColumns, "synced_at")
	for column := range rawColumns {
		if _, ok := historyColumns[column]; !ok {
			t.Fatalf("FBA inventory history is missing raw column %q", column)
		}
	}
}

func createTableColumns(t *testing.T, migration, table string) map[string]struct{} {
	t.Helper()
	pattern := regexp.MustCompile(`(?is)CREATE TABLE IF NOT EXISTS\s+` + regexp.QuoteMeta(table) + `\s*\((.*?)\)\s*ENGINE=`)
	match := pattern.FindStringSubmatch(migration)
	if len(match) != 2 {
		t.Fatalf("CREATE TABLE block not found for %s", table)
	}
	columns := make(map[string]struct{})
	for _, line := range strings.Split(match[1], "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "--") || strings.HasPrefix(strings.ToUpper(line), "PRIMARY ") || strings.HasPrefix(strings.ToUpper(line), "INDEX ") {
			continue
		}
		name := strings.Trim(strings.Fields(line)[0], "`,")
		columns[name] = struct{}{}
	}
	return columns
}
