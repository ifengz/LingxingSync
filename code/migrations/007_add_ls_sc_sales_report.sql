-- ls_sc_sales_report：亚马逊 ASIN/MSKU 日销量统计（asinDailyLists）
-- 接口：POST /erp/sc/data/sales_report/asinDailyLists（令牌桶容量 5）
-- 表名对齐运行配置 config.yaml 的 sc_sales_report.table = ls_sc_sales_report。
-- 宪法 §2：列名 = 领星「返回结果」字段名，polabel2 直读，不改名不猜。
--
-- ⚠️ map_value 是「请求参数 type 对应的统计量」——一次请求只返回一种指标
--    （销量 / 订单量 / 销售额之一，由 type 决定）。本表启用时 extra_params.type=2
--    故 map_value 语义为「销量（件数）」；同一路径同账号只能拉一种指标
--    （(quota_group, path) 限流键全局唯一，宪法 §5）。
CREATE TABLE IF NOT EXISTS ls_sc_sales_report (
    account_id    VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid           VARCHAR(32)   NOT NULL COMMENT '领星店铺编号',
    r_date        DATE          NOT NULL COMMENT '报表日期【站点时间】',
    seller_sku    VARCHAR(128)  NOT NULL COMMENT 'MSKU',
    asin          VARCHAR(16)   NOT NULL DEFAULT '' COMMENT 'ASIN（入主键，防同 MSKU 多 ASIN 互相覆盖）',
    product_name  VARCHAR(256)  NULL     COMMENT '品名',
    currency_code VARCHAR(8)    NULL     COMMENT '币种',
    map_value     DECIMAL(18,4) NOT NULL DEFAULT 0 COMMENT 'type 对应统计量：本表为销量（件数）',
    synced_at     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, r_date, seller_sku, asin),
    INDEX idx_asin_date (account_id, asin, r_date),
    INDEX idx_date      (account_id, r_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
