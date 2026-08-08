# 领星同步机 — 数据库设计（宪法层）

> 规则：系统表（sync_tasks / sync_task_logs）由框架维护；数据表（ls_*）每个接口一张，列名与领星 API 字段一一对应，polabel2 直读。

---

## 1. 系统表

### sync_tasks（同步任务状态，每次触发一行）

```sql
CREATE TABLE sync_tasks (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    endpoint     VARCHAR(64)  NOT NULL COMMENT '接口标识，如 sales_orders',
    account_id   VARCHAR(32)  NOT NULL COMMENT '领星账号 ID',
    status       ENUM('pending','running','success','error','cancelled')
                 NOT NULL DEFAULT 'pending',
    trigger_type ENUM('cron','manual') NOT NULL DEFAULT 'cron',
    started_at   DATETIME     NULL,
    finished_at  DATETIME     NULL,
    records_upserted INT       NOT NULL DEFAULT 0,
    pages_fetched    INT       NOT NULL DEFAULT 0,
    error_message    TEXT      NULL      COMMENT '失败原因，保留完整原始错误',
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_endpoint_status  (endpoint, status),
    INDEX idx_account_created  (account_id, created_at),
    INDEX idx_status_created   (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### sync_task_logs（每页请求的证据，不丢）

```sql
CREATE TABLE sync_task_logs (
    id           BIGINT AUTO_INCREMENT PRIMARY KEY,
    task_id      BIGINT       NOT NULL,
    page         INT          NOT NULL,
    http_status  SMALLINT     NULL COMMENT 'HTTP 状态码',
    api_code     INT          NULL COMMENT '领星 API code 字段',
    records_count INT         NOT NULL DEFAULT 0,
    error_raw    TEXT         NULL COMMENT '原始错误消息（不截断）',
    duration_ms  INT          NOT NULL DEFAULT 0,
    created_at   DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_task_id (task_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**留存策略**：`sync_task_logs` 保留 90 天；`sync_tasks` 保留 365 天。批量删除速率必须 > 插入速率（见07-deployment-maintenance）。

---

## 2. 数据表规范（ls_* 系列）

每个领星接口一张表，命名 `ls_{endpoint}`，遵守以下规范：

```sql
-- 模板（替换 {table}、{columns}、{unique_key_cols}）
-- 唯一键几乎都是复合键（领星 = 业务字段 + sid），见 09-endpoint-contract.md 格 3
CREATE TABLE ls_{table} (
    account_id        VARCHAR(32)  NOT NULL COMMENT '领星账号 ID（本系统内部账号）',

    -- 唯一键各列（照领星「唯一键说明」抄全；如 asin + sid）
    {unique_key_cols}

    -- 领星返回的业务字段（原名，不翻译）
    {columns}

    -- 系统字段
    gmt_modified  DATETIME     NULL COMMENT '领星侧最后修改时间',
    synced_at     DATETIME     NOT NULL DEFAULT CURRENT_TIMESTAMP
                               ON UPDATE CURRENT_TIMESTAMP,

    -- 主键 = account_id + 领星唯一键各列；缺一列都会跨店铺互相覆盖
    PRIMARY KEY (account_id, {unique_key_cols_names}),
    INDEX idx_gmt_modified (account_id, gmt_modified)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

> `record_id_fields`（config 里的数组）→ 逐列落成 `PRIMARY KEY (account_id, col1, col2, ...)`。
> 单键接口就是数组只有一个元素的特例，不是两套写法。

---

## 3. 数据表实例

### ls_sales_orders（FBA/FBM 销售订单）

```sql
CREATE TABLE ls_sales_orders (
    order_id          VARCHAR(64)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    order_status      VARCHAR(32)    NULL,
    order_type        VARCHAR(16)    NULL COMMENT 'FBA / FBM',
    store_id          VARCHAR(32)    NULL,
    store_name        VARCHAR(128)   NULL,
    asin              VARCHAR(16)    NULL,
    sku               VARCHAR(128)   NULL,
    listing_sku       VARCHAR(128)   NULL,
    quantity          INT            NOT NULL DEFAULT 0,
    amount            DECIMAL(14,4)  NOT NULL DEFAULT 0,
    currency          VARCHAR(8)     NULL,
    sales_channel     VARCHAR(64)    NULL,
    purchase_date     DATETIME       NULL,
    fulfillment_date  DATETIME       NULL,
    marketplace_id    VARCHAR(16)    NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, order_id),
    INDEX idx_store_date     (account_id, store_id, purchase_date),
    INDEX idx_sku_date       (account_id, sku, purchase_date),
    INDEX idx_gmt_modified   (account_id, gmt_modified)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### ls_fba_inventory（SC FBA 库存）

```sql
CREATE TABLE ls_fba_inventory (
    fnsku             VARCHAR(32)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    asin              VARCHAR(16)    NULL,
    sku               VARCHAR(128)   NULL,
    store_id          VARCHAR(32)    NULL,
    store_name        VARCHAR(128)   NULL,
    product_name      VARCHAR(256)   NULL,
    quantity          INT            NOT NULL DEFAULT 0,
    reserved_quantity INT            NOT NULL DEFAULT 0,
    inbound_quantity  INT            NOT NULL DEFAULT 0,
    warehouse         VARCHAR(64)    NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, fnsku),
    INDEX idx_store   (account_id, store_id),
    INDEX idx_sku     (account_id, sku)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### ls_ads_daily（广告日报）

```sql
CREATE TABLE ls_ads_daily (
    report_id         VARCHAR(64)    NOT NULL,
    account_id        VARCHAR(32)    NOT NULL,
    store_id          VARCHAR(32)    NULL,
    profile_id        VARCHAR(32)    NULL,
    campaign_id       VARCHAR(32)    NULL,
    campaign_name     VARCHAR(256)   NULL,
    ad_group_id       VARCHAR(32)    NULL,
    ad_group_name     VARCHAR(128)   NULL,
    asin              VARCHAR(16)    NULL,
    sku               VARCHAR(128)   NULL,
    report_date       DATE           NOT NULL,
    impressions       INT            NOT NULL DEFAULT 0,
    clicks            INT            NOT NULL DEFAULT 0,
    spend             DECIMAL(12,4)  NOT NULL DEFAULT 0,
    sales             DECIMAL(12,4)  NOT NULL DEFAULT 0,
    orders            INT            NOT NULL DEFAULT 0,
    currency          VARCHAR(8)     NULL,
    gmt_modified      DATETIME       NULL,
    synced_at         DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (account_id, report_id),
    INDEX idx_store_date  (account_id, store_id, report_date),
    INDEX idx_campaign    (account_id, campaign_id, report_date)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

---

## 4. polabel2 消费契约

polabel2 直连只读账号，遵守以下约定：

| 约定 | 内容 |
|---|---|
| 只读 | polabel2 使用独立只读 MySQL 账号，`GRANT SELECT ON lingsync.*` |
| 主键稳定 | `(account_id, {record_id_field})` 不改，polabel2 可以 JOIN |
| 字段追加 | 新增列可随时追加，不删、不改已有列名和类型 |
| 不暴露系统表 | `sync_tasks` / `sync_task_logs` 不在只读账号权限内 |
| JSON 不兜底 | 无 data JSON 大列；polabel2 直接用结构化列，无需解析 |
| 时区 | 所有 DATETIME 存 UTC；polabel2 自行转换展示 |
