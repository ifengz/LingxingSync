package db

import (
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestListingDailyMigrationHasOneFactTableAndExplicitIdentityDimension(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/033_add_listing_daily_metrics.sql")
	if err != nil {
		t.Fatalf("读取 listing 日维迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS LISTING_DIMENSIONS",
		"UNIQUE KEY UQ_LISTING_DIMENSION (STORE_ID, CHANNEL, IDENTITY_SCOPE, IDENTITY_KEY)",
		"CREATE TABLE IF NOT EXISTS LISTING_DAILY_METRICS",
		"PRIMARY KEY (LISTING_DIMENSION_ID, BUSINESS_DATE)",
		"CREATE TABLE IF NOT EXISTS LISTING_DAILY_RECONCILIATIONS",
		"PRIMARY KEY (REPORT_AUDIT_ID, BUSINESS_DATE)",
		"REPORT_TASK_ID", "MISSING_IN_DB", "MISSING_IN_REPORT", "FIELD_DIFFS", "ERROR_MESSAGE",
		"UPDATED_AT           DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)",
		"SALES_UNITS", "SALES_AMOUNT", "RETURNS_QTY", "INVENTORY_SELLABLE", "INVENTORY_INBOUND", "INVENTORY_RESERVED", "INVENTORY_UNFULFILLABLE", "INVENTORY_LOCAL_WAREHOUSE", "INVENTORY_UNHEALTHY_UNITS", "INVENTORY_AGED90_SELLABLE_UNITS", "INVENTORY_SELL_THROUGH_RATE", "INVENTORY_RECEIVE_FILL_RATE", "INVENTORY_VENDOR_CONFIRMATION_RATE", "INVENTORY_AVG_LEAD_TIME_DAYS", "INVENTORY_SELLABLE_COST", "INVENTORY_UNFULFILLABLE_COST", "INVENTORY_AGED90_COST", "INVENTORY_UNHEALTHY_COST", "INVENTORY_INBOUND_COST", "INVENTORY_CURRENCY", "INVENTORY_INBOUND_RECEIVING", "INVENTORY_INBOUND_SHIPPED", "INVENTORY_INBOUND_WORKING", "INVENTORY_RESERVED_CUSTOMER_ORDERS", "INVENTORY_RESERVED_FC_PROCESSING", "INVENTORY_RESERVED_FC_TRANSFERS",
		"SESSIONS_DESKTOP", "SESSIONS_MOBILE", "SESSIONS_TOTAL", "REVIEW_COUNT", "RATING",
		"SP_SPEND", "SD_SPEND", "HSA_SPEND", "SB_SPEND", "VERIFIED_FIELDS", "JSON NOT NULL",
		"IDENTITY_SCOPE = 'STORE' AND CHANNEL IN ('HSA', 'SB') AND ASIN IS NULL AND SKU IS NULL",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("listing 日维迁移缺少 %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("listing 日维迁移不得使用破坏性语句 %s", forbidden)
		}
	}
	if strings.Contains(sql, "INVENTORY_SNAPSHOT") {
		t.Fatal("listing 日维迁移不得保留含糊 inventory_snapshot 字段")
	}
}

func TestListingDailyTimestampPrecisionMigrationUpgradesExistingFacts(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/035_upgrade_listing_daily_updated_at_precision.sql")
	if err != nil {
		t.Fatalf("读取 listing 日维时间精度迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"INFORMATION_SCHEMA.COLUMNS",
		"TABLE_NAME = 'LISTING_DAILY_METRICS'",
		"COLUMN_NAME = 'UPDATED_AT'",
		"DATETIME_PRECISION",
		"ALTER TABLE LISTING_DAILY_METRICS MODIFY UPDATED_AT DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("listing 日维时间精度迁移缺少 %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("listing 日维时间精度迁移不得使用破坏性语句 %s", forbidden)
		}
	}
}

func TestListingDailyLegacySchemaMigrationAddsCurrentMetricColumns(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/036_upgrade_listing_daily_metric_columns.sql")
	if err != nil {
		t.Fatalf("读取 listing 日维旧表升级迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"INFORMATION_SCHEMA.COLUMNS",
		"TABLE_NAME = 'LISTING_DAILY_METRICS'",
		"DROP CHECK CHK_LISTING_DAILY_SOURCES",
		"CHANGE COLUMN INVENTORY_SNAPSHOT INVENTORY_SELLABLE BIGINT NULL",
		"CHANGE COLUMN INVENTORY_SNAPSHOT_SOURCE INVENTORY_SELLABLE_SOURCE VARCHAR(16) NOT NULL DEFAULT ''",
		"ADD COLUMN INVENTORY_INBOUND BIGINT NULL",
		"ADD COLUMN VERIFIED_FIELDS JSON NOT NULL",
		"ADD CONSTRAINT CHK_LISTING_DAILY_SOURCES",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("listing 日维旧表升级迁移缺少 %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("listing 日维旧表升级迁移不得使用破坏性语句 %s", forbidden)
		}
	}
}

func TestListingDailyTimestampPrecisionMigrationAgainstLocalMySQL(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the listing daily precision migration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接迁移测试数据库失败: %v", err)
	}
	defer dbx.Close()

	const table = "listing_daily_metrics_precision_035_test"
	defer func() { _, _ = dbx.Exec("DROP TABLE IF EXISTS " + table) }()
	if _, err := dbx.Exec("DROP TABLE IF EXISTS " + table); err != nil {
		t.Fatal(err)
	}
	if _, err := dbx.Exec("CREATE TABLE " + table + " (id BIGINT PRIMARY KEY, updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP)"); err != nil {
		t.Fatal(err)
	}
	if _, err := dbx.Exec("INSERT INTO " + table + " (id) VALUES (1)"); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("../../migrations/035_upgrade_listing_daily_updated_at_precision.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationSQL := strings.ReplaceAll(string(raw), "listing_daily_metrics", table)
	for attempt := 1; attempt <= 2; attempt++ {
		if _, err := dbx.Exec(migrationSQL); err != nil {
			t.Fatalf("时间精度迁移第 %d 次执行失败: %v", attempt, err)
		}
	}
	var precision int
	if err := dbx.Get(&precision, `SELECT DATETIME_PRECISION FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = 'updated_at'`, table); err != nil {
		t.Fatal(err)
	}
	if precision != 6 {
		t.Fatalf("updated_at precision = %d, want 6", precision)
	}
	var rows int
	if err := dbx.Get(&rows, "SELECT COUNT(*) FROM "+table); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("migration changed existing rows: %d", rows)
	}
}
