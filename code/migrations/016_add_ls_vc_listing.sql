-- VC Listing 原始表。
--
-- 接口：POST /basicOpen/listingManage/vcListing/pageList
-- 请求：vc_store_ids[] + offset/length。
-- 实测：task 715（自营 48 行）与 task 720（联营 41 行）字段全集一致，共 25 字段。
-- 两份样本中 (vc_store_id, asin) 均无空值、无重复；msku 重复，local_sku/product_id
-- 存在空值，不能作为业务键。数组字段按上游原样保存为 JSON。
CREATE TABLE IF NOT EXISTS ls_vc_listing (
    account_id          VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    vc_store_id         VARCHAR(32)  NOT NULL COMMENT '领星 VC 店铺 ID',
    asin                VARCHAR(32)  NOT NULL COMMENT 'Amazon ASIN',

    msku                VARCHAR(255) NULL,
    local_sku           VARCHAR(255) NULL,
    local_name          TEXT         NULL,
    item_name           TEXT         NULL,
    parent_asin         VARCHAR(32)  NULL,
    upc                 VARCHAR(64)  NULL,
    ean                 VARCHAR(64)  NULL,
    asin_url            TEXT         NULL,
    small_min_image_url TEXT         NULL,
    category_name       VARCHAR(512) NULL,
    brand_id            VARCHAR(64)  NULL,
    category_id         VARCHAR(64)  NULL,
    product_id          VARCHAR(64)  NULL,
    reviews_num         VARCHAR(32)  NULL,
    stars               VARCHAR(32)  NULL,
    remark              TEXT         NULL,
    on_sale_time        VARCHAR(64)  NULL,
    status              BIGINT       NULL,
    price               VARCHAR(64)  NULL,
    price_currency_icon VARCHAR(32)  NULL,

    classification_rank JSON         NULL,
    display_group_rank  JSON         NULL,
    principal_list      JSON         NULL,

    synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, vc_store_id, asin),
    INDEX idx_msku      (account_id, vc_store_id, msku),
    INDEX idx_local_sku (account_id, local_sku),
    INDEX idx_status    (account_id, vc_store_id, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'VC Listing /basicOpen/listingManage/vcListing/pageList';
