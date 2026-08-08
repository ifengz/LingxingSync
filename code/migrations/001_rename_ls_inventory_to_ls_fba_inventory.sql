-- 将早期 SC FBA 库存表迁移到明确的渠道命名。
-- 迁移必须排在 002 建表之前：否则 002 会在旧表重命名后重新创建旧表。
-- 旧表存在且新表不存在时只改表名，不复制、不删除数据；其他情况保持幂等。
SET @ls_inventory_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_inventory'
);
SET @ls_fba_inventory_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.TABLES
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_inventory'
);
SET @ls_fba_rename_sql := IF(
    @ls_inventory_exists = 1 AND @ls_fba_inventory_exists = 0,
    'RENAME TABLE ls_inventory TO ls_fba_inventory',
    'DO 0'
);
PREPARE ls_fba_rename_stmt FROM @ls_fba_rename_sql;
EXECUTE ls_fba_rename_stmt;
DEALLOCATE PREPARE ls_fba_rename_stmt;
