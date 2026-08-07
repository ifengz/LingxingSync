-- 系统表（宪法 doc/02-database.md §1）
-- 由框架维护；外部（polabel2 等）只读账号不暴露这两张表。

-- sync_tasks：同步任务状态，每次触发一行
CREATE TABLE IF NOT EXISTS sync_tasks (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    endpoint     VARCHAR(64)  NOT NULL COMMENT '接口标识，如 sales_orders',
    account_id   VARCHAR(32)  NOT NULL COMMENT '领星账号 ID',
    status       ENUM('pending','running','success','error','cancelled')
                 NOT NULL DEFAULT 'pending',
    trigger_type ENUM('cron','manual') NOT NULL DEFAULT 'cron',
    started_at   DATETIME     NULL,
    finished_at  DATETIME     NULL,
    records_upserted INT       NOT NULL DEFAULT 0,
    pages_fetched    INT       NOT NULL DEFAULT 0,
    error_message    TEXT      NULL      COMMENT '失败原因，保留完整原始错误',
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_endpoint_status  (endpoint, status),
    INDEX idx_account_created  (account_id, created_at),
    INDEX idx_status_created   (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- sync_task_logs：每页请求的证据，不丢
CREATE TABLE IF NOT EXISTS sync_task_logs (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id      BIGINT       NOT NULL,
    page         INT          NOT NULL,
    http_status  SMALLINT     NULL COMMENT 'HTTP 状态码',
    api_code     INT          NULL COMMENT '领星 API code 字段',
    records_count INT         NOT NULL DEFAULT 0,
    error_raw    TEXT         NULL COMMENT '原始错误消息（不截断）',
    duration_ms  INT          NOT NULL DEFAULT 0,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
