SET @all_orders_item_id_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_amazon_all_orders_by_order_date' AND COLUMN_NAME = 'order-item-id'
);
SET @all_orders_item_id_sql := IF(@all_orders_item_id_exists = 0,
    'ALTER TABLE ls_amazon_all_orders_by_order_date ADD COLUMN `order-item-id` VARCHAR(128) NULL AFTER `order-status`',
    'SELECT 1'
);
PREPARE all_orders_item_id_stmt FROM @all_orders_item_id_sql;
EXECUTE all_orders_item_id_stmt;
DEALLOCATE PREPARE all_orders_item_id_stmt;

SET @all_orders_cpf_exists := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_amazon_all_orders_by_order_date' AND COLUMN_NAME = 'cpf'
);
SET @all_orders_cpf_sql := IF(@all_orders_cpf_exists = 0,
    'ALTER TABLE ls_amazon_all_orders_by_order_date ADD COLUMN cpf VARCHAR(128) NULL AFTER `promotion-ids`',
    'SELECT 1'
);
PREPARE all_orders_cpf_stmt FROM @all_orders_cpf_sql;
EXECUTE all_orders_cpf_stmt;
DEALLOCATE PREPARE all_orders_cpf_stmt;
