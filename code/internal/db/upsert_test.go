package db

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestNormalizeUpsertValueIsColumnAware(t *testing.T) {
	if got := normalizeUpsertValue("", true); got != nil {
		t.Fatalf("JSON 空字符串 = %#v, want nil", got)
	}
	if got := normalizeUpsertValue("", false); got != "" {
		t.Fatalf("普通列空字符串 = %#v, want empty string", got)
	}
	if got := normalizeUpsertValue(`{"key":"value"}`, true); got != `{"key":"value"}` {
		t.Fatalf("JSON 字符串被错误改写: %#v", got)
	}
	if got := normalizeUpsertValue(map[string]any{"key": "value"}, true); !reflect.DeepEqual(got, `{"key":"value"}`) {
		t.Fatalf("JSON 对象序列化结果 = %#v", got)
	}
}

func TestBuildUpsertStatementTouchesSyncedAtForReturnedSnapshotRows(t *testing.T) {
	stmt := buildUpsertStatement([]string{"account_id", "sid", "fnsku", "sellable"}, true, 2)
	if !strings.Contains(stmt, "`synced_at` = CURRENT_TIMESTAMP") {
		t.Fatalf("snapshot upsert must touch synced_at, statement=%s", stmt)
	}
	if got := strings.Count(stmt, "(?,?,?,?)"); got != 2 {
		t.Fatalf("batch placeholders=%d, want 2; statement=%s", got, stmt)
	}
}

func TestBuildUpsertStatementDoesNotInventSyncedAtColumn(t *testing.T) {
	stmt := buildUpsertStatement([]string{"account_id", "sid", "fnsku", "sellable"}, false, 1)
	if strings.Contains(stmt, "synced_at") {
		t.Fatalf("upsert must not write a column absent from the table, statement=%s", stmt)
	}
}

func TestOnlyCurrentStateFBAInventoryUsesSnapshotTouch(t *testing.T) {
	columns := []string{"account_id", "sid", "synced_at"}
	if !shouldTouchSnapshot("ls_fba_inventory", columns) {
		t.Fatal("FBA current-state inventory must refresh its snapshot timestamp")
	}
	if shouldTouchSnapshot("ls_sc_sales_report", columns) {
		t.Fatal("non-snapshot raw tables must retain ordinary upsert semantics")
	}
}

func TestUpsertRowsTouchesOnlySnapshotRowsReturnedToday(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the snapshot upsert integration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接迁移测试数据库失败: %v", err)
	}
	defer dbx.Close()
	dbx.SetMaxOpenConns(1)
	dbx.SetMaxIdleConns(1)

	_, err = dbx.Exec(`CREATE TEMPORARY TABLE ls_fba_inventory (
account_id VARCHAR(64) NOT NULL,
sid VARCHAR(64) NOT NULL,
fnsku VARCHAR(64) NOT NULL,
sellable BIGINT NULL,
synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
PRIMARY KEY (account_id, sid, fnsku)
)`)
	if err != nil {
		t.Fatalf("创建快照测试表失败: %v", err)
	}
	columns := []string{"account_id", "sid", "fnsku", "sellable", "synced_at"}
	rows := []map[string]any{
		{"sid": "store-1", "fnsku": "returned", "sellable": int64(3)},
		{"sid": "store-1", "fnsku": "not-returned", "sellable": int64(5)},
	}
	if err := UpsertRows(dbx, "ls_fba_inventory", rows, columns, nil, "account-1"); err != nil {
		t.Fatalf("写入初始快照失败: %v", err)
	}
	old := time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC)
	if _, err := dbx.Exec("UPDATE ls_fba_inventory SET synced_at = ? WHERE account_id = ? AND sid = ? AND fnsku IN (?, ?)", old, "account-1", "store-1", "returned", "not-returned"); err != nil {
		t.Fatalf("固定历史快照时间失败: %v", err)
	}
	if err := UpsertRows(dbx, "ls_fba_inventory", rows[:1], columns, nil, "account-1"); err != nil {
		t.Fatalf("重复写入本次返回行失败: %v", err)
	}

	var got []struct {
		FNSKU    string    `db:"fnsku"`
		SyncedAt time.Time `db:"synced_at"`
	}
	if err := dbx.Select(&got, "SELECT fnsku, synced_at FROM ls_fba_inventory WHERE account_id = ? AND sid = ? AND fnsku IN (?, ?) ORDER BY fnsku", "account-1", "store-1", "returned", "not-returned"); err != nil {
		t.Fatalf("读取快照测试结果失败: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("快照行数=%d, want 2", len(got))
	}
	for _, row := range got {
		switch row.FNSKU {
		case "returned":
			if !row.SyncedAt.After(old) {
				t.Fatalf("本次返回行 synced_at=%s, want after %s", row.SyncedAt, old)
			}
		case "not-returned":
			if !row.SyncedAt.Equal(old) {
				t.Fatalf("未返回行 synced_at=%s, want unchanged %s", row.SyncedAt, old)
			}
		}
	}
}
