package db

import (
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestCustomerReturnsReportMigrationContractIsIndependentAndIdempotent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/032_add_fba_customer_returns_report.sql")
	if err != nil {
		t.Fatalf("读取正式报表迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS LS_REPORT_EXPORT_TASKS",
		"CREATE TABLE IF NOT EXISTS LS_FBA_FULFILLMENT_CUSTOMER_RETURNS",
		"INDEX IDX_REPORT_EXPORT_SCOPE (ACCOUNT_ID, SELLER_ID, STORE_ID, REPORT_TYPE, DATE_FROM, DATE_TO)",
		"DOWNLOAD_URL",
		"REGION",
		"MARKETPLACE_IDS JSON",
		"ACTIVE_SCOPE_KEY",
		"UNIQUE KEY UQ_REPORT_EXPORT_ACTIVE_SCOPE (ACTIVE_SCOPE_KEY)",
		"PRIMARY KEY (REPORT_TASK_ID, `ROW_NUMBER`)",
		"`RETURN-DATE`",
		"`ORDER-ID`",
		"`FULFILLMENT-CENTER-ID`",
		"`DETAILED-DISPOSITION`",
		"`LICENSE-PLATE-NUMBER`",
		"`CUSTOMER-COMMENTS`",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("正式报表迁移缺少合同 %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("正式报表迁移不得包含破坏性语句 %s", forbidden)
		}
	}
	if strings.Contains(sql, "UNIQUE KEY UQ_REPORT_EXPORT_SCOPE") {
		t.Fatal("成功的同日期范围报告必须允许后续重新生成，不能用范围唯一键永久跳过")
	}
}

func TestReservedInventoryFCTransfersMigrationIsIdempotentAndNonDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/041_add_reserved_inventory_fc_transfers.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"INFORMATION_SCHEMA.COLUMNS", "LS_FBA_RESERVED_INVENTORY", "RESERVED_FC-TRANSFERS", "ALTER TABLE"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Reserved Inventory migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("Reserved Inventory migration contains destructive %s", forbidden)
		}
	}
}

func TestCustomerReturnsReportMigrationIsRepeatableAgainstLocalMySQL(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the local report migration idempotency test")
	}
	db, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接迁移测试数据库失败: %v", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("首次执行全量迁移失败: %v", err)
	}
	if err := RunMigrations(db, "../../migrations"); err != nil {
		t.Fatalf("重复执行全量迁移失败: %v", err)
	}

	for _, table := range []string{"ls_report_export_tasks", "ls_fba_fulfillment_customer_returns"} {
		var count int
		if err := db.Get(&count, "SELECT COUNT(*) FROM INFORMATION_SCHEMA.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?", table); err != nil {
			t.Fatalf("检查 %s 是否存在失败: %v", table, err)
		}
		if count != 1 {
			t.Fatalf("重复迁移后 %s 存在数=%d, want 1", table, count)
		}
	}
}
