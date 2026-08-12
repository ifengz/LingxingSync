-- Formal Amazon FBA customer returns report evidence.
--
-- The report lane is independent from API-originated ls_* tables.  The audit
-- row records the asynchronous task and download lifecycle; the raw table
-- keeps each accepted TSV row with its report task and content hash.  No
-- report row is used to overwrite an API raw table.

CREATE TABLE IF NOT EXISTS ls_report_export_tasks (
    id                    BIGINT AUTO_INCREMENT PRIMARY KEY,
    account_id            VARCHAR(32)  NOT NULL,
    seller_id             VARCHAR(64)  NOT NULL,
    store_id              VARCHAR(64)  NOT NULL DEFAULT '',
    report_type           VARCHAR(128) NOT NULL,
    region                VARCHAR(8)   NOT NULL DEFAULT '',
    marketplace_ids       JSON         NULL,
    date_from             VARCHAR(32)  NOT NULL DEFAULT '',
    date_to               VARCHAR(32)  NOT NULL DEFAULT '',
    report_task_id        VARCHAR(128) NOT NULL DEFAULT '',
    report_document_id    VARCHAR(256) NULL,
    status                VARCHAR(32)  NOT NULL DEFAULT 'PENDING',
    compression_algorithm VARCHAR(32)  NULL,
    download_url          VARCHAR(2048) NULL,
    download_sha256       CHAR(64)     NULL,
    downloaded_at         DATETIME     NULL,
    rows_imported         INT          NOT NULL DEFAULT 0,
    error_message         TEXT         NULL,
    active_scope_key       CHAR(64)     NULL,
    created_at             DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at             DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_report_export_scope (account_id, seller_id, store_id, report_type, date_from, date_to),
    INDEX idx_report_export_status (status, created_at),
    INDEX idx_report_export_task (report_task_id),
    UNIQUE KEY uq_report_export_active_scope (active_scope_key)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '正式 Amazon 报表异步任务审计';

-- 032 may already exist in a local database from the first report rollout.
-- Add the active-scope columns/index independently so re-running startup
-- migrations upgrades that schema without dropping audit evidence.
SET @ls_report_region_exists := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_report_export_tasks' AND COLUMN_NAME = 'region');
SET @ls_report_region_sql := IF(@ls_report_region_exists = 0, 'ALTER TABLE ls_report_export_tasks ADD COLUMN region VARCHAR(8) NULL AFTER report_type', 'SELECT 1');
PREPARE ls_report_region_stmt FROM @ls_report_region_sql;
EXECUTE ls_report_region_stmt;
DEALLOCATE PREPARE ls_report_region_stmt;

SET @ls_report_marketplaces_exists := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_report_export_tasks' AND COLUMN_NAME = 'marketplace_ids');
SET @ls_report_marketplaces_sql := IF(@ls_report_marketplaces_exists = 0, 'ALTER TABLE ls_report_export_tasks ADD COLUMN marketplace_ids JSON NULL AFTER region', 'SELECT 1');
PREPARE ls_report_marketplaces_stmt FROM @ls_report_marketplaces_sql;
EXECUTE ls_report_marketplaces_stmt;
DEALLOCATE PREPARE ls_report_marketplaces_stmt;

SET @ls_report_active_key_exists := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_report_export_tasks' AND COLUMN_NAME = 'active_scope_key');
SET @ls_report_active_key_sql := IF(@ls_report_active_key_exists = 0, 'ALTER TABLE ls_report_export_tasks ADD COLUMN active_scope_key CHAR(64) NULL AFTER error_message', 'SELECT 1');
PREPARE ls_report_active_key_stmt FROM @ls_report_active_key_sql;
EXECUTE ls_report_active_key_stmt;
DEALLOCATE PREPARE ls_report_active_key_stmt;

SET @ls_report_active_index_exists := (SELECT COUNT(*) FROM INFORMATION_SCHEMA.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'ls_report_export_tasks' AND INDEX_NAME = 'uq_report_export_active_scope');
SET @ls_report_active_index_sql := IF(@ls_report_active_index_exists = 0, 'ALTER TABLE ls_report_export_tasks ADD UNIQUE KEY uq_report_export_active_scope (active_scope_key)', 'SELECT 1');
PREPARE ls_report_active_index_stmt FROM @ls_report_active_index_sql;
EXECUTE ls_report_active_index_stmt;
DEALLOCATE PREPARE ls_report_active_index_stmt;

CREATE TABLE IF NOT EXISTS ls_fba_fulfillment_customer_returns (
    account_id                VARCHAR(32)  NOT NULL,
    seller_id                 VARCHAR(64)  NOT NULL,
    store_id                  VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id            VARCHAR(128) NOT NULL,
    `row_number`              INT UNSIGNED NOT NULL,
    row_sha256                CHAR(64)     NOT NULL,

    `return-date`             VARCHAR(40)  NULL,
    `order-id`                VARCHAR(128) NULL,
    `sku`                     VARCHAR(256) NULL,
    `asin`                    VARCHAR(32)  NULL,
    `fnsku`                   VARCHAR(64)  NULL,
    `product-name`            TEXT         NULL,
    `quantity`                VARCHAR(32)  NULL,
    `fulfillment-center-id`   VARCHAR(64)  NULL,
    `detailed-disposition`    VARCHAR(128) NULL,
    `reason`                  VARCHAR(256) NULL,
    `status`                  VARCHAR(64)  NULL,
    `license-plate-number`    VARCHAR(128) NULL,
    `customer-comments`       TEXT         NULL,

    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_fba_returns_scope (account_id, seller_id, store_id, `return-date`),
    INDEX idx_fba_returns_order (account_id, seller_id, `order-id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA Customer Returns 正式报告原始行';
