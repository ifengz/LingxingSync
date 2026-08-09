-- SP 商品广告日报原始表。
--
-- 接口：POST /pb/openapi/newad/spProductAdReports，协议头 X-API-VERSION: 2。
-- 输入：ls_ad_accounts 中每个有效 seller 的 sid + profile_id，加单日报告日期。
-- 联营真实样本确认字段；sid/profile_id 由请求值强制回注，避免上游大整数 JSON number 精度风险。
CREATE TABLE IF NOT EXISTS ls_ad_sp_product (
    account_id   VARCHAR(32)   NOT NULL COMMENT '本系统内部账号 ID',
    sid          VARCHAR(32)   NOT NULL COMMENT 'SC 店铺 ID（请求值）',
    profile_id   VARCHAR(32)   NOT NULL COMMENT '广告 profile ID（请求值）',
    report_date  DATE          NOT NULL,
    ad_id        BIGINT        NOT NULL,
    campaign_id  BIGINT        NULL,
    ad_group_id  BIGINT        NULL,
    asin         VARCHAR(32)   NULL,
    sku          VARCHAR(255)  NULL,
    impressions  BIGINT        NULL,
    clicks       BIGINT        NULL,
    cost         DECIMAL(18,4) NULL,
    same_orders  BIGINT        NULL,
    orders       BIGINT        NULL,
    same_sales   DECIMAL(18,4) NULL,
    sales        DECIMAL(18,4) NULL,
    units        BIGINT        NULL,
    same_units   BIGINT        NULL,
    synced_at    DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid, profile_id, report_date, ad_id),
    INDEX idx_asin (account_id, sid, asin, report_date),
    INDEX idx_campaign (account_id, profile_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='SP 商品广告日报 /pb/openapi/newad/spProductAdReports';
