-- Formal Amazon FBA customer shipment replacements report evidence.
CREATE TABLE IF NOT EXISTS ls_fba_fulfillment_customer_shipment_replacements (
    account_id                         VARCHAR(32)  NOT NULL,
    seller_id                          VARCHAR(64)  NOT NULL,
    store_id                           VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id                     VARCHAR(128) NOT NULL,
    `row_number`                       INT UNSIGNED NOT NULL,
    row_sha256                         CHAR(64)     NOT NULL,
    `shipment-date`                    VARCHAR(40)  NULL,
    sku                                VARCHAR(256) NULL,
    asin                               VARCHAR(32)  NULL,
    `fulfillment-center-id`            VARCHAR(64)  NULL,
    `original-fulfillment-center-id`   VARCHAR(64)  NULL,
    quantity                           VARCHAR(32)  NULL,
    `replacement-reason-code`          VARCHAR(128) NULL,
    `replacement-amazon-order-id`      VARCHAR(128) NULL,
    `original-amazon-order-id`         VARCHAR(128) NULL,
    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_fba_replacements_scope (account_id, seller_id, store_id, `shipment-date`),
    INDEX idx_fba_replacements_order (account_id, seller_id, store_id, `original-amazon-order-id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA customer shipment replacements formal report raw rows';
