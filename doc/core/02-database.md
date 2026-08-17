# 领星同步机 — 数据库设计（宪法层）

> 规则：系统表（sync_tasks / sync_task_logs）由框架维护；每个接口或正式报告合同各有一张 `ls_*` 原始证据表，列名与其领星来源字段一一对应。对外数据表必须是静态迁移、来源明确、粒度固定的独立数据产品。

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

每个领星接口或正式报告合同一张表，命名 `ls_{endpoint}` 或明确的 `ls_{report}`，遵守以下规范：

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

`ls_fba_inventory` 是 `/erp/sc/routing/fba/fbaStock/fbaList` 的当前状态原始表，真实列合同以 migration 006 为准。唯一键固定为 `(account_id, sid, fnsku)`；本次接口返回行会刷新 `synced_at`，未返回的旧行不会被伪装成当天数据。

它不是历史表。每次完整 FBA 库存同步成功后，系统把本次返回行写入静态数据产品表 `fba_inventory_daily_snapshots`：

```sql
PRIMARY KEY (account_id, sid, fnsku, snapshot_date)
INDEX idx_fba_inventory_snapshot_date (sid, snapshot_date, account_id)
INDEX idx_fba_inventory_snapshot_changes (sid, updated_at, account_id, fnsku, snapshot_date)
```

历史表显式镜像 migration 006 的全部业务列，并增加 `snapshot_date`、`source_synced_at`、`updated_at`。同一店铺同一天重复成功同步会在一个事务内重建当日快照；跨日新增记录。历史只能从 migration 063 部署后的成功同步开始累计，禁止把当前状态倒填为部署前历史。

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

## 4. 原始数据表合同

`ls_*` 表只保存本项目已验证的领星原始字段，遵守以下约定：

| 约定 | 内容 |
|---|---|
| 项目边界 | 不连接消费者数据库、不适配消费者页面；只通过固定 HTTPS 数据集 API 发布已注册数据表的允许字段 |
| 主键稳定 | 主键按真实接口合同确定；变更前必须先验证冲突并设计数据迁移 |
| 字段追加 | 新增列可随时追加，不删、不改已有列名和类型 |
| JSON 不兜底 | 不用单个 `data` JSON 代替已验证的顶层结构化字段 |
| 时区 | 所有 DATETIME 存 UTC；展示转换不属于同步落库职责 |

## 5. 正式报告原始证据合同（已授权，未声明已实现）

- Amazon 正式报告导出必须使用领星 OpenAPI 凭证和任务链路；ERP `auth-token`、页面 Cookie 与浏览器自动化都不得进入本项目。
- 每个正式报告合同写入自己的一张 `ls_*` 原始表，不与 API 原始表共表，也不 UPDATE API 原始行。
- 报告下载、解析或对账任一步失败时，本批报告不得进入有效日维结果；错误必须可追溯，不能静默采用部分文件。
- 只有成功解析并完成对账的正式报告值，才在 `listing_daily_metrics` 同字段上优先于 API 值；没有报告覆盖时可暂用 API 原始值。

## 6. 静态数据产品合同

`listing_daily_metrics` 是第一个日维数据表，固定粒度为：

```text
store + channel + asin + sku + business_date
```

可以增加一对一的 `listing_dimension_id` 作为紧凑索引键，但必须能无歧义还原上述完整粒度，不能借维度键合并不同 listing。字段和索引由独立迁移显式声明，禁止 UI 或请求参数动态创建 schema。

| 约束 | 内容 |
|---|---|
| 唯一性 | 同一 store/channel/ASIN/SKU/business-date 只能有一条有效行 |
| 来源优先级 | 成功解析、对账的正式报告值 > API 原始值；两份原始证据均保持不变 |
| 未覆盖值 | 保持 `NULL` 并标记未验证，不得把未知来源覆盖伪造成 `0` |
| HSA | 缺 ASIN/SKU 时只进入店铺级独立数据，或以明确 `allocated` 标识进入分摊结果；不得伪装成原始 listing 值 |
| 不同粒度 | 退货原因、订单配送地址等域留在各自明确的明细数据表，禁止塞入 `listing_daily_metrics` |
| 派生上限 | 禁止通用宽表、通用 staging 层或消费者专用投影表；每张数据表必须独立注册、迁移与固定 Reader |

该表只通过版本化 HTTPS 数据集 API 对外读取；消费者不得直连 MySQL、提交远程 SQL 或指定任意表名。
