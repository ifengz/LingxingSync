-- SD 商品广告日报原始表。
-- 接口：POST /pb/openapi/newad/sdProductAdReports，协议头 X-API-VERSION: 2。
-- 输入：每个有效 seller 广告账号的 sid + profile_id，加单日报告日期。
-- 两账号真实样本确认字段和业务键。
CREATE TABLE IF NOT EXISTS ls_ad_sd_product (
    account_id                                    VARCHAR(32)   NOT NULL,
    sid                                           VARCHAR(32)   NOT NULL,
    profile_id                                    VARCHAR(32)   NOT NULL,
    report_date                                   DATE          NOT NULL,
    ad_id                                         BIGINT        NOT NULL,
    asin                                          VARCHAR(32)   NULL,
    sku                                           VARCHAR(255)  NULL,
    ad_group_id                                   BIGINT        NULL,
    campaign_id                                   BIGINT        NULL,
    impressions                                   BIGINT        NULL,
    clicks                                        BIGINT        NULL,
    cost                                          DECIMAL(18,4) NULL,
    tactic                                        VARCHAR(64)   NULL,
    same_orders                                   BIGINT        NULL,
    orders                                        BIGINT        NULL,
    same_sales                                    DECIMAL(18,4) NULL,
    sales                                         DECIMAL(18,4) NULL,
    units                                         BIGINT        NULL,
    attributed_orders_new_to_brand_14d            BIGINT        NULL,
    attributed_sales_new_to_brand_14d             DECIMAL(18,4) NULL,
    attributed_units_ordered_new_to_brand_14d     BIGINT        NULL,
    view_impressions                              BIGINT        NULL,
    synced_at                                     DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, profile_id, report_date, ad_id),
    INDEX idx_asin (account_id, sid, asin, report_date),
    INDEX idx_campaign (account_id, profile_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='SD 商品广告日报 /pb/openapi/newad/sdProductAdReports';
