package db

import (
	"os"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"
)

func TestRenameAccountMigrationPreservesConflicts(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/009_rename_account.sql")
	if err != nil {
		t.Fatalf("读取 009 迁移失败: %v", err)
	}
	legacySQL := strings.ToUpper(string(raw))
	if strings.Contains(legacySQL, "DELETE OLD FROM") {
		t.Fatal("账号迁移不得通过 DELETE 删除主键冲突的旧数据")
	}
	postRaw, err := os.ReadFile("../../migrations/015_rename_account_non_destructive.sql")
	if err != nil {
		t.Fatalf("读取 015 迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(postRaw))
	if !strings.Contains(sql, "MIGRATION_ACCOUNT_CONFLICTS") {
		t.Fatal("账号迁移必须记录主键冲突，让受影响接口可见地不可用")
	}
	if !strings.Contains(sql, "LEFT JOIN") || !strings.Contains(sql, "TARGET.ACCOUNT_ID IS NULL") {
		t.Fatal("账号迁移必须只更新不冲突的行")
	}
}

func TestFBAInventoryTableRenameMigrationIsPresent(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/001_rename_ls_inventory_to_ls_fba_inventory.sql")
	if err != nil {
		t.Fatalf("读取 FBA 库存表迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	if !strings.Contains(sql, "RENAME TABLE LS_INVENTORY TO LS_FBA_INVENTORY") {
		t.Fatal("FBA 库存表迁移必须把旧表重命名为 ls_fba_inventory")
	}
	if !strings.Contains(sql, "INFORMATION_SCHEMA.TABLES") {
		t.Fatal("FBA 库存表迁移必须先检查旧表和新表是否存在")
	}
}

func TestVCReportStoreScopeMigrationPreservesLegacyRows(t *testing.T) {
	baseRaw, err := os.ReadFile("../../migrations/008_add_vc_report_tables.sql")
	if err != nil {
		t.Fatalf("读取 VC 报表基础迁移失败: %v", err)
	}
	baseSQL := strings.ToUpper(string(baseRaw))
	for _, want := range []string{
		"PRIMARY KEY (ACCOUNT_ID, SID, ASIN, `DATE`)",
		"PRIMARY KEY (ACCOUNT_ID, SID, ASIN, STARTTIME)",
	} {
		if !strings.Contains(baseSQL, want) {
			t.Fatalf("VC 报表基础迁移缺少 %q", want)
		}
	}

	raw, err := os.ReadFile("../../migrations/023_scope_vc_reports_by_store.sql")
	if err != nil {
		t.Fatalf("读取 VC 报表店铺键迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, destructive := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, destructive) {
			t.Fatalf("VC 报表迁移不得使用破坏性语句 %s", destructive)
		}
	}
	for _, want := range []string{
		"LS_VC_SALES_REPORT_LEGACY_UNSCOPED",
		"PRIMARY KEY (ACCOUNT_ID, SID, ASIN, `DATE`)",
		"LS_VC_REALTIME_SALES_LEGACY_UNSCOPED",
		"PRIMARY KEY (ACCOUNT_ID, SID, ASIN, STARTTIME)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("VC 报表迁移缺少 %q", want)
		}
	}
}

func TestCampaignAdMigrationsMatchVerifiedKeys(t *testing.T) {
	tests := []struct {
		file  string
		table string
	}{
		{file: "../../migrations/024_add_ls_ad_sd_campaign.sql", table: "LS_AD_SD_CAMPAIGN"},
		{file: "../../migrations/025_add_ls_ad_hsa_campaign.sql", table: "LS_AD_HSA_CAMPAIGN"},
	}
	for _, tc := range tests {
		t.Run(tc.table, func(t *testing.T) {
			raw, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("读取广告活动迁移失败: %v", err)
			}
			sql := strings.ToUpper(string(raw))
			if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+tc.table) {
				t.Fatalf("迁移未创建 %s", tc.table)
			}
			if !strings.Contains(sql, "PRIMARY KEY (ACCOUNT_ID, SID, PROFILE_ID, REPORT_DATE, CAMPAIGN_ID)") {
				t.Fatal("广告活动表主键必须隔离账号、店铺、profile、日期和 campaign")
			}
		})
	}
}

func TestSCRevenueMigrationKeepsMetricInSeparateRawTable(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/026_add_ls_sc_sales_revenue.sql")
	if err != nil {
		t.Fatalf("读取 SC 销售额迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS LS_SC_SALES_REVENUE",
		"MAP_VALUE",
		"CURRENCY_CODE",
		"PRIMARY KEY (ACCOUNT_ID, SID, R_DATE, ASIN)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("SC 销售额迁移缺少 %q", want)
		}
	}
	for _, destructive := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, destructive) {
			t.Fatalf("SC 销售额迁移不得使用破坏性语句 %s", destructive)
		}
	}
}

func TestStockAndAddressMigrationsPreserveVerifiedBusinessKeys(t *testing.T) {
	tests := []struct {
		file  string
		table string
		key   string
	}{
		{
			file:  "../../migrations/027_add_ls_sc_removal_orders.sql",
			table: "LS_SC_REMOVAL_ORDERS",
			key:   "PRIMARY KEY (ACCOUNT_ID, SELLER_ID, ORDER_ID, SKU, FNSKU, DISPOSITION)",
		},
		{
			file:  "../../migrations/028_add_ls_sc_fba_order_addresses.sql",
			table: "LS_SC_FBA_ORDER_ADDRESSES",
			key:   "PRIMARY KEY (ACCOUNT_ID, SID, SHIPMENT_ID, SHIPMENT_ITEM_ID)",
		},
	}

	for _, tc := range tests {
		t.Run(tc.table, func(t *testing.T) {
			raw, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatalf("读取迁移失败: %v", err)
			}
			sql := strings.ToUpper(string(raw))
			if !strings.Contains(sql, "CREATE TABLE IF NOT EXISTS "+tc.table) {
				t.Fatalf("迁移未创建 %s", tc.table)
			}
			if !strings.Contains(sql, tc.key) {
				t.Fatalf("迁移缺少已验证业务键 %q", tc.key)
			}
			for _, destructive := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
				if strings.Contains(sql, destructive) {
					t.Fatalf("新增原始表迁移不得使用破坏性语句 %s", destructive)
				}
			}
		})
	}
}

func TestVCPODetailsMigrationPreservesObjectAndStoreScopedKey(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/030_add_ls_vc_po_details.sql")
	if err != nil {
		t.Fatalf("读取 VC PO detail 迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, want := range []string{
		"CREATE TABLE IF NOT EXISTS LS_VC_PO_DETAILS",
		"VC_STORE_ID",
		"LOCAL_PO_NUMBER",
		"PURCHASE_ORDER_NUMBER",
		"PURCHASE_ORDER_DATE",
		"PURCHASE_ORDER_STATE",
		"PAYMENT_METHOD",
		"TOTAL_PRICE",
		"CURRENCY_CODE",
		"ITEM_AMOUNT",
		"SHIP_WINDOW_START",
		"SHIP_WINDOW_END",
		"DELIVERY_WINDOW_START",
		"DELIVERY_WINDOW_END",
		"ITEMS JSON",
		"PRIMARY KEY (ACCOUNT_ID, VC_STORE_ID, LOCAL_PO_NUMBER)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("VC PO detail 迁移缺少 %q", want)
		}
	}
	for _, destructive := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE"} {
		if strings.Contains(sql, destructive) {
			t.Fatalf("新增 VC PO detail 原始表迁移不得使用破坏性语句 %s", destructive)
		}
	}
}

func TestVCOrdersStoreScopeMigrationIsEndpointSafe(t *testing.T) {
	raw, err := os.ReadFile("../../migrations/031_scope_ls_vc_orders_by_store.sql")
	if err != nil {
		t.Fatalf("读取 VC PO 列表店铺键迁移失败: %v", err)
	}
	sql := strings.ToUpper(string(raw))
	for _, forbidden := range []string{
		"__MIGRATION_031_BLOCKED",
		"DELETE FROM",
		"TRUNCATE",
	} {
		if strings.Contains(sql, forbidden) {
			t.Fatalf("迁移不得通过 %s 让全站退出或删除原始数据", forbidden)
		}
	}
	for _, want := range []string{
		"DATA_TYPE",
		"CHARACTER_MAXIMUM_LENGTH",
		"@VC_ORDERS_EMPTY_STORE_COUNT = 0",
		"PRIMARY KEY (ACCOUNT_ID, VC_STORE_ID, LOCAL_PO_NUMBER)",
	} {
		if !strings.Contains(sql, want) {
			t.Fatalf("迁移缺少安全门禁 %q", want)
		}
	}
}

func TestVCOrdersStoreScopeMigrationAgainstLocalMySQL(t *testing.T) {
	dsn := os.Getenv("LINGXING_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set LINGXING_MIGRATION_TEST_DSN to run the local VC PO migration integration test")
	}
	dbx, err := sqlx.Connect("mysql", dsn)
	if err != nil {
		t.Fatalf("连接迁移测试数据库失败: %v", err)
	}
	defer dbx.Close()

	const table = "ls_vc_orders_scope_031_test"
	defer func() { _, _ = dbx.Exec("DROP TABLE IF EXISTS " + table) }()
	raw, err := os.ReadFile("../../migrations/031_scope_ls_vc_orders_by_store.sql")
	if err != nil {
		t.Fatal(err)
	}
	migrationSQL := strings.ReplaceAll(string(raw), "ls_vc_orders", table)

	createOld := func(storeDefinition string) {
		t.Helper()
		if _, err := dbx.Exec("DROP TABLE IF EXISTS " + table); err != nil {
			t.Fatal(err)
		}
		stmt := "CREATE TABLE " + table + " (" +
			"account_id VARCHAR(64) NOT NULL," +
			"local_po_number VARCHAR(64) NOT NULL," +
			"vc_store_id " + storeDefinition + "," +
			"PRIMARY KEY (account_id, local_po_number))"
		if _, err := dbx.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("upgrades once and preserves two stores", func(t *testing.T) {
		createOld("VARCHAR(32) NULL")
		if _, err := dbx.Exec("INSERT INTO " + table + " VALUES ('a','po-1','store-1')"); err != nil {
			t.Fatal(err)
		}
		if _, err := dbx.Exec(migrationSQL); err != nil {
			t.Fatalf("首次迁移失败: %v", err)
		}
		if _, err := dbx.Exec(migrationSQL); err != nil {
			t.Fatalf("重复迁移失败: %v", err)
		}
		if err := ValidateVCOrdersStoreScope(dbx, table); err != nil {
			t.Fatalf("迁移后结构不合格: %v", err)
		}
		if _, err := dbx.Exec("INSERT INTO " + table + " VALUES ('a','po-1','store-2')"); err != nil {
			t.Fatalf("同账号同 PO 的第二店铺无法共存: %v", err)
		}
		var count int
		if err := dbx.Get(&count, "SELECT COUNT(*) FROM "+table+" WHERE account_id='a' AND local_po_number='po-1'"); err != nil {
			t.Fatal(err)
		}
		if count != 2 {
			t.Fatalf("同 PO 店铺行数=%d, want 2", count)
		}
	})

	t.Run("leaves null store schema untouched for endpoint failure", func(t *testing.T) {
		createOld("VARCHAR(32) NULL")
		if _, err := dbx.Exec("INSERT INTO " + table + " VALUES ('a','po-1',NULL)"); err != nil {
			t.Fatal(err)
		}
		if _, err := dbx.Exec(migrationSQL); err != nil {
			t.Fatalf("脏数据不得拖垮全站迁移: %v", err)
		}
		if err := ValidateVCOrdersStoreScope(dbx, table); err == nil {
			t.Fatal("空店铺必须让该 endpoint 不可同步")
		}
	})

	t.Run("rejects unexpected store type", func(t *testing.T) {
		createOld("VARCHAR(64) NULL")
		if _, err := dbx.Exec("INSERT INTO " + table + " VALUES ('a','po-1','store-1')"); err != nil {
			t.Fatal(err)
		}
		if _, err := dbx.Exec(migrationSQL); err != nil {
			t.Fatalf("漂移结构不得拖垮全站迁移: %v", err)
		}
		if err := ValidateVCOrdersStoreScope(dbx, table); err == nil {
			t.Fatal("漂移的 vc_store_id 类型必须让该 endpoint 不可同步")
		}
	})
}
