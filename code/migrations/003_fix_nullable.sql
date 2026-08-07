-- 修正：ls_stores.store_type 改为允许 NULL（领星 API 实际不返回此字段）
-- 已有 DEFAULT 'SC'，可 NULL 时 ON DUPLICATE KEY UPDATE 行为不变
-- MODIFY COLUMN 在 MySQL 8 下幂等（重复执行不报错）
ALTER TABLE ls_stores
  MODIFY COLUMN store_type VARCHAR(8) NULL DEFAULT 'SC' COMMENT 'SC / VC';
