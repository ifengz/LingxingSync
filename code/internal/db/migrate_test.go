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
