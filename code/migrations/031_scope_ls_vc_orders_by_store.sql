-- VC PO 列表按账号、店铺、PO 号隔离。
-- vc_store_id 来自 pageList 真实响应；空店铺或未知结构必须停下人工核对，禁止猜值。

SET @vc_orders_primary_key := (
    SELECT GROUP_CONCAT(COLUMN_NAME ORDER BY SEQ_IN_INDEX SEPARATOR ',')
    FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_vc_orders'
      AND INDEX_NAME = 'PRIMARY'
);

SET @vc_orders_store_nullable := (
    SELECT IS_NULLABLE
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_vc_orders'
      AND COLUMN_NAME = 'vc_store_id'
);

SET @vc_orders_store_data_type := (
    SELECT DATA_TYPE
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_vc_orders'
      AND COLUMN_NAME = 'vc_store_id'
);

SET @vc_orders_store_max_length := (
    SELECT CHARACTER_MAXIMUM_LENGTH
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_vc_orders'
      AND COLUMN_NAME = 'vc_store_id'
);

SET @vc_orders_empty_store_count := (
    SELECT COUNT(*)
    FROM ls_vc_orders
    WHERE vc_store_id IS NULL OR TRIM(vc_store_id) = ''
);

SET @vc_orders_scope_sql := CASE
    WHEN @vc_orders_primary_key = 'account_id,vc_store_id,local_po_number'
         AND @vc_orders_store_nullable = 'NO'
         AND @vc_orders_store_data_type = 'varchar'
         AND @vc_orders_store_max_length = 32
         AND @vc_orders_empty_store_count = 0
        THEN 'DO 0'
    WHEN @vc_orders_primary_key = 'account_id,local_po_number'
         AND @vc_orders_store_nullable = 'YES'
         AND @vc_orders_store_data_type = 'varchar'
         AND @vc_orders_store_max_length = 32
         AND @vc_orders_empty_store_count = 0
        THEN 'ALTER TABLE ls_vc_orders MODIFY vc_store_id VARCHAR(32) NOT NULL COMMENT ''VC 店铺 id'', DROP PRIMARY KEY, ADD PRIMARY KEY (account_id, vc_store_id, local_po_number)'
    ELSE 'DO 0'
END;

PREPARE vc_orders_scope_stmt FROM @vc_orders_scope_sql;
EXECUTE vc_orders_scope_stmt;
DEALLOCATE PREPARE vc_orders_scope_stmt;
