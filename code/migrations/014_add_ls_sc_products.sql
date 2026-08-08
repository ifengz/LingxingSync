-- 新建 SC 产品列表原始表。
--
-- 接口：POST /erp/sc/routing/data/local_inventory/productList
-- 请求：账号级 offset/length 分页。
-- 字段与类型：2026-08-09 两账号真实全量审计，sc_us_1 共 8474 行、sc_us_2 共
-- 207 行；字段全集一致，均为 30 个字段。sku 无空值且各账号内无重复，作为业务键；
-- id 保留为领星上游产品 ID 并建索引，不替代 sku。
--
-- 数值字符串保留 VARCHAR，避免空字符串写 DECIMAL 失败；供应商报价等数组保留 JSON。
CREATE TABLE IF NOT EXISTS ls_sc_products (
    account_id              VARCHAR(32)  NOT NULL COMMENT '本系统内部账号 ID',
    sku                     VARCHAR(255) NOT NULL COMMENT '领星产品 SKU，业务唯一键',

    id                      BIGINT       NULL COMMENT '领星上游产品 ID',
    sku_identifier          VARCHAR(255) NULL,
    product_name            TEXT         NULL,
    pic_url                 TEXT         NULL,
    brand_name              VARCHAR(255) NULL,
    category_name           VARCHAR(255) NULL,
    spu                     VARCHAR(255) NULL,
    status                  BIGINT       NULL,
    status_text             VARCHAR(64)  NULL,
    open_status             BIGINT       NULL,
    is_combo                BIGINT       NULL,

    bid                     BIGINT       NULL,
    cid                     BIGINT       NULL,
    ps_id                   BIGINT       NULL,
    cg_delivery             BIGINT       NULL,
    cg_price                VARCHAR(64)  NULL,
    cg_transport_costs      VARCHAR(64)  NULL,
    cg_opt_uid              BIGINT       NULL,
    cg_opt_username         VARCHAR(255) NULL,
    product_developer_uid   BIGINT       NULL,
    product_developer       VARCHAR(255) NULL,
    purchase_remark         TEXT         NULL,
    create_time             BIGINT       NULL,
    update_time             BIGINT       NULL,

    attribute               JSON         NULL,
    aux_relation_list       JSON         NULL,
    custom_fields           JSON         NULL,
    global_tags             JSON         NULL,
    supplier_quote           JSON        NULL,

    synced_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    PRIMARY KEY (account_id, sku),
    INDEX idx_upstream_id (account_id, id),
    INDEX idx_status      (account_id, status),
    INDEX idx_updated_at  (account_id, update_time)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4
  COMMENT 'SC 产品列表 /erp/sc/routing/data/local_inventory/productList';
