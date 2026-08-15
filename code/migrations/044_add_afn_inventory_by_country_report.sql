-- Formal Amazon FBA inventory by country report evidence.
CREATE TABLE IF NOT EXISTS ls_afn_inventory_by_country (
    account_id                         VARCHAR(32)  NOT NULL,
    seller_id                          VARCHAR(64)  NOT NULL,
    store_id                           VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id                     VARCHAR(128) NOT NULL,
    `row_number`                       INT UNSIGNED NOT NULL,
    row_sha256                         CHAR(64)     NOT NULL,
    `seller-sku`                       VARCHAR(256) NULL,
    `fulfillment-channel-sku`          VARCHAR(256) NULL,
    asin                               VARCHAR(32)  NULL,
    `condition-type`                   VARCHAR(64)  NULL,
    country                            VARCHAR(16)  NULL,
    `quantity-for-local-fulfillment`   VARCHAR(32)  NULL,
    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_afn_inventory_country_scope (account_id, seller_id, store_id, country),
    INDEX idx_afn_inventory_country_asin (account_id, seller_id, store_id, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA inventory by country formal report raw rows';
