-- Formal Amazon FBA overage fee charges report evidence.
CREATE TABLE IF NOT EXISTS ls_fba_overage_fee_charges (
    account_id                         VARCHAR(32)  NOT NULL,
    seller_id                          VARCHAR(64)  NOT NULL,
    store_id                           VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id                     VARCHAR(128) NOT NULL,
    `row_number`                       INT UNSIGNED NOT NULL,
    row_sha256                         CHAR(64)     NOT NULL,
    charged_date                       VARCHAR(40)  NULL,
    country_code                       VARCHAR(16)  NULL,
    storage_type                       VARCHAR(64)  NULL,
    charge_rate                        VARCHAR(64)  NULL,
    storage_usage_volume               VARCHAR(64)  NULL,
    storage_limit_volume               VARCHAR(64)  NULL,
    overage_volume                     VARCHAR(64)  NULL,
    volume_unit                        VARCHAR(32)  NULL,
    charged_fee_amount                 VARCHAR(64)  NULL,
    currency_code                      VARCHAR(16)  NULL,
    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_fba_overage_fee_scope (account_id, seller_id, store_id, charged_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA overage fee charges formal report raw rows';
