-- /basicOpen/platformOrder/vcOrderPo/detail 原始单对象响应。
-- detail 不回 vc_store_id；该列只从同账号 ls_vc_orders 候选上下文强制写入。
CREATE TABLE IF NOT EXISTS ls_vc_po_details (
    account_id             VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    vc_store_id            VARCHAR(32)  NOT NULL COMMENT '同账号 VC PO 列表候选的店铺 ID',

    purchase_order_number  VARCHAR(64)  NULL,
    local_po_number        VARCHAR(64)  NOT NULL,
    purchase_order_date    VARCHAR(32)  NULL,
    purchase_order_state   VARCHAR(32)  NULL,
    payment_method         VARCHAR(64)  NULL,
    total_price            VARCHAR(32)  NULL,
    currency_code          VARCHAR(8)   NULL,
    item_amount            DECIMAL(18,4) NULL,
    ship_window_start      VARCHAR(32)  NULL,
    ship_window_end        VARCHAR(32)  NULL,
    delivery_window_start  VARCHAR(32)  NULL,
    delivery_window_end    VARCHAR(32)  NULL,
    items JSON NULL,

    synced_at              DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, vc_store_id, local_po_number),
    INDEX idx_purchase_order (account_id, vc_store_id, purchase_order_number),
    INDEX idx_purchase_date (account_id, vc_store_id, purchase_order_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'VC PO 订单详情原始对象';
