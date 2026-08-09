-- SC FBA 退货订单原始表。
--
-- 接口：POST /erp/sc/data/mws_report/refundOrders
-- 请求：sid + date_type=1 + start_date/end_date + offset/length。
-- 真实字段来自两账号探针；license_plate_number 在领星 SDK 中标为“唯一序列号”，
-- 两账号样本均非空且无重复，故使用 (account_id, sid, license_plate_number) 作为主键。
CREATE TABLE IF NOT EXISTS ls_sc_refunds (
    account_id              VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid                     VARCHAR(32)  NOT NULL COMMENT 'SC 店铺 ID',
    license_plate_number    VARCHAR(64)  NOT NULL COMMENT '领星唯一序列号',
    order_id                VARCHAR(64)  NULL,
    asin                    VARCHAR(32)  NULL,
    sku                     VARCHAR(255) NULL,
    fnsku                   VARCHAR(255) NULL,
    local_sku               VARCHAR(255) NULL,
    product_name            TEXT         NULL,
    quantity                INT          NULL COMMENT '退货数量',
    return_date             VARCHAR(64)  NULL COMMENT '退货时间（上游 UTC/时区原样）',
    return_date_locale      VARCHAR(32)  NULL COMMENT '退货站点日期',
    purchase_date           VARCHAR(64)  NULL COMMENT '下单时间（上游 UTC/时区原样）',
    purchase_date_locale    VARCHAR(32)  NULL COMMENT '下单站点日期',
    gmt_modified            VARCHAR(64)  NULL COMMENT '上游更新时间',
    fulfillment_center_id   VARCHAR(64)  NULL,
    detailed_disposition    VARCHAR(128) NULL,
    reason                  VARCHAR(128) NULL,
    status                  VARCHAR(255) NULL,
    customer_comments       TEXT         NULL,
    remark                  TEXT         NULL,
    tag                     JSON         NULL,
    synced_at               DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, license_plate_number),
    INDEX idx_return_date (account_id, sid, return_date_locale),
    INDEX idx_order (account_id, sid, order_id),
    INDEX idx_asin (account_id, sid, asin)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='SC FBA 退货订单 /erp/sc/data/mws_report/refundOrders';
