-- VC 店铺广告 Profile ID 人工映射。该字段不是店铺接口返回值，不能写入 ls_stores。
CREATE TABLE IF NOT EXISTS vc_store_profiles (
    account_id VARCHAR(32) NOT NULL COMMENT '本系统内部账号 ID',
    sid        VARCHAR(32) NOT NULL COMMENT 'ls_stores 中的 VC 店铺 ID',
    profile_id VARCHAR(32) NOT NULL COMMENT '人工确认的广告 Profile ID',
    updated_at DATETIME    NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, sid)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
