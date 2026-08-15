-- Preserve the production MYI columns observed in the formal report download.
-- This migration only adds nullable columns and never rewrites existing rows.
SET @has_myi_fc_transfer := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_unsuppressed_inventory'
      AND COLUMN_NAME = 'afn-fc-transfer-quantity'
);
SET @sql := IF(@has_myi_fc_transfer = 0,
    'ALTER TABLE ls_fba_myi_unsuppressed_inventory ADD COLUMN `afn-fc-transfer-quantity` VARCHAR(32) NULL AFTER `afn-future-supply-buyable`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_myi_onhand_buyable := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_unsuppressed_inventory'
      AND COLUMN_NAME = 'afn-onhand-buyable-quantity'
);
SET @sql := IF(@has_myi_onhand_buyable = 0,
    'ALTER TABLE ls_fba_myi_unsuppressed_inventory ADD COLUMN `afn-onhand-buyable-quantity` VARCHAR(32) NULL AFTER `afn-fc-transfer-quantity`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_myi_store := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_unsuppressed_inventory'
      AND COLUMN_NAME = 'store'
);
SET @sql := IF(@has_myi_store = 0,
    'ALTER TABLE ls_fba_myi_unsuppressed_inventory ADD COLUMN `store` VARCHAR(128) NULL AFTER `afn-onhand-buyable-quantity`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_all_fc_transfer := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_all_inventory'
      AND COLUMN_NAME = 'afn-fc-transfer-quantity'
);
SET @sql := IF(@has_all_fc_transfer = 0,
    'ALTER TABLE ls_fba_myi_all_inventory ADD COLUMN `afn-fc-transfer-quantity` VARCHAR(32) NULL AFTER `afn-future-supply-buyable`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_all_onhand_buyable := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_all_inventory'
      AND COLUMN_NAME = 'afn-onhand-buyable-quantity'
);
SET @sql := IF(@has_all_onhand_buyable = 0,
    'ALTER TABLE ls_fba_myi_all_inventory ADD COLUMN `afn-onhand-buyable-quantity` VARCHAR(32) NULL AFTER `afn-fc-transfer-quantity`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_all_store := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_all_inventory'
      AND COLUMN_NAME = 'store'
);
SET @sql := IF(@has_all_store = 0,
    'ALTER TABLE ls_fba_myi_all_inventory ADD COLUMN `store` VARCHAR(128) NULL AFTER `afn-onhand-buyable-quantity`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
