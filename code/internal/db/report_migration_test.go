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

func TestCustomerShipmentReplacementsMigrationContractIsIndependent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/042_add_fba_customer_shipment_replacements_report.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"LS_FBA_FULFILLMENT_CUSTOMER_SHIPMENT_REPLACEMENTS", "REPLACEMENT-AMAZON-ORDER-ID", "ORIGINAL-AMAZON-ORDER-ID", "CREATE TABLE IF NOT EXISTS"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("replacement migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("replacement migration contains destructive %s", forbidden)
		}
	}
}

func TestFBAReimbursementsMigrationContractIsIndependent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/043_add_fba_reimbursements_report.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"LS_FBA_REIMBURSEMENTS", "QUANTITY-REIMBURSED-TOTAL", "ORIGINAL-REIMBURSEMENT-TYPE", "CREATE TABLE IF NOT EXISTS"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("reimbursements migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("reimbursements migration contains destructive %s", forbidden)
		}
	}
}

func TestAFNInventoryByCountryMigrationContractIsIndependent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/044_add_afn_inventory_by_country_report.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"LS_AFN_INVENTORY_BY_COUNTRY", "QUANTITY-FOR-LOCAL-FULFILLMENT", "CREATE TABLE IF NOT EXISTS"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("AFN by country migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("AFN by country migration contains destructive %s", forbidden)
		}
	}
}

func TestFeeReportMigrationsAreIndependent(t *testing.T) {
	for _, test := range []struct {
		file  string
		table string
		field string
	}{
		{"../../migrations/045_add_fba_storage_fee_charges_report.sql", "LS_FBA_STORAGE_FEE_CHARGES", "ESTIMATED_MONTHLY_STORAGE_FEE"},
		{"../../migrations/046_add_fba_overage_fee_charges_report.sql", "LS_FBA_OVERAGE_FEE_CHARGES", "CHARGED_FEE_AMOUNT"},
		{"../../migrations/047_add_fba_longterm_storage_fee_charges_report.sql", "LS_FBA_LONGTERM_STORAGE_FEE_CHARGES", "AMOUNT-CHARGED"},
		{"../../migrations/048_add_fba_stranded_inventory_report.sql", "LS_FBA_STRANDED_INVENTORY", "DATE-STRANDED"},
		{"../../migrations/049_add_fba_estimated_fees_report.sql", "LS_FBA_ESTIMATED_FEES", "ESTIMATED-FEE-TOTAL"},
		{"../../migrations/050_add_fba_inbound_noncompliance_report.sql", "LS_FBA_INBOUND_NONCOMPLIANCE", "ISSUE-REPORTED-DATE"},
		{"../../migrations/051_add_fba_recommended_removal_report.sql", "LS_FBA_RECOMMENDED_REMOVALS", "SELLABLE-REMOVAL-QUANTITY"},
		{"../../migrations/052_add_fba_removal_order_report.sql", "LS_FBA_REMOVAL_ORDER_DETAILS", "REQUESTED-QUANTITY"},
		{"../../migrations/053_add_fba_removal_shipment_report.sql", "LS_FBA_REMOVAL_SHIPMENT_DETAILS", "TRACKING-NUMBER"},
		{"../../migrations/054_add_amazon_all_orders_report.sql", "LS_AMAZON_ALL_ORDERS_BY_ORDER_DATE", "AMAZON-ORDER-ID"},
		{"../../migrations/056_add_amazon_fulfilled_shipments_report.sql", "LS_AMAZON_FULFILLED_SHIPMENTS", "SHIPMENT-ITEM-ID"},
	} {
		t.Run(test.table, func(t *testing.T) {
			raw, err := os.ReadFile(test.file)
			if err != nil {
				t.Fatal(err)
			}
			sql := strings.ToUpper(string(raw))
			for _, want := range []string{test.table, test.field, "CREATE TABLE IF NOT EXISTS"} {
				if !strings.Contains(sql, want) {
					t.Fatalf("migration missing %q", want)
				}
			}
			for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
				if strings.Contains(sql, forbidden) {
					t.Fatalf("migration contains destructive %s", forbidden)
				}
			}
		})
	}
}

func TestStorageFeeProductionColumnsMigrationIsGuardedAndNonDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/060_add_fba_storage_fee_charges_production_columns.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"INFORMATION_SCHEMA.COLUMNS", "LS_FBA_STORAGE_FEE_CHARGES", "ALTER TABLE", "SKU",
		"STORAGE_UTILIZATION_RATIO", "STORAGE_UTILIZATION_RATIO_UNITS", "BASE_RATE", "UTILIZATION_SURCHARGE_RATE",
		"AVG_QTY_FOR_SUS", "EST_VOL_FOR_SUS", "EST_BASE_MSF", "EST_SUS",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("storage fee migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN", "DELETE FROM", "TRUNCATE", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("storage fee migration contains destructive %s", forbidden)
		}
	}
}

func TestAllOrdersVariantMigrationIsGuardedAndNonDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/055_add_amazon_all_orders_variant_columns.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"INFORMATION_SCHEMA.COLUMNS", "ORDER-ITEM-ID", "CPF", "ALTER TABLE LS_AMAZON_ALL_ORDERS_BY_ORDER_DATE ADD COLUMN"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN", "DELETE FROM", "TRUNCATE", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains destructive %s", forbidden)
		}
	}
}

func TestAllOrdersSignatureConfirmationMigrationIsGuardedAndNonDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/058_add_amazon_all_orders_signature_confirmation.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"INFORMATION_SCHEMA.COLUMNS", "SIGNATURE-CONFIRMATION-RECOMMENDED", "ALTER TABLE LS_AMAZON_ALL_ORDERS_BY_ORDER_DATE ADD COLUMN"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN", "DELETE FROM", "TRUNCATE", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("migration contains destructive %s", forbidden)
		}
	}
}

func TestMYIProductionColumnsMigrationIsGuardedAndNonDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/059_add_fba_myi_production_columns.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"INFORMATION_SCHEMA.COLUMNS", "LS_FBA_MYI_UNSUPPRESSED_INVENTORY", "LS_FBA_MYI_ALL_INVENTORY", "AFN-FC-TRANSFER-QUANTITY", "AFN-ONHAND-BUYABLE-QUANTITY", "STORE", "ALTER TABLE"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("MYI migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN", "DELETE FROM", "TRUNCATE", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("MYI migration contains destructive %s", forbidden)
		}
	}
}

func TestEstimatedFeesProductionColumnsMigrationIsGuardedAndNonDestructive(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/061_add_fba_estimated_fees_production_columns.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{"INFORMATION_SCHEMA.COLUMNS", "LS_FBA_ESTIMATED_FEES", "ALTER TABLE", "AMAZON-STORE", "PRODUCT-SIZE-TIER", "ESTIMATED-FUTURE-FEE", "EXPECTED-FUTURE-FULFILLMENT-FEE-PER-UNIT"} {
		if !strings.Contains(sql, want) {
			t.Fatalf("Estimated Fees migration missing %q", want)
		}
	}
	for _, forbidden := range []string{"DROP TABLE", "DROP COLUMN", "DELETE FROM", "TRUNCATE", "UPDATE "} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("Estimated Fees migration contains destructive %s", forbidden)
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
