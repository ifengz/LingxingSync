-- 唯一 listing 日维有效事实。ls_* API/报告原始证据均不在本迁移中修改。
-- listing_dimensions 将完整 listing 身份压缩为一对一键；事实行仍可无歧义还原
-- store/channel/ASIN/SKU/business_date 粒度。HSA 店铺级行显式 identity_scope=store。
CREATE TABLE IF NOT EXISTS listing_dimensions (
    id             BIGINT UNSIGNED NOT NULL AUTO_INCREMENT,
    store_id       VARCHAR(64)  NOT NULL,
    channel        VARCHAR(32)  NOT NULL,
    identity_scope VARCHAR(16)  NOT NULL,
    identity_key   VARCHAR(600) NOT NULL,
    asin           VARCHAR(32)  NULL,
    sku            VARCHAR(255) NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (id),
    UNIQUE KEY uq_listing_dimension (store_id, channel, identity_scope, identity_key),
    CONSTRAINT chk_listing_dimension_scope CHECK (
        (identity_scope = 'listing' AND asin IS NOT NULL AND sku IS NOT NULL) OR
        (identity_scope = 'allocated' AND channel IN ('hsa', 'sb') AND asin IS NOT NULL AND sku IS NOT NULL) OR
        (identity_scope = 'store' AND channel IN ('hsa', 'sb') AND asin IS NULL AND sku IS NULL)
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='listing 一对一身份维度；HSA 店铺级不伪造 ASIN/SKU';

CREATE TABLE IF NOT EXISTS listing_daily_metrics (
    listing_dimension_id BIGINT UNSIGNED NOT NULL,
    business_date        DATE NOT NULL,

    sales_units              BIGINT NULL,
    sales_units_source       VARCHAR(16) NOT NULL DEFAULT '',
    sales_amount             DECIMAL(20,6) NULL,
    sales_amount_source      VARCHAR(16) NOT NULL DEFAULT '',
    returns_qty              BIGINT NULL,
    returns_qty_source       VARCHAR(16) NOT NULL DEFAULT '',
    inventory_sellable       BIGINT NULL,
    inventory_sellable_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_inbound        BIGINT NULL,
    inventory_inbound_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_reserved       BIGINT NULL,
    inventory_reserved_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_unfulfillable  BIGINT NULL,
    inventory_unfulfillable_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_local_warehouse BIGINT NULL,
    inventory_local_warehouse_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_unhealthy_units BIGINT NULL,
    inventory_unhealthy_units_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_aged90_sellable_units BIGINT NULL,
    inventory_aged90_sellable_units_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_sell_through_rate DECIMAL(20,6) NULL,
    inventory_sell_through_rate_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_receive_fill_rate DECIMAL(20,6) NULL,
    inventory_receive_fill_rate_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_vendor_confirmation_rate DECIMAL(20,6) NULL,
    inventory_vendor_confirmation_rate_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_avg_lead_time_days DECIMAL(20,6) NULL,
    inventory_avg_lead_time_days_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_sellable_cost DECIMAL(20,6) NULL,
    inventory_sellable_cost_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_unfulfillable_cost DECIMAL(20,6) NULL,
    inventory_unfulfillable_cost_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_aged90_cost DECIMAL(20,6) NULL,
    inventory_aged90_cost_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_unhealthy_cost DECIMAL(20,6) NULL,
    inventory_unhealthy_cost_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_inbound_cost DECIMAL(20,6) NULL,
    inventory_inbound_cost_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_currency VARCHAR(8) NULL,
    inventory_currency_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_inbound_receiving BIGINT NULL,
    inventory_inbound_receiving_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_inbound_shipped BIGINT NULL,
    inventory_inbound_shipped_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_inbound_working BIGINT NULL,
    inventory_inbound_working_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_reserved_customer_orders BIGINT NULL,
    inventory_reserved_customer_orders_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_reserved_fc_processing BIGINT NULL,
    inventory_reserved_fc_processing_source VARCHAR(16) NOT NULL DEFAULT '',
    inventory_reserved_fc_transfers BIGINT NULL,
    inventory_reserved_fc_transfers_source VARCHAR(16) NOT NULL DEFAULT '',
    sessions_desktop         BIGINT NULL,
    sessions_desktop_source  VARCHAR(16) NOT NULL DEFAULT '',
    sessions_mobile          BIGINT NULL,
    sessions_mobile_source   VARCHAR(16) NOT NULL DEFAULT '',
    sessions_total           BIGINT NULL,
    sessions_total_source    VARCHAR(16) NOT NULL DEFAULT '',
    review_count             BIGINT NULL,
    review_count_source      VARCHAR(16) NOT NULL DEFAULT '',
    rating                   DECIMAL(10,4) NULL,
    rating_source            VARCHAR(16) NOT NULL DEFAULT '',
    sp_spend                 DECIMAL(20,6) NULL,
    sp_spend_source          VARCHAR(16) NOT NULL DEFAULT '',
    sp_sales                 DECIMAL(20,6) NULL,
    sp_sales_source          VARCHAR(16) NOT NULL DEFAULT '',
    sp_orders                BIGINT NULL,
    sp_orders_source         VARCHAR(16) NOT NULL DEFAULT '',
    sd_spend                 DECIMAL(20,6) NULL,
    sd_spend_source          VARCHAR(16) NOT NULL DEFAULT '',
    sd_sales                 DECIMAL(20,6) NULL,
    sd_sales_source          VARCHAR(16) NOT NULL DEFAULT '',
    sd_orders                BIGINT NULL,
    sd_orders_source         VARCHAR(16) NOT NULL DEFAULT '',
    hsa_spend                DECIMAL(20,6) NULL,
    hsa_spend_source         VARCHAR(16) NOT NULL DEFAULT '',
    hsa_sales                DECIMAL(20,6) NULL,
    hsa_sales_source         VARCHAR(16) NOT NULL DEFAULT '',
    hsa_orders               BIGINT NULL,
    hsa_orders_source        VARCHAR(16) NOT NULL DEFAULT '',
    sb_spend                 DECIMAL(20,6) NULL,
    sb_spend_source          VARCHAR(16) NOT NULL DEFAULT '',
    sb_sales                 DECIMAL(20,6) NULL,
    sb_sales_source          VARCHAR(16) NOT NULL DEFAULT '',
    sb_orders                BIGINT NULL,
    sb_orders_source         VARCHAR(16) NOT NULL DEFAULT '',

    is_provisional       BOOLEAN NOT NULL DEFAULT TRUE,
    is_verified          BOOLEAN NOT NULL DEFAULT FALSE,
    verified_fields      JSON NOT NULL,
    report_verified_at   DATETIME NULL,
    created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at           DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (listing_dimension_id, business_date),
    CONSTRAINT fk_listing_daily_dimension
        FOREIGN KEY (listing_dimension_id) REFERENCES listing_dimensions(id),
    CONSTRAINT chk_listing_daily_sources CHECK (
        sales_units_source IN ('', 'api', 'report') AND
        sales_amount_source IN ('', 'api', 'report') AND
        returns_qty_source IN ('', 'api', 'report') AND
        inventory_sellable_source IN ('', 'api', 'report') AND
        inventory_inbound_source IN ('', 'api', 'report') AND
        inventory_reserved_source IN ('', 'api', 'report') AND
        inventory_unfulfillable_source IN ('', 'api', 'report') AND
        inventory_local_warehouse_source IN ('', 'api', 'report') AND
        inventory_unhealthy_units_source IN ('', 'api', 'report') AND
        inventory_aged90_sellable_units_source IN ('', 'api', 'report') AND
        inventory_sell_through_rate_source IN ('', 'api', 'report') AND
        inventory_receive_fill_rate_source IN ('', 'api', 'report') AND
        inventory_vendor_confirmation_rate_source IN ('', 'api', 'report') AND
        inventory_avg_lead_time_days_source IN ('', 'api', 'report') AND
        inventory_sellable_cost_source IN ('', 'api', 'report') AND
        inventory_unfulfillable_cost_source IN ('', 'api', 'report') AND
        inventory_aged90_cost_source IN ('', 'api', 'report') AND
        inventory_unhealthy_cost_source IN ('', 'api', 'report') AND
        inventory_inbound_cost_source IN ('', 'api', 'report') AND
        inventory_currency_source IN ('', 'api', 'report') AND
        inventory_inbound_receiving_source IN ('', 'api', 'report') AND
        inventory_inbound_shipped_source IN ('', 'api', 'report') AND
        inventory_inbound_working_source IN ('', 'api', 'report') AND
        inventory_reserved_customer_orders_source IN ('', 'api', 'report') AND
        inventory_reserved_fc_processing_source IN ('', 'api', 'report') AND
        inventory_reserved_fc_transfers_source IN ('', 'api', 'report') AND
        sessions_desktop_source IN ('', 'api', 'report') AND
        sessions_mobile_source IN ('', 'api', 'report') AND
        sessions_total_source IN ('', 'api', 'report') AND
        review_count_source IN ('', 'api', 'report') AND
        rating_source IN ('', 'api', 'report') AND
        sp_spend_source IN ('', 'api', 'report') AND
        sp_sales_source IN ('', 'api', 'report') AND
        sp_orders_source IN ('', 'api', 'report') AND
        sd_spend_source IN ('', 'api', 'report') AND
        sd_sales_source IN ('', 'api', 'report') AND
        sd_orders_source IN ('', 'api', 'report') AND
        hsa_spend_source IN ('', 'api', 'report') AND
        hsa_sales_source IN ('', 'api', 'report') AND
        hsa_orders_source IN ('', 'api', 'report') AND
        sb_spend_source IN ('', 'api', 'report') AND
        sb_sales_source IN ('', 'api', 'report') AND
        sb_orders_source IN ('', 'api', 'report')
    )
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='唯一 listing 日维事实；报告字段优先、未知保持 NULL';

CREATE TABLE IF NOT EXISTS listing_daily_reconciliations (
    report_audit_id    BIGINT NOT NULL,
    report_task_id     VARCHAR(128) NOT NULL,
    business_date      DATE NOT NULL,
    status             VARCHAR(16) NOT NULL,
    missing_in_db      JSON NOT NULL,
    missing_in_report  JSON NOT NULL,
    field_diffs        JSON NOT NULL,
    error_message      TEXT NULL,
    created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (report_audit_id, business_date),
    INDEX idx_listing_daily_reconciliation_task (report_task_id, business_date),
    CONSTRAINT chk_listing_daily_reconciliation_status CHECK (status IN ('matched', 'corrected', 'failed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='正式报表与 API 日维对账结果；不存业务事实';
