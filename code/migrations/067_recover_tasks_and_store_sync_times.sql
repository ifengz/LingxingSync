-- 兼容已部署旧系统表，允许把空结果标记为需核验的 empty。
ALTER TABLE sync_tasks
    MODIFY COLUMN status ENUM('pending','running','success','empty','error','cancelled')
    NOT NULL DEFAULT 'pending';

SET @has_store_attempt_at := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_stores' AND COLUMN_NAME = 'last_attempt_at'
);
SET @has_store_success_at := (
    SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_stores' AND COLUMN_NAME = 'last_success_at'
);
SET @store_sync_time_sql := CASE
    WHEN @has_store_attempt_at = 0 AND @has_store_success_at = 0
        THEN 'ALTER TABLE ls_stores ADD COLUMN last_attempt_at DATETIME NULL, ADD COLUMN last_success_at DATETIME NULL'
    WHEN @has_store_attempt_at = 0
        THEN 'ALTER TABLE ls_stores ADD COLUMN last_attempt_at DATETIME NULL'
    WHEN @has_store_success_at = 0
        THEN 'ALTER TABLE ls_stores ADD COLUMN last_success_at DATETIME NULL'
    ELSE 'DO 0'
END;
PREPARE store_sync_time_stmt FROM @store_sync_time_sql;
EXECUTE store_sync_time_stmt;
DEALLOCATE PREPARE store_sync_time_stmt;
