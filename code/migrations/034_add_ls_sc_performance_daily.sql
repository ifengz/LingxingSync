-- 034：新建 ls_sc_performance_daily（SC 产品表现日维原始表）。
--
-- 接口：POST /bd/productPerformance/openApi/asinList
-- 列名 = 领星返回字段名，一字不改（CLAUDE.md §1.6）。
-- 列类型来源：本地 probe 实测响应（sid=12534，90 行样本），逐字段按真实值判定，
-- 不按字段名猜。实测证据见 sync_task_logs task_id=525。
--
-- ============================================================================
-- 这张表为什么需要「行整形」才能落库（重要，看懂这段才改得动）
-- ============================================================================
-- 领星这个接口返回的一行 = 一个产品在时间窗内的表现，顶层 138 个字段全是指标，
-- 但**没有能认出这行是谁的身份字段**：
--   - asin：不在顶层，埋在 asins[0].asin
--   - sid ：领星压根不返回（它是请求参数，我们按店铺迭代时传进去的）
-- 通用 Upsert 只认顶层字段（列名=字段名），因此这两个键取不到，任何写入都会
-- 撞 NOT NULL 主键而失败。
--
-- 解法不是给这个接口写死代码，而是两个配置驱动的通用机制（internal/worker/shape.go）：
--   field_paths:   {asin: "asins[0].asin"}   把嵌套身份字段提到顶层
--   inject_params: [sid]                     把请求参数回填进每行
-- 对照 polabel2 code/packages/connectors/lingxing-openapi/src/endpoints/sc-performance.ts：
-- 它的 performanceAsin() 同样是「先取顶层 asin，缺失则退到 asins[0].asin」，
-- sid 同样用请求的 storeId 补（optionalPositiveIntegerString(row.sid, ..., request.storeId)）。
-- 结论一致，只是它写成了该接口的专用代码，本项目做成了通用配置。
--
-- ============================================================================
-- 日维语义
-- ============================================================================
-- 配置 single_day_window=true 后，一个 job 把 window_days 或手动范围拆成逐日请求；每次
-- 实际请求 start_date=end_date，worker 把该 start_date 注入 business_date。business_date
-- 只用于本地 raw row，不发送给领星。
--
-- price_list 保持 JSON 原样，不拆行。消费方可按自己的日维需求展开；本表只保存 raw row。
--
-- 幂等：纯 CREATE TABLE IF NOT EXISTS，无 DROP、无 ALTER，重复启动安全。
CREATE TABLE IF NOT EXISTS ls_sc_performance_daily (
    account_id VARCHAR(32) NOT NULL COMMENT '本系统内部账号 ID',

    -- 身份字段（非领星顶层返回，由 shape.go 整形得到，见上方说明）
    sid        VARCHAR(32) NOT NULL COMMENT '领星店铺编号（inject_params 从请求参数回填）',
    asin       VARCHAR(32) NOT NULL COMMENT 'ASIN（field_paths 从 asins[0].asin 提升）',
    business_date DATE NOT NULL COMMENT '实际请求 start_date（single-day window）',

    -- ------------------------------------------------------------------
    -- 领星返回字段（按字母序，138 个，列名严格对齐 API）
    -- ------------------------------------------------------------------
    acoas                                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    acos                                 DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ad_cvr                               DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ad_direct_order_quantity             BIGINT        NULL,
    ad_direct_sales_amount               DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ad_order_quantity                    BIGINT        NULL,
    ad_sales_amount                      DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ads_sales_volume_quantity            BIGINT        NULL,
    ads_sd_cost                          DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ads_sd_sales                         DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ads_sd_sales_volume_quantity         BIGINT        NULL,
    ads_sp_cost                          DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ads_sp_sales                         DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ads_sp_sales_volume_quantity         BIGINT        NULL,
    adv_rate                             DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    afn_fulfillable_quantity             BIGINT        NULL,
    afn_inbound_receiving_quantity       BIGINT        NULL,
    afn_inbound_shipped_quantity         BIGINT        NULL,
    afn_inbound_working_quantity         BIGINT        NULL,
    afn_reserved_quantity                BIGINT        NULL,
    afn_total_inbound                    BIGINT        NULL,
    afn_unsellable_quantity              BIGINT        NULL,
    amount                               DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    amount_chain                         DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    amount_chain_ratio                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    asins                                JSON          NULL,
    asoas                                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    attributes                           JSON          NULL,
    available_days                       BIGINT        NULL,
    available_inventory                  JSON          NULL,
    avg_custom_price                     DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    avg_landed_price                     DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    avg_star                             DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    avg_volume                           DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    b2b_amount                           DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    b2b_order_items                      BIGINT        NULL,
    b2b_volume                           BIGINT        NULL,
    brands                               JSON          NULL,
    buy_box_percentage                   VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    cate_rank                            BIGINT        NULL,
    categories                           JSON          NULL,
    cg_price                             VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    cg_price_currency_icon               VARCHAR(8)    NULL,
    clicks                               BIGINT        NULL,
    comment_rate                         DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    cpc                                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    cpm                                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    cpo                                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    ctr                                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    currency_code                        VARCHAR(8)    NULL,
    currency_icon                        VARCHAR(8)    NULL,
    cvr                                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    developer_names                      JSON          NULL,
    fba_refunds_quantity                 BIGINT        NULL,
    fba_return_goods_count               BIGINT        NULL,
    fba_return_goods_rate                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    fbm_available_days                   VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    fbm_buyer_expenses                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    fbm_quantity                         BIGINT        NULL,
    fbm_refunds_quantity                 BIGINT        NULL,
    fbm_return_goods_count               BIGINT        NULL,
    fbm_return_goods_rate                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    gross_margin                         DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    gross_profit                         DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    has_oprator_log                      VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    icon_num                             BIGINT        NULL,
    impressions                          BIGINT        NULL,
    inventory_sales_ratio                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    item_name                            VARCHAR(1024) NULL,
    local_name                           VARCHAR(512)  NULL,
    local_quantity                       VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    model                                JSON          NULL,
    month_stock_sales_ratio              DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    net_amount                           DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    order_chain_ratio                    DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    order_items                          BIGINT        NULL,
    order_items_chain                    BIGINT        NULL,
    oversea_quantity                     VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    page_views                           BIGINT        NULL,
    page_views_mobile                    BIGINT        NULL,
    page_views_total                     BIGINT        NULL,
    parent_asins                         JSON          NULL,
    pids                                 VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    points_number                        BIGINT        NULL,
    predict_gross_margin                 DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    predict_gross_profit                 DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    prev_cate_rank                       BIGINT        NULL,
    prev_star                            DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    price_list                           JSON          NULL,
    principal_names                      JSON          NULL,
    product_create_time                  VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    promotion_amount                     DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    promotion_discount                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    promotion_order_items                BIGINT        NULL,
    promotion_volume                     BIGINT        NULL,
    rank_category                        VARCHAR(128)  NULL,
    ranking_update_time                  VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    reserved_customerorders              BIGINT        NULL,
    reserved_fc_processing               BIGINT        NULL,
    reserved_fc_transfers                BIGINT        NULL,
    return_amount                        DECIMAL(20,6) NULL,
    return_count                         BIGINT        NULL,
    return_goods_count                   BIGINT        NULL,
    return_goods_rate                    DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    return_rate                          DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    reviews_count                        BIGINT        NULL,
    roas                                 DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    roi                                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    seller_store_countries               JSON          NULL,
    sessions                             BIGINT        NULL,
    sessions_mobile                      BIGINT        NULL,
    sessions_total                       BIGINT        NULL,
    shared_ads_al_cost                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_cc_cost                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_sar_cost                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_sb_cost                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_sb_sales                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_sb_sales_volume_quantity  BIGINT        NULL,
    shared_ads_sbv_cost                  DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_sbv_sales                 DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_ads_sbv_sales_volume_quantity BIGINT        NULL,
    shared_ads_sspaot_cost               DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    shared_cost_of_advertising           VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    sids                                 JSON          NULL,
    sku                                  VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',
    small_cate_rank                      JSON          NULL,
    small_image_url                      VARCHAR(512)  NULL,
    spend                                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    spu_spu_names                        JSON          NULL,
    stock_up_num                         BIGINT        NULL,
    suppliers                            JSON          NULL,
    tacos                                DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    tag_set                              JSON          NULL,
    volume                               BIGINT        NULL,
    volume_chain                         BIGINT        NULL,
    volume_chain_ratio                   DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    volume_cvr                           DECIMAL(20,6) NULL COMMENT '领星以字符串返回小数',
    whs_value                            VARCHAR(64)   NULL COMMENT '未实测：本店铺 90 行全为 null，用 VARCHAR 兼容任意形态',

    synced_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, sid, asin, business_date),
    INDEX idx_asin   (account_id, asin, business_date),
    INDEX idx_amount (account_id, sid, amount)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC 产品表现日维原始表 /bd/productPerformance/openApi/asinList';
