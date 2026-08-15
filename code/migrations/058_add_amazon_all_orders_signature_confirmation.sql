SET @all_orders_signature_confirmation_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_amazon_all_orders_by_order_date'
      AND COLUMN_NAME = 'signature-confirmation-recommended'
);
SET @all_orders_signature_confirmation_sql := IF(@all_orders_signature_confirmation_exists = 0,
    'ALTER TABLE ls_amazon_all_orders_by_order_date ADD COLUMN `signature-confirmation-recommended` VARCHAR(16) NULL AFTER `price-designation`',
    'SELECT 1'
);
PREPARE all_orders_signature_confirmation_stmt FROM @all_orders_signature_confirmation_sql;
EXECUTE all_orders_signature_confirmation_stmt;
DEALLOCATE PREPARE all_orders_signature_confirmation_stmt;
