-- SC MSKU 日销量/销售额原始表。
--
-- 接口：POST /erp/sc/data/sales_report/asinDailyLists
-- 固定请求：asin_type=2；type=2 为销量，type=1 为销售额。
-- MSKU 响应以 seller_sku 标识 listing，不能再从 ASIN 猜测 SKU。
-- 旧 ASIN 粒度表保留为历史原始证据，不迁移、不删除。

CREATE TABLE IF NOT EXISTS ls_sc_sales_report_msku (
    account_id    VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid           VARCHAR(32)  NOT NULL COMMENT '领星店铺编号',
    r_date        VARCHAR(32)  NOT NULL COMMENT '报表日期【站点时间】，原样字符串',
    asin          VARCHAR(32)  NOT NULL COMMENT 'ASIN',
    seller_sku    VARCHAR(255) NOT NULL COMMENT 'MSKU，领星返回字段原名',
    product_name  VARCHAR(512) NULL     COMMENT '品名',
    currency_code VARCHAR(8)   NULL     COMMENT '币种',
    map_value     VARCHAR(32)  NULL     COMMENT 'type=2 销量，原样字符串',
    synced_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, r_date, asin, seller_sku),
    INDEX idx_asin_date (account_id, asin, r_date),
    INDEX idx_synced_at (synced_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC MSKU 销量 /erp/sc/data/sales_report/asinDailyLists type=2 asin_type=2';

CREATE TABLE IF NOT EXISTS ls_sc_sales_revenue_msku (
    account_id    VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid           VARCHAR(32)  NOT NULL COMMENT '领星店铺编号',
    r_date        VARCHAR(32)  NOT NULL COMMENT '报表日期【站点时间】，原样字符串',
    asin          VARCHAR(32)  NOT NULL COMMENT 'ASIN',
    seller_sku    VARCHAR(255) NOT NULL COMMENT 'MSKU，领星返回字段原名',
    product_name  VARCHAR(512) NULL     COMMENT '品名',
    currency_code VARCHAR(8)   NULL     COMMENT '币种',
    map_value     VARCHAR(32)  NULL     COMMENT 'type=1 销售额，原样字符串',
    synced_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, r_date, asin, seller_sku),
    INDEX idx_asin_date (account_id, asin, r_date),
    INDEX idx_synced_at (synced_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC MSKU 销售额 /erp/sc/data/sales_report/asinDailyLists type=1 asin_type=2';
