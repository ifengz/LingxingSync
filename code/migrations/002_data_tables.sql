-- 数据表（宪法 doc/02-database.md §2/§3）
-- 每个 ls_* 表一张结构化表，列名 = 领星字段名，polabel2 直读。
-- record_id_fields（config 数组）→ PRIMARY KEY (account_id, col1, col2, ...)。

-- ls_stores：店铺列表（SC 店铺来源接口 is_store_source=true 写入；
-- 其他接口 iterate_by_store 从这里读 sid）。宪法 §10。
CREATE TABLE IF NOT EXISTS ls_stores (
    account_id   VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid          VARCHAR(32)  NOT NULL COMMENT '领星店铺编号',
    store_type   VARCHAR(8)   NULL     DEFAULT 'SC' COMMENT 'SC / VC',
    store_name   VARCHAR(128) NULL,
    seller_id    VARCHAR(64)  NULL,
    marketplace_id VARCHAR(16) NULL,
    country      VARCHAR(32)  NULL,
    has_ads_setting TINYINT   NOT NULL DEFAULT 0 COMMENT '1=已做广告授权',
    status       VARCHAR(32)  NULL,
    gmt_modified DATETIME     NULL,
    synced_at    DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid),
    INDEX idx_marketplace (account_id, marketplace_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ls_sales_orders：FBA/FBM 销售订单（宪法 §3）
CREATE TABLE IF NOT EXISTS ls_sales_orders (
    order_id          VARCHAR(64)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    order_status      VARCHAR(32)    NULL,
    order_type        VARCHAR(16)    NULL COMMENT 'FBA / FBM',
    store_id          VARCHAR(32)    NULL,
    store_name        VARCHAR(128)   NULL,
    asin              VARCHAR(16)    NULL,
    sku               VARCHAR(128)   NULL,
    listing_sku       VARCHAR(128)   NULL,
    quantity          INT            NOT NULL DEFAULT 0,
    amount            DECIMAL(14,4)  NOT NULL DEFAULT 0,
    currency          VARCHAR(8)     NULL,
    sales_channel     VARCHAR(64)    NULL,
    purchase_date     DATETIME       NULL,
    fulfillment_date  DATETIME       NULL,
    marketplace_id    VARCHAR(16)    NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, order_id),
    INDEX idx_store_date     (account_id, store_id, purchase_date),
    INDEX idx_sku_date       (account_id, sku, purchase_date),
    INDEX idx_gmt_modified   (account_id, gmt_modified)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ls_inventory：FBA 库存（宪法 §3）
CREATE TABLE IF NOT EXISTS ls_inventory (
    fnsku             VARCHAR(32)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    asin              VARCHAR(16)    NULL,
    sku               VARCHAR(128)   NULL,
    store_id          VARCHAR(32)    NULL,
    store_name        VARCHAR(128)   NULL,
    product_name      VARCHAR(256)   NULL,
    quantity          INT            NOT NULL DEFAULT 0,
    reserved_quantity INT            NOT NULL DEFAULT 0,
    inbound_quantity  INT            NOT NULL DEFAULT 0,
    warehouse         VARCHAR(64)    NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, fnsku),
    INDEX idx_store   (account_id, store_id),
    INDEX idx_sku     (account_id, sku)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- ls_ads_daily：广告日报（宪法 §3）
CREATE TABLE IF NOT EXISTS ls_ads_daily (
    report_id         VARCHAR(64)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    store_id          VARCHAR(32)    NULL,
    profile_id        VARCHAR(32)    NULL,
    campaign_id       VARCHAR(32)    NULL,
    campaign_name     VARCHAR(256)   NULL,
    ad_group_id       VARCHAR(32)    NULL,
    ad_group_name     VARCHAR(128)   NULL,
    asin              VARCHAR(16)    NULL,
    sku               VARCHAR(128)   NULL,
    report_date       DATE           NOT NULL,
    impressions       INT            NOT NULL DEFAULT 0,
    clicks            INT            NOT NULL DEFAULT 0,
    spend             DECIMAL(12,4)  NOT NULL DEFAULT 0,
    sales             DECIMAL(12,4)  NOT NULL DEFAULT 0,
    orders            INT            NOT NULL DEFAULT 0,
    currency          VARCHAR(8)     NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, report_id),
    INDEX idx_store_date  (account_id, store_id, report_date),
    INDEX idx_campaign    (account_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
