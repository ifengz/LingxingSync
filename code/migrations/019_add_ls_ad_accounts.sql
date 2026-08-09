-- 广告账号目录原始表。
--
-- 接口：POST /basicOpen/baseData/account/list，当前读取 seller 类型。
-- 真实字段来自两个账号探针；profile_id 两边均非空且各自无重复，故作为上游业务键。
CREATE TABLE IF NOT EXISTS ls_ad_accounts (
    account_id    VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    profile_id    VARCHAR(32)  NOT NULL COMMENT '广告 profile ID',
    sid           VARCHAR(32)  NULL COMMENT '关联 SC 店铺 ID',
    country_code  VARCHAR(16)  NULL,
    currency_code VARCHAR(16)  NULL,
    status        INT          NULL,
    name          VARCHAR(512) NULL,
    type          VARCHAR(32)  NULL,
    synced_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, profile_id),
    INDEX idx_sid (account_id, sid),
    INDEX idx_status (account_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT='广告账号目录 /basicOpen/baseData/account/list';
