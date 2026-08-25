-- /pb/mp/shop/v2/getSellerList 的多平台店铺映射原始响应。
CREATE TABLE IF NOT EXISTS ls_mp_store_mappings (
    account_id        VARCHAR(32) NOT NULL,
    store_id          VARCHAR(64) NOT NULL,
    sid               VARCHAR(64) NULL,
    store_name        VARCHAR(255) NULL,
    platform_code     VARCHAR(32) NULL,
    platform_name     VARCHAR(128) NULL,
    country_code      CHAR(2) NULL,
    currency          VARCHAR(16) NULL,
    is_sync           TINYINT NULL,
    status            INT NULL,
    synced_at         DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    PRIMARY KEY (account_id, store_id),
    KEY idx_mp_store_sid (account_id, sid),
    KEY idx_mp_store_platform (account_id, platform_code, country_code)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='领星多平台店铺 store_id 映射原始事实';
