-- retention 每日按 created_at 删除旧页日志；只在没有 created_at 前导索引时补齐。
SET @sync_task_logs_created_at_index_exists := (
    SELECT COUNT(*)
    FROM INFORMATION_SCHEMA.STATISTICS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'sync_task_logs'
      AND COLUMN_NAME = 'created_at'
      AND SEQ_IN_INDEX = 1
);
SET @sync_task_logs_created_at_index_sql := IF(
    @sync_task_logs_created_at_index_exists = 0,
    'ALTER TABLE sync_task_logs ADD INDEX idx_created_at (created_at)',
    'DO 0'
);
PREPARE sync_task_logs_created_at_index_stmt FROM @sync_task_logs_created_at_index_sql;
EXECUTE sync_task_logs_created_at_index_stmt;
DEALLOCATE PREPARE sync_task_logs_created_at_index_stmt;
