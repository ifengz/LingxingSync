-- Preserve the production Removal Order field observed in audit 67.
-- Keep the historical service-speed column and existing rows unchanged.
SET @removal_order_source_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_fba_removal_order_details' AND COLUMN_NAME = 'order-source'
);
SET @removal_order_source_sql := IF(@removal_order_source_exists = 0,
    'ALTER TABLE ls_fba_removal_order_details ADD COLUMN `order-source` VARCHAR(64) NULL AFTER `order-id`',
    'DO 0');
PREPARE removal_order_source_stmt FROM @removal_order_source_sql;
EXECUTE removal_order_source_stmt;
DEALLOCATE PREPARE removal_order_source_stmt;
