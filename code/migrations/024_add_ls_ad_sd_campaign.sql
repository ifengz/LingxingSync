-- SD 活动广告日报原始表。
-- 接口：POST /pb/openapi/newad/sdCampaignReports。
-- 两账号 probe 均非空；sid/profile_id 由 seller 广告账号请求值强制回填。
CREATE TABLE IF NOT EXISTS ls_ad_sd_campaign (
    account_id                                VARCHAR(32)   NOT NULL,
    sid                                       VARCHAR(32)   NOT NULL,
    profile_id                                VARCHAR(32)   NOT NULL,
    report_date                               DATE          NOT NULL,
    campaign_id                               BIGINT        NOT NULL,
    impressions                               BIGINT        NULL,
    clicks                                    BIGINT        NULL,
    cost                                      DECIMAL(18,4) NULL,
    tactic                                    VARCHAR(64)   NULL,
    same_orders                               BIGINT        NULL,
    orders                                    BIGINT        NULL,
    same_sales                                DECIMAL(18,4) NULL,
    sales                                     DECIMAL(18,4) NULL,
    units                                     BIGINT        NULL,
    attributed_orders_new_to_brand_14d        BIGINT        NULL,
    attributed_sales_new_to_brand_14d         DECIMAL(18,4) NULL,
    attributed_units_ordered_new_to_brand_14d BIGINT        NULL,
    view_impressions                          BIGINT        NULL,
    synced_at                                 DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, profile_id, report_date, campaign_id),
    INDEX idx_campaign (account_id, profile_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='SD 活动广告日报 /pb/openapi/newad/sdCampaignReports';
