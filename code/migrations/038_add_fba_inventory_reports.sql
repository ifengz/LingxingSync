-- Formal Amazon inventory report evidence. Each report has its own table and
-- retains the exact upstream header names; it never updates API raw tables.

CREATE TABLE IF NOT EXISTS ls_fba_myi_unsuppressed_inventory (
    account_id                         VARCHAR(32)  NOT NULL,
    seller_id                          VARCHAR(64)  NOT NULL,
    store_id                           VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id                     VARCHAR(128) NOT NULL,
    `row_number`                       INT UNSIGNED NOT NULL,
    row_sha256                         CHAR(64)     NOT NULL,
    sku                                VARCHAR(256) NULL,
    fnsku                              VARCHAR(64)  NULL,
    asin                               VARCHAR(32)  NULL,
    `product-name`                     TEXT         NULL,
    `condition`                        VARCHAR(64)  NULL,
    `your-price`                       VARCHAR(64)  NULL,
    `mfn-listing-exists`               VARCHAR(32)  NULL,
    `mfn-fulfillable-quantity`         VARCHAR(32)  NULL,
    `afn-listing-exists`               VARCHAR(32)  NULL,
    `afn-warehouse-quantity`           VARCHAR(32)  NULL,
    `afn-fulfillable-quantity`         VARCHAR(32)  NULL,
    `afn-unsellable-quantity`          VARCHAR(32)  NULL,
    `afn-reserved-quantity`            VARCHAR(32)  NULL,
    `afn-total-quantity`               VARCHAR(32)  NULL,
    `per-unit-volume`                  VARCHAR(64)  NULL,
    `afn-inbound-working-quantity`     VARCHAR(32)  NULL,
    `afn-inbound-shipped-quantity`     VARCHAR(32)  NULL,
    `afn-inbound-receiving-quantity`   VARCHAR(32)  NULL,
    `afn-researching-quantity`         VARCHAR(32)  NULL,
    `afn-reserved-future-supply`       VARCHAR(32)  NULL,
    `afn-future-supply-buyable`        VARCHAR(32)  NULL,
    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_fba_myi_inventory_scope (account_id, seller_id, store_id),
    INDEX idx_fba_myi_inventory_asin (account_id, seller_id, store_id, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA Manage Inventory 正式报告原始行';

CREATE TABLE IF NOT EXISTS ls_fba_reserved_inventory (
    account_id                    VARCHAR(32)  NOT NULL,
    seller_id                     VARCHAR(64)  NOT NULL,
    store_id                      VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id                VARCHAR(128) NOT NULL,
    `row_number`                  INT UNSIGNED NOT NULL,
    row_sha256                    CHAR(64)     NOT NULL,
    sku                           VARCHAR(256) NULL,
    fnsku                         VARCHAR(64)  NULL,
    asin                          VARCHAR(32)  NULL,
    `product-name`                TEXT         NULL,
    reserved_qty                  VARCHAR(32)  NULL,
    reserved_customerorders       VARCHAR(32)  NULL,
    `reserved_fc-processing`       VARCHAR(32)  NULL,
    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_fba_reserved_inventory_scope (account_id, seller_id, store_id),
    INDEX idx_fba_reserved_inventory_asin (account_id, seller_id, store_id, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon FBA Reserved Inventory 正式报告原始行';

CREATE TABLE IF NOT EXISTS ls_afn_inventory (
    account_id                       VARCHAR(32)  NOT NULL,
    seller_id                        VARCHAR(64)  NOT NULL,
    store_id                         VARCHAR(64)  NOT NULL DEFAULT '',
    report_task_id                   VARCHAR(128) NOT NULL,
    `row_number`                     INT UNSIGNED NOT NULL,
    row_sha256                       CHAR(64)     NOT NULL,
    `seller-sku`                     VARCHAR(256) NULL,
    `fulfillment-channel-sku`        VARCHAR(256) NULL,
    asin                             VARCHAR(32)  NULL,
    `condition-type`                 VARCHAR(64)  NULL,
    `Warehouse-Condition-code`       VARCHAR(64)  NULL,
    `Quantity Available`             VARCHAR(32)  NULL,
    PRIMARY KEY (report_task_id, `row_number`),
    INDEX idx_afn_inventory_scope (account_id, seller_id, store_id),
    INDEX idx_afn_inventory_asin (account_id, seller_id, store_id, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT 'Amazon AFN Inventory 正式报告原始行';
