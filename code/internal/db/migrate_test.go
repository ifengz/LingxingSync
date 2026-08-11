package db

import (
	"os"
	"strings"
	"testing"
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
