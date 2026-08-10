-- 领星 /erp/sc/routing/data/order/removalOrderListNew 原始退仓订单。
-- 响应按 seller_id 维度返回；同一 seller_id 下多个 sid 请求会重复同一份报表。
-- 本表保留 seller_id 作为业务键维度，调用配置可再用 store_sids 选代表店铺。
CREATE TABLE IF NOT EXISTS ls_sc_removal_orders (
    account_id           VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    seller_id            VARCHAR(64)  NOT NULL COMMENT '亚马逊 seller ID',
    sid                  VARCHAR(32)  NULL COMMENT '领星店铺 ID，0 表示未确定店铺',
    region               VARCHAR(16)  NULL,
    request_date         VARCHAR(40)  NULL,
    order_id             VARCHAR(64)  NOT NULL,
    order_type           VARCHAR(32)  NULL,
    order_status         VARCHAR(32)  NULL,
    last_updated_date    VARCHAR(40)  NULL,
    sku                  VARCHAR(128) NOT NULL,
    fnsku                VARCHAR(64)  NOT NULL,
    disposition          VARCHAR(32)  NOT NULL,
    requested_quantity   INT          NULL,
    cancelled_quantity   INT          NULL,
    disposed_quantity    INT          NULL,
    shipped_quantity     INT          NULL,
    in_process_quantity  INT          NULL,
    removal_fee          VARCHAR(32)  NULL COMMENT '领星原始金额字符串',
    currency             VARCHAR(8)   NULL,
    address_detail       TEXT         NULL COMMENT '退仓配送地址，不是买家收件地址',
    country_code         VARCHAR(8)   NULL,
    local_sku             VARCHAR(128) NULL,
    local_name            VARCHAR(256) NULL,
    synced_at             DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, seller_id, order_id, sku, fnsku, disposition),
    INDEX idx_seller_updated (account_id, seller_id, last_updated_date),
    INDEX idx_order (account_id, order_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT '领星 SC 退仓订单原始事实';
