-- SC ASIN 日销售额原始表。
--
-- 接口：POST /erp/sc/data/sales_report/asinDailyLists
-- 固定请求：type=1, asin_type=1, sid, event_date, offset, length。
-- 生产非空 probe task 1079 已确认返回字段只有：
--   sid, r_date, currency_code, asin, product_name, map_value
-- map_value 为销售额字符串，币种由 currency_code 单独返回；不与 type=2 销量表混存。
CREATE TABLE IF NOT EXISTS ls_sc_sales_revenue (
    account_id    VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid           VARCHAR(32)  NOT NULL COMMENT '领星店铺编号',
    r_date        VARCHAR(32)  NOT NULL COMMENT '报表日期【站点时间】，原样字符串',
    asin          VARCHAR(32)  NOT NULL COMMENT 'ASIN',
    product_name  VARCHAR(512) NULL     COMMENT '品名',
    currency_code VARCHAR(8)   NULL     COMMENT '币种',
    map_value     VARCHAR(32)  NULL     COMMENT 'type=1 销售额，原样字符串',
    synced_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, r_date, asin),
    INDEX idx_asin_date (account_id, asin, r_date),
    INDEX idx_date      (account_id, r_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC 亚马逊销售额 /erp/sc/data/sales_report/asinDailyLists type=1';
