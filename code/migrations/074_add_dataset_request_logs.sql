-- 074_add_dataset_request_logs.sql
-- dataset_request_logs：下游项目经数据集 API 拉数的请求日志（系统表，非 ls_* 数据表）。
--
-- 动机：下游「什么时候拉了什么数据集、拉到哪、拉了多少行、成功还是被拒」此前只有
-- stdout 访问行（进程重启即失），没有任何可查证据。本表让下游读取与上游 sync_tasks
-- 对等：一请求一行，由 datasetapi.Handler 出口单点写入。
--
-- 写入方：internal/datasetapi handler（经 Config.RequestLogger 钩子注入 db 写入），
--   与 PersistFields 同款模式；不传钩子（测试环境）则不落库。
-- 留存：随 retention cron 与 sync_task_logs 一起清理（db.CleanupOld）。
CREATE TABLE IF NOT EXISTS dataset_request_logs (
    id            BIGINT       NOT NULL AUTO_INCREMENT,
    dataset_id    VARCHAR(64)  NOT NULL COMMENT '数据集 ID，如 listing-daily-v1',
    endpoint      VARCHAR(16)  NOT NULL COMMENT 'snapshot | changes | fields',
    project_id    VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '下游项目 ID，认证失败时为空',
    token_id      VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '命中的 token ID，认证失败时为空',
    store         VARCHAR(64)  NOT NULL DEFAULT '' COMMENT '请求的店铺，未传为空',
    date_from     VARCHAR(10)  NOT NULL DEFAULT '' COMMENT '请求开始日，changes/fields 请求为空',
    date_to       VARCHAR(10)  NOT NULL DEFAULT '' COMMENT '请求结束日，changes/fields 请求为空',
    status_code   INT          NOT NULL COMMENT 'HTTP 状态码（200/400/401/403/500）',
    rows_returned INT          NOT NULL DEFAULT 0 COMMENT '本次响应行数（失败为 0）',
    duration_ms   INT          NOT NULL DEFAULT 0 COMMENT 'handler 内处理耗时',
    error_message VARCHAR(512) NOT NULL DEFAULT '' COMMENT '失败原因摘要，成功为空',
    created_at    DATETIME(3)  NOT NULL DEFAULT CURRENT_TIMESTAMP(3) COMMENT '请求落定时间',
    PRIMARY KEY (id),
    KEY idx_dataset_request_logs_created_at (created_at),
    KEY idx_dataset_request_logs_project (project_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
