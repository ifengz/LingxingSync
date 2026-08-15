-- The verified Reserved Inventory contract includes the FC transfer component.
SET @has_reserved_fc_transfers := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_fba_reserved_inventory'
      AND COLUMN_NAME = 'reserved_fc-transfers'
);
SET @sql := IF(@has_reserved_fc_transfers = 0,
    'ALTER TABLE ls_fba_reserved_inventory ADD COLUMN `reserved_fc-transfers` VARCHAR(32) NULL AFTER reserved_customerorders',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
