-- 重建 ls_inventory：列名一字不差对齐领星 /erp/sc/routing/fba/fbaStock/fbaList 返回字段。
-- 探测来源：本地探测模式抓取的真实响应（26 行样本，共 50 个字段）。
-- 字段类型按领星返回样例推断：数值量类用 INT/DECIMAL，文本类用 VARCHAR，数组/对象类用 JSON。
-- 原则：领星返回什么就存什么，不做任何改名/拆分/转换（通用 Upsert：列名=字段名）。

-- 先删旧表（旧的 quantity/reserved_quantity 等列名与领星不符，已无价值）。
--
-- 幂等守卫（重要，勿简化回裸 DROP TABLE）：
-- migrate.go 每次进程启动都把 migrations/ 下所有 .sql 全量重跑（不维护 schema_versions）。
-- 这里原本是裸 `DROP TABLE IF EXISTS ls_inventory`，后果是**每次重启都清空库存表**——
-- 而宪法 §4.6 规定结构性改配置就要重启进程，等于正常运维动作就丢数据。
-- 实测复现：06:03 成功同步 5079 行，重启一次后 ls_inventory 变 0 行。
-- 因此只在「表还是旧结构（没有 fnsku 列）」时才 DROP；已是新结构则空转。
SET @ls_inventory_is_old := (
    SELECT COUNT(*) = 0
    FROM INFORMATION_SCHEMA.COLUMNS
    WHERE TABLE_SCHEMA = DATABASE()
      AND TABLE_NAME = 'ls_inventory'
      AND COLUMN_NAME = 'fnsku'
);
SET @ls_inventory_sql := IF(@ls_inventory_is_old,
    'DROP TABLE IF EXISTS ls_inventory',
    'DO 0');
PREPARE ls_inv_stmt FROM @ls_inventory_sql;
EXECUTE ls_inv_stmt;
DEALLOCATE PREPARE ls_inv_stmt;

CREATE TABLE IF NOT EXISTS ls_inventory (
    account_id              VARCHAR(32)    NOT NULL COMMENT '本系统内部账号 ID',

    -- 领星返回字段（顺序按探测样本，列名严格对齐 API）
    afn_erp_real_shipped_quantity   INT      NULL,
    afn_fulfillable_quantity        INT      NULL,
    afn_fulfillable_quantity_multi  JSON     NULL COMMENT '多站点可售量数组',
    afn_inbound_receiving_quantity  INT      NULL,
    afn_inbound_shipped_quantity    INT      NULL,
    afn_inbound_working_quantity    INT      NULL,
    afn_researching_quantity        INT      NULL,
    afn_reserved_quantity           INT      NULL,
    afn_unsellable_quantity         INT      NULL,
    asin                           VARCHAR(16)  NULL,
    brand_id                       BIGINT       NULL,
    brand_name                     VARCHAR(128) NULL,
    category_id                    BIGINT       NULL,
    category_name                  VARCHAR(128) NULL,
    cost                           DECIMAL(14,4) NULL,
    estimated_excess_quantity      DECIMAL(14,4) NULL,
    estimated_storage_cost_next_month DECIMAL(14,4) NULL,
    fba_inventory_level_health_status VARCHAR(64) NULL,
    fba_minimum_inventory_level    DECIMAL(14,4) NULL,
    fnsku                          VARCHAR(32)  NOT NULL,
    fulfillment_channel_name       VARCHAR(32)  NULL,
    historical_days_of_supply      DECIMAL(14,4) NULL,
    inv_age_0_to_30_days           INT      NULL,
    inv_age_0_to_90_days           INT      NULL,
    inv_age_181_to_270_days        INT      NULL,
    inv_age_271_to_330_days        INT      NULL,
    inv_age_271_to_365_days        INT      NULL,
    inv_age_31_to_60_days          INT      NULL,
    inv_age_331_to_365_days        INT      NULL,
    inv_age_365_plus_days          INT      NULL,
    inv_age_61_to_90_days          INT      NULL,
    inv_age_91_to_180_days         INT      NULL,
    long_term_historical_days_of_supply DECIMAL(14,4) NULL,
    low_inventory_level_fee_applied VARCHAR(32) NULL,
    msku                           VARCHAR(128) NULL,
    name                           VARCHAR(256) NULL COMMENT '产品名',
    product_image                  VARCHAR(512) NULL,
    product_name                   VARCHAR(256) NULL,
    recommended_action             VARCHAR(64)  NULL,
    reserved_customerorders        INT      NULL,
    reserved_fc_processing         INT      NULL,
    reserved_fc_transfers          INT      NULL,
    sell_through                   DECIMAL(10,4) NULL,
    share_type                     VARCHAR(32)  NULL,
    short_term_historical_days_of_supply DECIMAL(14,4) NULL,
    sid                            VARCHAR(32)  NOT NULL,
    sku                            VARCHAR(128) NULL,
    stock_cost_total               DECIMAL(14,4) NULL,
    total_fulfillable_quantity     INT      NULL,
    wname                          VARCHAR(64)  NULL COMMENT '仓库名',

    synced_at                      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, sid, fnsku),
    INDEX idx_asin   (account_id, asin),
    INDEX idx_sku    (account_id, sku),
    INDEX idx_sid    (account_id, sid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
