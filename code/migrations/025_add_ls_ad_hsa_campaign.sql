-- HSA 活动广告日报原始表。
-- 接口：POST /pb/openapi/newad/hsaCampaignReports。
-- 联营 probe 非空、自营有效空结果；sid/profile_id 由 seller 广告账号请求值强制回填。
CREATE TABLE IF NOT EXISTS ls_ad_hsa_campaign (
    account_id                   VARCHAR(32)   NOT NULL,
    sid                          VARCHAR(32)   NOT NULL,
    profile_id                   VARCHAR(32)   NOT NULL,
    report_date                  DATE          NOT NULL,
    campaign_id                  BIGINT        NOT NULL,
    impressions                  BIGINT        NULL,
    clicks                       BIGINT        NULL,
    cost                         DECIMAL(18,4) NULL,
    same_orders                  BIGINT        NULL,
    orders                       BIGINT        NULL,
    same_sales                   DECIMAL(18,4) NULL,
    sales                        DECIMAL(18,4) NULL,
    same_units                   BIGINT        NULL,
    units                        BIGINT        NULL,
    new_to_brand_orders          BIGINT        NULL,
    new_to_brand_sales           DECIMAL(18,4) NULL,
    new_to_brand_units           BIGINT        NULL,
    new_to_brand_order_rate      DECIMAL(18,6) NULL,
    new_to_brand_order_percentage DECIMAL(18,6) NULL,
    vctr                         DECIMAL(18,6) NULL,
    vtr                          DECIMAL(18,6) NULL,
    synced_at                    DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, profile_id, report_date, campaign_id),
    INDEX idx_campaign (account_id, profile_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='HSA 活动广告日报 /pb/openapi/newad/hsaCampaignReports';
