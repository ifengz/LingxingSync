-- 新建 SC Listing 原始表。
--
-- 接口：POST /erp/sc/data/mws/listing
-- 请求：按店铺 sid 迭代，分页 offset/length。
-- 字段与类型：2026-08-09 两账号真实全量审计，sc_us_1/sid=1135 共 969 行，
-- sc_us_2/sid=4085 共 487 行；两边字段全集一致，均为 63 个字段。
-- 唯一键：两份样本中 sid、seller_sku 均无空值，组合无重复；listing_id 分别有
-- 21/18 个空值，不能作为主键。polabel2 的官方合同也明确为 sid + seller_sku。
--
-- 字符串数值按上游实际 JSON 类型保留为 VARCHAR，避免空字符串写 DECIMAL 失败；
-- 数组字段保留 JSON，由通用 Upsert 原样序列化。纯 CREATE IF NOT EXISTS，重启幂等。
CREATE TABLE IF NOT EXISTS ls_sc_listing (
    account_id                      VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sid                             VARCHAR(32)  NOT NULL COMMENT '领星 SC 店铺编号',
    seller_sku                      VARCHAR(255) NOT NULL COMMENT 'Seller Central Listing SKU',

    listing_id                      VARCHAR(64)  NULL,
    asin                            VARCHAR(32)  NULL,
    fnsku                           VARCHAR(128) NULL,
    parent_asin                     VARCHAR(32)  NULL,
    parent_msku                     VARCHAR(255) NULL,
    local_sku                       VARCHAR(255) NULL,
    local_name                      VARCHAR(512) NULL,
    item_name                       TEXT         NULL,
    small_image_url                 TEXT         NULL,
    seller_brand                    VARCHAR(255) NULL,
    seller_category                 TEXT         NULL,
    marketplace                     VARCHAR(64)  NULL,
    currency_code                   VARCHAR(8)   NULL,
    fulfillment_channel_type        VARCHAR(16)  NULL,
    store_type                      VARCHAR(16)  NULL,

    status                          BIGINT       NULL,
    is_delete                       BIGINT       NULL,
    quantity                        BIGINT       NULL,
    afn_fulfillable_quantity        BIGINT       NULL,
    afn_reserved_quantity           BIGINT       NULL,
    afn_inbound_shipped_quantity    BIGINT       NULL,
    afn_unsellable_quantity         BIGINT       NULL,
    afn_inbound_working_quantity    BIGINT       NULL,
    afn_inbound_receiving_quantity  BIGINT       NULL,
    reserved_fc_transfers           BIGINT       NULL,
    reserved_fc_processing          BIGINT       NULL,
    reserved_customerorders         BIGINT       NULL,
    seller_rank                     BIGINT       NULL,
    review_num                      BIGINT       NULL,

    price                           VARCHAR(64)  NULL,
    landed_price                    VARCHAR(64)  NULL,
    listing_price                   VARCHAR(64)  NULL,
    list_price                      VARCHAR(64)  NULL,
    b2b_price                       VARCHAR(64)  NULL,
    shipping                        VARCHAR(64)  NULL,
    points                          VARCHAR(64)  NULL,
    last_star                       VARCHAR(64)  NULL,
    total_volume                    VARCHAR(64)  NULL,
    yesterday_volume                VARCHAR(64)  NULL,
    fourteen_volume                 VARCHAR(64)  NULL,
    thirty_volume                   VARCHAR(64)  NULL,
    yesterday_amount                VARCHAR(64)  NULL,
    seven_amount                    VARCHAR(64)  NULL,
    fourteen_amount                 VARCHAR(64)  NULL,
    thirty_amount                   VARCHAR(64)  NULL,
    average_seven_volume            VARCHAR(64)  NULL,
    average_fourteen_volume         VARCHAR(64)  NULL,
    average_thirty_volume           VARCHAR(64)  NULL,

    open_date                       VARCHAR(64)  NULL,
    open_date_display               VARCHAR(64)  NULL,
    listing_update_date             VARCHAR(64)  NULL,
    pair_update_time                VARCHAR(64)  NULL,
    on_sale_time                    VARCHAR(64)  NULL,
    first_order_time                VARCHAR(64)  NULL,

    b2b_price_discount              JSON         NULL,
    dimension_info                  JSON         NULL,
    global_tags                     JSON         NULL,
    principal_info                  JSON         NULL,
    seller_category_new             JSON         NULL,
    small_rank                      JSON         NULL,
    variant                         JSON         NULL,

    synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, sid, seller_sku),
    INDEX idx_asin       (account_id, sid, asin),
    INDEX idx_local_sku  (account_id, local_sku),
    INDEX idx_updated_at (account_id, listing_update_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC Listing /erp/sc/data/mws/listing';
