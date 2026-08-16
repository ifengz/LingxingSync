-- Preserve the production NA storage fee columns observed in audit 58.
-- This migration only adds nullable columns and never rewrites existing rows.
SET @has_storage_fee_sku := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'sku'
);
SET @sql := IF(@has_storage_fee_sku = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN sku VARCHAR(128) NULL AFTER average_quantity_customer_orders',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_storage_utilization_ratio := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'storage_utilization_ratio'
);
SET @sql := IF(@has_storage_utilization_ratio = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN storage_utilization_ratio VARCHAR(64) NULL AFTER sku',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_storage_utilization_ratio_units := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'storage_utilization_ratio_units'
);
SET @sql := IF(@has_storage_utilization_ratio_units = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN storage_utilization_ratio_units VARCHAR(64) NULL AFTER storage_utilization_ratio',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_storage_fee_base_rate := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'base_rate'
);
SET @sql := IF(@has_storage_fee_base_rate = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN base_rate VARCHAR(64) NULL AFTER storage_utilization_ratio_units',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_utilization_surcharge_rate := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'utilization_surcharge_rate'
);
SET @sql := IF(@has_utilization_surcharge_rate = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN utilization_surcharge_rate VARCHAR(64) NULL AFTER base_rate',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_avg_qty_for_sus := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'avg_qty_for_sus'
);
SET @sql := IF(@has_avg_qty_for_sus = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN avg_qty_for_sus VARCHAR(64) NULL AFTER utilization_surcharge_rate',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_est_vol_for_sus := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'est_vol_for_sus'
);
SET @sql := IF(@has_est_vol_for_sus = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN est_vol_for_sus VARCHAR(64) NULL AFTER avg_qty_for_sus',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_est_base_msf := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'est_base_msf'
);
SET @sql := IF(@has_est_base_msf = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN est_base_msf VARCHAR(64) NULL AFTER est_vol_for_sus',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;

SET @has_est_sus := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_storage_fee_charges' AND COLUMN_NAME = 'est_sus'
);
SET @sql := IF(@has_est_sus = 0,
    'ALTER TABLE ls_fba_storage_fee_charges ADD COLUMN est_sus VARCHAR(64) NULL AFTER est_base_msf',
    'DO 0');
PREPARE stmt FROM @sql; EXECUTE stmt; DEALLOCATE PREPARE stmt;
