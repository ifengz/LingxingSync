-- Amazon's EU FBA quantity variant appends local and remote fulfillable quantities.
SET @has_myi_local := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_unsuppressed_inventory'
      AND COLUMN_NAME = 'afn-fulfillable-quantity-local'
);
SET @sql := IF(@has_myi_local = 0,
    'ALTER TABLE ls_fba_myi_unsuppressed_inventory ADD COLUMN `afn-fulfillable-quantity-local` VARCHAR(32) NULL AFTER `afn-future-supply-buyable`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_myi_remote := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_unsuppressed_inventory'
      AND COLUMN_NAME = 'afn-fulfillable-quantity-remote'
);
SET @sql := IF(@has_myi_remote = 0,
    'ALTER TABLE ls_fba_myi_unsuppressed_inventory ADD COLUMN `afn-fulfillable-quantity-remote` VARCHAR(32) NULL AFTER `afn-fulfillable-quantity-local`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_myi_all_local := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_all_inventory'
      AND COLUMN_NAME = 'afn-fulfillable-quantity-local'
);
SET @sql := IF(@has_myi_all_local = 0,
    'ALTER TABLE ls_fba_myi_all_inventory ADD COLUMN `afn-fulfillable-quantity-local` VARCHAR(32) NULL AFTER `afn-future-supply-buyable`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_myi_all_remote := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_myi_all_inventory'
      AND COLUMN_NAME = 'afn-fulfillable-quantity-remote'
);
SET @sql := IF(@has_myi_all_remote = 0,
    'ALTER TABLE ls_fba_myi_all_inventory ADD COLUMN `afn-fulfillable-quantity-remote` VARCHAR(32) NULL AFTER `afn-fulfillable-quantity-local`',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
