CREATE TABLE IF NOT EXISTS ls_ad_vendor_accounts (
    account_id    VARCHAR(32)  NOT NULL,
    profile_id    VARCHAR(64)  NOT NULL,
    country_code  VARCHAR(16)  NULL,
    currency_code VARCHAR(16)  NULL,
    currency      VARCHAR(16)  NULL,
    status        INT          NULL,
    name          VARCHAR(512) NULL,
    type          VARCHAR(32)  NOT NULL DEFAULT 'vendor',
    synced_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, profile_id),
    INDEX idx_status (account_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC vendor 广告 profile 目录 /basicOpen/baseData/account/list';

CREATE TABLE IF NOT EXISTS ls_ad_vc_sp_product (
    account_id   VARCHAR(32)   NOT NULL,
    profile_id   VARCHAR(64)   NOT NULL,
    report_date  DATE          NOT NULL,
    ad_id        BIGINT        NOT NULL,
    campaign_id  BIGINT        NULL,
    ad_group_id  BIGINT        NULL,
    asin         VARCHAR(32)   NULL,
    sku          VARCHAR(255)  NULL,
    impressions  BIGINT        NULL,
    clicks       BIGINT        NULL,
    cost         DECIMAL(18,4) NULL,
    orders       BIGINT        NULL,
    sales        DECIMAL(18,4) NULL,
    synced_at    DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, profile_id, report_date, ad_id),
    INDEX idx_asin (account_id, profile_id, asin, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC SP 商品广告日报 /pb/openapi/newad/spProductAdReports';

CREATE TABLE IF NOT EXISTS ls_ad_vc_sd_product (
    account_id   VARCHAR(32)   NOT NULL,
    profile_id   VARCHAR(64)   NOT NULL,
    report_date  DATE          NOT NULL,
    ad_id        BIGINT        NOT NULL,
    campaign_id  BIGINT        NULL,
    ad_group_id  BIGINT        NULL,
    asin         VARCHAR(32)   NULL,
    sku          VARCHAR(255)  NULL,
    impressions  BIGINT        NULL,
    clicks       BIGINT        NULL,
    cost         DECIMAL(18,4) NULL,
    orders       BIGINT        NULL,
    sales        DECIMAL(18,4) NULL,
    synced_at    DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, profile_id, report_date, ad_id),
    INDEX idx_asin (account_id, profile_id, asin, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC SD 商品广告日报 /pb/openapi/newad/sdProductAdReports';

CREATE TABLE IF NOT EXISTS ls_ad_vc_hsa_product (
    account_id      VARCHAR(32)   NOT NULL,
    profile_id      VARCHAR(64)   NOT NULL,
    report_date     DATE          NOT NULL,
    ad_creative_id  BIGINT        NOT NULL,
    campaign_id     BIGINT        NULL,
    ad_group_id     BIGINT        NULL,
    impressions     BIGINT        NULL,
    clicks          BIGINT        NULL,
    cost            DECIMAL(18,4) NULL,
    orders          BIGINT        NULL,
    sales           DECIMAL(18,4) NULL,
    synced_at       DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, profile_id, report_date, ad_creative_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC HSA/SB 商品广告日报 /pb/openapi/newad/listHsaProductAdReport';

CREATE TABLE IF NOT EXISTS vc_ad_daily (
    account_id         VARCHAR(32)   NOT NULL,
    profile_id         VARCHAR(64)   NOT NULL,
    attribution_scope  VARCHAR(24)   NOT NULL,
    asin               VARCHAR(32)   NOT NULL DEFAULT '',
    business_date      DATE          NOT NULL,
    campaign_type      VARCHAR(8)    NOT NULL,
    spend              DECIMAL(18,4) NULL,
    ad_sales           DECIMAL(18,4) NULL,
    ad_orders          BIGINT        NULL,
    clicks             BIGINT        NULL,
    impressions        BIGINT        NULL,
    currency           VARCHAR(16)   NULL,
    synced_at          DATETIME      NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, profile_id, attribution_scope, asin, business_date, campaign_type),
    INDEX idx_profile_date (account_id, profile_id, business_date),
    CONSTRAINT chk_vc_ad_scope CHECK (attribution_scope IN ('asin', 'profile_unattributed'))
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='VC 广告按 profile/ASIN/日期规范事实';
