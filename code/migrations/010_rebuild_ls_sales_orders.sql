-- 重建 ls_sales_orders：列名一字不差对齐领星 POST /erp/sc/data/mws/orders 返回字段。
--
-- 为什么要重建：旧表是 polabel2 风格的「翻译列名」（order_id / sku / amount /
-- quantity / store_name / currency / purchase_date …），领星这个接口一个都不返回。
-- 通用 Upsert 按「列名 = 领星字段名」匹配，旧表结果是：主键列 order_id 永远取不到值，
-- NOT NULL 主键写 NULL 直接报错，整个接口永远落不了库。
-- 违反 CLAUDE.md §1.6「表结构 = 领星字段名，不翻译」。
--
-- 探测来源：probe 模式真实响应（sc_us_1，200 行样本，39 个字段）。
-- 原则同 006：领星返回什么就存什么，不改名、不拆分、不转换。
--
-- 日期字段为什么全是 VARCHAR 而不是 DATETIME（实测结论，勿改回）：
--   1. 领星对无值日期返回空字符串 ""（如 posted_date），
--      STRICT_TRANS_TABLES 下写 DATETIME 报 1292 Incorrect datetime value，整批 fail。
--   2. 同一响应里日期有三种格式并存：
--        purchase_date        2026-08-01T14:11:11Z        ← 带 T/Z，MySQL 拒收
--        purchase_date_utc    2026-08-01 14:11:11         ← 标准格式
--        shipment_date        2026-08-03T00:03:30+00:00   ← 带时区偏移
--      带 T/Z 的那种同样报 1292。
--   存 VARCHAR 保真透传，格式转换交给消费方（polabel2 直读时 STR_TO_DATE / CAST）。
--
-- 主键：(account_id, amazon_order_id) —— amazon_order_id 是领星返回的亚马逊订单号，
-- 一个订单在本接口只有一行（商品明细在 item_list 里，不拆行）。
--
-- ⚠️ PII 列：address / buyer_email / buyer_name / phone / name 是买家个人信息。
-- 按宪法「领星返回什么就存什么」保留建列；本次探测样本这 5 列全为空串
-- （该领星账号未开放买家信息），如确认业务不需要，删掉这 5 行建表语句即可 ——
-- 通用 Upsert 只写表里存在的列，删列后这些字段自动丢弃，无需改任何代码。

-- 幂等守卫（重要，勿简化成裸 DROP TABLE）：
-- migrate.go 每次进程启动都把 migrations/ 下所有 .sql 全量重跑（不维护 schema_versions）。
-- 裸 `DROP TABLE` 会导致「每次重启清空这张表」——而宪法 §4.6 规定结构性改配置就要
-- 重启进程，等于正常运维动作就会丢数据。
-- 因此只在「表还是旧结构（没有 amazon_order_id 列）」时才 DROP；已是新结构则空转。
SET @ls_sales_orders_is_old := (
    SELECT COUNT(*) = 0
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_sales_orders'
      AND COLUMN_NAME = 'amazon_order_id'
);
SET @ls_sales_orders_sql := IF(@ls_sales_orders_is_old,
    'DROP TABLE IF EXISTS ls_sales_orders',
    'DO 0');
PREPARE ls_so_stmt FROM @ls_sales_orders_sql;
EXECUTE ls_so_stmt;
DEALLOCATE PREPARE ls_so_stmt;

CREATE TABLE IF NOT EXISTS ls_sales_orders (
    account_id                  VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID（非领星字段）',

    -- ---- 订单标识 / 店铺 ----
    amazon_order_id             VARCHAR(64)   NOT NULL COMMENT '亚马逊订单号',
    sid                         VARCHAR(32)   NULL COMMENT '领星店铺 id',
    seller_name                 VARCHAR(128)  NULL COMMENT '店铺名称',
    sales_channel               VARCHAR(64)   NULL COMMENT '销售渠道',

    -- ---- 订单状态 / 金额 ----
    order_status                VARCHAR(32)   NULL COMMENT 'Pending/Unshipped/PartiallyShipped/Shipped/Canceled',
    order_total_amount          VARCHAR(32)   NULL COMMENT '订单金额（领星以字符串返回，保真存原样）',
    order_total_currency_code   VARCHAR(8)    NULL COMMENT '币种，金额为 0 时可能为空',
    refund_amount               DECIMAL(14,4) NULL COMMENT '退款金额',
    fulfillment_channel         VARCHAR(16)   NULL COMMENT 'AFN 亚马逊配送 / MFN 自发货',

    -- ---- 标记位（领星返回 0/1）----
    is_return                   TINYINT       NULL COMMENT '退款状态：0 未退款 1 退款中 2 退款完成',
    is_mcf_order                TINYINT       NULL COMMENT '是否多渠道订单',
    is_assessed                 TINYINT       NULL COMMENT '是否推广订单',
    is_replaced_order           TINYINT       NULL COMMENT '是否换货订单',
    is_replacement_order        TINYINT       NULL COMMENT '是否已换货订单',
    is_return_order             TINYINT       NULL COMMENT '是否退货订单',

    -- ---- 商品明细（数组，通用 Upsert 自动 JSON 序列化）----
    item_list                   JSON          NULL COMMENT '商品信息数组（asin/local_sku/quantity 等）',

    -- ---- 时间字段（全 VARCHAR，见文件头说明）----
    purchase_date               VARCHAR(32)   NULL COMMENT '订购时间【亚马逊返回，ISO 带 T/Z】',
    purchase_date_utc           VARCHAR(32)   NULL COMMENT '订购时间【UTC】',
    purchase_date_local         VARCHAR(32)   NULL COMMENT '订购时间【站点时间】',
    purchase_date_local_utc     VARCHAR(32)   NULL COMMENT '订购时间【站点时间转 UTC】',
    shipment_date               VARCHAR(32)   NULL COMMENT '发货日期【亚马逊返回】',
    shipment_date_utc           VARCHAR(32)   NULL COMMENT '发货日期【UTC】',
    shipment_date_local         VARCHAR(32)   NULL COMMENT '发货日期【站点时间】',
    posted_date                 VARCHAR(32)   NULL COMMENT '付款时间【亚马逊返回，常为空串】',
    posted_date_utc             VARCHAR(32)   NULL COMMENT '付款时间【UTC】',
    earliest_ship_date          VARCHAR(32)   NULL COMMENT '发货时限【亚马逊返回】',
    earliest_ship_date_utc      VARCHAR(32)   NULL COMMENT '发货时限【UTC】',
    last_update_date            VARCHAR(32)   NULL COMMENT '订单更新时间【站点时间】',
    last_update_date_utc        VARCHAR(32)   NULL COMMENT '订单更新时间【UTC】',
    gmt_modified                VARCHAR(32)   NULL COMMENT '订单修改时间',
    gmt_modified_utc            VARCHAR(32)   NULL COMMENT '订单修改时间【UTC】',
    hide_time                   VARCHAR(32)   NULL COMMENT '领星侧隐藏时间',

    -- ---- 物流 / 收件信息 ----
    tracking_number             VARCHAR(64)   NULL COMMENT '物流运单号',
    postal_code                 VARCHAR(32)   NULL COMMENT '邮编',

    -- ---- PII：见文件头 ⚠️，不需要可直接删这 5 行 ----
    buyer_name                  VARCHAR(128)  NULL COMMENT '买家姓名（PII）',
    buyer_email                 VARCHAR(255)  NULL COMMENT '买家邮箱（PII）',
    name                        VARCHAR(128)  NULL COMMENT '收件人姓名（PII）',
    phone                       VARCHAR(64)   NULL COMMENT '收件人电话（PII）',
    address                     TEXT          NULL COMMENT '收件地址（PII）',

    synced_at                   DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, amazon_order_id),
    INDEX idx_sid_purchase    (account_id, sid, purchase_date_utc),
    INDEX idx_status          (account_id, order_status),
    INDEX idx_gmt_modified    (account_id, gmt_modified_utc)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '领星 SC 销售订单（/erp/sc/data/mws/orders）';
