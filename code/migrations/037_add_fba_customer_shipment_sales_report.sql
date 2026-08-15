-- Formal Amazon FBA customer shipment sales report evidence.
-- The report stays separate from API-originated sales tables.
CREATE TABLE IF NOT EXISTS ls_fba_fulfillment_customer_shipment_sales (
    account_id                VARCHAR(32)  NOT NULL,
    seller_id                 VARCHAR(64)  NOT NULL,
    store_id                  VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id            VARCHAR(128) NOT NULL,
    `row_number`              INT UNSIGNED NOT NULL,
    row_sha256                CHAR(64)     NOT NULL,

    `shipment-date`           VARCHAR(40)  NULL,
    sku                       VARCHAR(256) NULL,
    fnsku                     VARCHAR(64)  NULL,
    asin                      VARCHAR(32)  NULL,
    `fulfillment-center-id`   VARCHAR(64)  NULL,
    quantity                  VARCHAR(32)  NULL,
    `amazon-order-id`         VARCHAR(128) NULL,
    currency                  VARCHAR(16)  NULL,
    `item-price-per-unit`     VARCHAR(64)  NULL,
    `shipping-price`          VARCHAR(64)  NULL,
    `gift-wrap-price`         VARCHAR(64)  NULL,
    `ship-city`               VARCHAR(256) NULL,
    `ship-state`              VARCHAR(128) NULL,
    `ship-postal-code`        VARCHAR(64)  NULL,

    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_fba_shipment_sales_scope (account_id, seller_id, store_id, `shipment-date`),
    INDEX idx_fba_shipment_sales_asin (account_id, seller_id, store_id, asin, `shipment-date`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA Customer Shipment Sales 正式报告原始行';
