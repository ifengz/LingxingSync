package db

import (
	"context"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
)

func TestAdPartitionWhereUsesVerifiedScopes(t *testing.T) {
	tests := []struct {
		table  string
		where  string
		hasSID bool
	}{
		{table: "ls_ad_sp_product", where: "`account_id` = ? AND `sid` = ? AND `profile_id` = ? AND `report_date` = ?", hasSID: true},
		{table: "ls_ad_sd_campaign", where: "`account_id` = ? AND `sid` = ? AND `profile_id` = ? AND `report_date` = ?", hasSID: true},
		{table: "ls_ad_hsa_campaign", where: "`account_id` = ? AND `sid` = ? AND `profile_id` = ? AND `report_date` = ?", hasSID: true},
		{table: "ls_ad_vc_sp_product", where: "`account_id` = ? AND `profile_id` = ? AND `report_date` = ?", hasSID: false},
		{table: "ls_ad_vc_sd_product", where: "`account_id` = ? AND `profile_id` = ? AND `report_date` = ?", hasSID: false},
		{table: "ls_ad_vc_hsa_product", where: "`account_id` = ? AND `profile_id` = ? AND `report_date` = ?", hasSID: false},
	}
	for _, tt := range tests {
		t.Run(tt.table, func(t *testing.T) {
			where, hasSID := adPartitionWhere(tt.table)
			if where != tt.where || hasSID != tt.hasSID {
				t.Fatalf("adPartitionWhere(%q) = %q, %v; want %q, %v", tt.table, where, hasSID, tt.where, tt.hasSID)
			}
		})
	}
	if where, ok := adPartitionWhere("ls_sales_orders"); ok || where != "" {
		t.Fatalf("non-ad table accepted for partition replacement: %q, %v", where, ok)
	}
}

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

func TestBuildUpsertColumnsPreservesStoreRunTimes(t *testing.T) {
	cols := buildUpsertColumns([]string{"sid", "store_name", "synced_at", "last_attempt_at", "last_success_at"})
	want := []string{"account_id", "sid", "store_name"}
	if !reflect.DeepEqual(cols, want) {
		t.Fatalf("store metadata columns = %#v, want %#v", cols, want)
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

func TestUpsertRowsRejectsBlankFNSKUForFBAInventory(t *testing.T) {
	for _, fnsku := range []any{"", nil} {
		rows := []map[string]any{{"sid": "store-1", "fnsku": fnsku}}
		if err := validateFBAInventoryRows("ls_fba_inventory", []string{"sid", "fnsku"}, rows); err == nil || !strings.Contains(err.Error(), "empty FNSKU") {
			t.Fatalf("blank FNSKU=%#v error=%v, want explicit rejection", fnsku, err)
		}
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

func TestReplaceAdPartitionReplacesOnlyOneScope(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the ad partition integration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接迁移测试数据库失败: %v", err)
	}
	defer dbx.Close()
	dbx.SetMaxOpenConns(1)
	dbx.SetMaxIdleConns(1)
	_, err = dbx.Exec(`CREATE TEMPORARY TABLE ls_ad_sp_product (
account_id VARCHAR(64) NOT NULL,
sid VARCHAR(64) NOT NULL,
profile_id VARCHAR(64) NOT NULL,
report_date DATE NOT NULL,
ad_id BIGINT NOT NULL,
asin VARCHAR(32) NULL,
synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
PRIMARY KEY (account_id, sid, profile_id, report_date, ad_id)
)`)
	if err != nil {
		t.Fatalf("创建广告测试表失败: %v", err)
	}
	const accountID = "__ad_partition_test__"
	const sid = "store-1"
	const profileID = "profile-1"
	const reportDate = "2026-09-09"
	columns := []string{"account_id", "sid", "profile_id", "report_date", "ad_id", "asin", "synced_at"}
	seed := []map[string]any{
		{"sid": sid, "profile_id": profileID, "report_date": reportDate, "ad_id": 1, "asin": "OLD"},
		{"sid": sid, "profile_id": profileID, "report_date": reportDate, "ad_id": 2, "asin": "KEEP?"},
		{"sid": sid, "profile_id": profileID, "report_date": "2026-09-08", "ad_id": 3, "asin": "OTHER-DATE"},
	}
	if err := UpsertRows(dbx, "ls_ad_sp_product", seed, columns, nil, accountID); err != nil {
		t.Fatalf("写入初始广告分区失败: %v", err)
	}
	rows := []map[string]any{
		{"sid": sid, "profile_id": profileID, "report_date": reportDate, "ad_id": 2, "asin": "NEW"},
		{"sid": sid, "profile_id": profileID, "report_date": reportDate, "ad_id": 4, "asin": "FRESH"},
	}
	if err := ReplaceAdPartition(context.Background(), dbx, "ls_ad_sp_product", accountID, sid, profileID, reportDate, rows, columns, nil); err != nil {
		t.Fatalf("替换广告分区失败: %v", err)
	}
	var count int
	if err := dbx.Get(&count, "SELECT COUNT(*) FROM ls_ad_sp_product WHERE account_id = ? AND sid = ? AND profile_id = ? AND report_date = ?", accountID, sid, profileID, reportDate); err != nil {
		t.Fatalf("读取替换分区行数失败: %v", err)
	}
	if count != 2 {
		t.Fatalf("替换分区行数=%d, want 2", count)
	}
	var oldCount int
	if err := dbx.Get(&oldCount, "SELECT COUNT(*) FROM ls_ad_sp_product WHERE account_id = ? AND ad_id = ?", accountID, 1); err != nil {
		t.Fatalf("读取旧行失败: %v", err)
	}
	if oldCount != 0 {
		t.Fatalf("旧广告行仍存在: %d", oldCount)
	}
	if err := dbx.Get(&count, "SELECT COUNT(*) FROM ls_ad_sp_product WHERE account_id = ? AND report_date = ?", accountID, "2026-09-08"); err != nil {
		t.Fatalf("读取其他日期失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("其他日期行数=%d, want 1", count)
	}
}

func TestReplaceAdPartitionRollsBackWhenInsertFails(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the ad partition rollback integration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接迁移测试数据库失败: %v", err)
	}
	defer dbx.Close()
	dbx.SetMaxOpenConns(1)
	dbx.SetMaxIdleConns(1)
	_, err = dbx.Exec(`CREATE TEMPORARY TABLE ls_ad_sp_product (
account_id VARCHAR(64) NOT NULL,
sid VARCHAR(64) NOT NULL,
profile_id VARCHAR(64) NOT NULL,
report_date DATE NOT NULL,
ad_id BIGINT NOT NULL,
asin VARCHAR(32) NULL,
synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
PRIMARY KEY (account_id, sid, profile_id, report_date, ad_id)
)`)
	if err != nil {
		t.Fatalf("创建广告回滚测试表失败: %v", err)
	}
	const accountID = "__ad_partition_rollback__"
	const sid = "store-1"
	const profileID = "profile-1"
	const reportDate = "2026-09-09"
	validColumns := []string{"account_id", "sid", "profile_id", "report_date", "ad_id", "asin", "synced_at"}
	seed := []map[string]any{{"sid": sid, "profile_id": profileID, "report_date": reportDate, "ad_id": 9, "asin": "OLD"}}
	if err := UpsertRows(dbx, "ls_ad_sp_product", seed, validColumns, nil, accountID); err != nil {
		t.Fatalf("写入回滚测试初始行失败: %v", err)
	}
	badColumns := append(validColumns, "missing_column")
	rows := []map[string]any{{"sid": sid, "profile_id": profileID, "report_date": reportDate, "ad_id": 10, "asin": "NEW"}}
	if err := ReplaceAdPartition(context.Background(), dbx, "ls_ad_sp_product", accountID, sid, profileID, reportDate, rows, badColumns, nil); err == nil {
		t.Fatal("缺失列写入应失败")
	}
	var count int
	if err := dbx.Get(&count, "SELECT COUNT(*) FROM ls_ad_sp_product WHERE account_id = ? AND sid = ? AND profile_id = ? AND report_date = ? AND ad_id = ?", accountID, sid, profileID, reportDate, 9); err != nil {
		t.Fatalf("读取回滚旧行失败: %v", err)
	}
	if count != 1 {
		t.Fatalf("事务失败后旧分区行数=%d, want 1", count)
	}
}
