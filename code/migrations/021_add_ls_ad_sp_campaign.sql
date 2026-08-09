-- SP 活动广告日报原始表。
-- 接口：POST /pb/openapi/newad/spCampaignReports。
-- 输入：每个有效 seller 广告账号的 sid + profile_id，加单日报告日期。
-- 真实联营样本确认 campaign_id 在同 profile/日期内唯一。
CREATE TABLE IF NOT EXISTS ls_ad_sp_campaign (
    account_id     VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid            VARCHAR(32)   NOT NULL COMMENT 'SC 店铺 ID（请求值）',
    profile_id     VARCHAR(32)   NOT NULL COMMENT '广告 profile ID（请求值）',
    report_date    DATE          NOT NULL,
    campaign_id    BIGINT        NOT NULL,
    targeting_type VARCHAR(64)   NULL,
    impressions    BIGINT        NULL,
    clicks         BIGINT        NULL,
    cost           DECIMAL(18,4) NULL,
    same_orders    BIGINT        NULL,
    orders         BIGINT        NULL,
    same_sales     DECIMAL(18,4) NULL,
    sales          DECIMAL(18,4) NULL,
    units          BIGINT        NULL,
    same_units     BIGINT        NULL,
    synced_at      DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, profile_id, report_date, campaign_id),
    INDEX idx_campaign (account_id, profile_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='SP 活动广告日报 /pb/openapi/newad/spCampaignReports';
