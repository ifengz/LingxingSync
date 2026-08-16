SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'amazon-store'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `amazon-store` VARCHAR(128) NULL AFTER `fulfilled-by`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'product-size-tier'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `product-size-tier` VARCHAR(128) NULL AFTER `product-size-weight-band`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-order-handling-fee-per-order'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-order-handling-fee-per-order` VARCHAR(64) NULL AFTER `estimated-variable-closing-fee`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-pick-pack-fee-per-unit'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-pick-pack-fee-per-unit` VARCHAR(64) NULL AFTER `estimated-order-handling-fee-per-order`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-weight-handling-fee-per-unit'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-weight-handling-fee-per-unit` VARCHAR(64) NULL AFTER `estimated-pick-pack-fee-per-unit`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'expected-fulfillment-fee-per-unit'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `expected-fulfillment-fee-per-unit` VARCHAR(64) NULL AFTER `estimated-weight-handling-fee-per-unit`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-future-fee'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-future-fee` VARCHAR(64) NULL AFTER `expected-fulfillment-fee-per-unit`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-future-order-handling-fee-per-order'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-future-order-handling-fee-per-order` VARCHAR(64) NULL AFTER `estimated-future-fee`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-future-pick-pack-fee-per-unit'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-future-pick-pack-fee-per-unit` VARCHAR(64) NULL AFTER `estimated-future-order-handling-fee-per-order`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'estimated-future-weight-handling-fee-per-unit'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `estimated-future-weight-handling-fee-per-unit` VARCHAR(64) NULL AFTER `estimated-future-pick-pack-fee-per-unit`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;

SET @estimated_fees_column_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_estimated_fees' AND COLUMN_NAME = 'expected-future-fulfillment-fee-per-unit'
);
SET @estimated_fees_sql := IF(@estimated_fees_column_exists = 0,
    'ALTER TABLE ls_fba_estimated_fees ADD COLUMN `expected-future-fulfillment-fee-per-unit` VARCHAR(64) NULL AFTER `estimated-future-weight-handling-fee-per-unit`',
    'DO 0');
PREPARE estimated_fees_stmt FROM @estimated_fees_sql;
EXECUTE estimated_fees_stmt;
DEALLOCATE PREPARE estimated_fees_stmt;
