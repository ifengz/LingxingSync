# 领星接口/正式报告接入合同（Endpoint/Report Contract）

> 每新增一个领星接口，只需填这五格。所有信息直接来自领星官方接口文档页。
>
> 官方接口文档才是权威字典 — 它给出令牌桶容量、限流间隔、参数类型、唯一键，这些是 SDK 抽象掉的东西。
>
> 每个接口或正式报告合同只对应自己的一张 `ls_*` 原始证据表。接口响应与报告文件不得共表或互相覆盖。

---

## 五格接入合同

| # | 格 | 从文档哪里拿 |
|---|---|---|
| 1 | **path + method** | 文档页标题区 |
| 2 | **请求参数**（字段名/类型/必填） | Request Parameters 表 |
| 3 | **幂等键**（Upsert 主键） | Response 里唯一标识字段 |
| 4 | **限流档案**（桶容量、间隔、限流维度） | Rate Limit / 频率限制 区块 |
| 5 | **目标表结构**（列名 = 领星字段名） | Response Data 表 |

填完即可执行：建表 → config.yaml 加段 → 重启。零代码改动。

### 正式报告合同的附加格

正式 Amazon 报告不是 ERP 页面导出，也不能套用一个模糊的「报表」endpoint。除上述字段外，还必须逐报告写清：

| 格 | 必填证据 |
|---|---|
| OpenAPI 身份 | `seller_id`、`report_type` 与所用 app 凭证范围；禁止 ERP `auth-token` / Cookie |
| 异步链路 | 创建任务、查询状态、下载链接获取/续期各自的正式 path、method 和终态 |
| 文件合同 | 文件格式、编码、sheet/header、业务日期、唯一键、字段类型和空值语义 |
| 独立原始表 | 该报告独占的 `ls_*` 表；不得写 API 原始表 |
| 对账门 | 下载、解析、唯一键和覆盖范围全部成功后，才允许报告值进入有效日维结果 |

任何一格未知都保持候选/未验证；不得通过 ERP 内部页面浏览器自动化补洞，也不得把未知覆盖写成零。

---

## 完整示例：查询产品表现（/bd/productPerformance/openApi/asinList）

### 格 1 — Path + Method

```
POST /bd/productPerformance/openApi/asinList
```

### 格 2 — 请求参数

| 参数 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `sid` | string | 是 | 店铺 ID |
| `start_date` | string | 是 | 开始日期 YYYY-MM-DD |
| `end_date` | string | 是 | 结束日期 YYYY-MM-DD |
| `offset` | int | 是 | 分页偏移，从 0 开始 |
| `length` | int | 是 | 每页条数，最大 200 |

### 格 3 — 幂等键

官方文档唯一键（按实际返回选其一）：

| 唯一键组合 | 适用场景 |
|---|---|
| `asin + sid` | 按 ASIN 聚合 |
| `parent_asin + sid` | 按父 ASIN 聚合 |
| `seller_sku + sid` | 按 Seller SKU 聚合 |
| `sku + sid` | 按 SKU 聚合 |

建表时选一个作 PRIMARY KEY，其余可建普通索引。

### 格 4 — 限流档案

从文档 Rate Limit 区块原样抄写：

```yaml
rate:
  bucket: 1                # 令牌桶容量
  serial: true             # 桶=1 → 强制串行：前一个请求返回后才能发下一个
  interval_ms: 1000        # 单店铺调用最小间隔（ms）
  multi_interval_ms: 10000 # 多店铺调用最小间隔（ms）
  dimension: "account+path"  # 限流维度：领星按 (账号+路径) 共享所有 appId 的配额
```

⚠️ `bucket: 1` 意味着这个接口**不能并发翻页**，只能串行分页 + 间隔等待。并发只能发生在不同 path 或不同账号之间。

### 格 5 — 目标表结构

```sql
-- migrations/00N_add_ls_product_perf.sql
CREATE TABLE ls_product_perf (
    asin               VARCHAR(16)    NOT NULL,
    sid                VARCHAR(32)    NOT NULL,
    parent_asin        VARCHAR(16)    NULL,
    seller_sku         VARCHAR(128)   NULL,
    sku                VARCHAR(128)   NULL,
    -- 按文档 Response 字段 1:1 填入（字段名不翻译）
    page_views         INT            NOT NULL DEFAULT 0,
    sessions           INT            NOT NULL DEFAULT 0,
    units_ordered      INT            NOT NULL DEFAULT 0,
    ordered_product_sales DECIMAL(14,4) NOT NULL DEFAULT 0,
    currency           VARCHAR(8)     NULL,
    report_date        DATE           NOT NULL,
    gmt_modified       DATETIME       NULL,
    synced_at          DATETIME       NOT NULL DEFAULT CURRENT_TIMESTAMP
                                      ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (asin, sid),            -- 幂等键
    INDEX idx_date  (sid, report_date),
    INDEX idx_sku   (sid, seller_sku)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 对应 config.yaml 段

```yaml
- name: sc_product_perf
  display: "产品表现（ASIN）"
  account: "sc_us"
  path: "/bd/productPerformance/openApi/asinList"   # 格 1：从领星文档"API Path"抄
  method: "POST"                                     # 格 1：从领星文档"请求方式"抄
  table: "ls_product_perf"
  record_id_fields: ["asin", "sid"]      # 格 3：复合主键，照领星"唯一键说明"填
  iterate_by_store: true                 # Worker 自动对每个 sid 循环一次
  store_param_name: "sid"               # 迭代时注入的参数名
  rate:                                  # 格 4：从领星文档"限流算法说明"抄
    bucket: 1
    interval_ms: 1000
    multi_interval_ms: 10000
    dimension: "account+path"
  cron: "0 4 * * *"
  enabled: true
  window_days: 30
```

---

## 限流键规则（技术核心）

领星按 `(账号, path)` 维度做全局配额，同一账号下所有 appId 共享这个桶。

**运行时限流器 key = `(quota_group, path)`**

| 场景 | quota_group | 结果 |
|---|---|---|
| 一个 app_key → 一个账号 | 取 `accounts[].id` | 各账号独立桶，互不干扰 |
| 多个 app_key → 同一领星公司账号 | 手动设 `quota_group` 为同一值 | 共享一个桶，不超配额 |

在 `accounts` 配置里可选填 `quota_group`；默认等于 `id`：

```yaml
accounts:
  - id: "sc_us_key1"
    quota_group: "sc_us"     # 同公司的两个 key 共享限流桶
    app_key: "..."
    app_secret: "..."

  - id: "sc_us_key2"
    quota_group: "sc_us"
    app_key: "..."
    app_secret: "..."
```

---

## 串行 vs 并发 — 一眼判断

```
官方文档 bucket = 1  →  串行 + interval 等待
                         不能并发翻页
                         多店铺之间用 multi_interval_ms 错开

官方文档 bucket > 1  →  可并发翻页（最多 bucket 个请求同时在途）
                         令牌桶自动控速
```

**并发只发生在以下场景：**
- 不同 path（各自有独立桶）
- 不同 quota_group（各自有独立桶）
- 同一 path + bucket > 1（桶允许并发）

---

## 加接口速查清单

```
□ 打开领星官方接口文档，找到目标接口页
□ 抄格 1 — path + method
□ 抄格 2 — 请求参数表（名、类型、必填）
□ 看格 3 — Response，确认唯一键字段
□ 抄格 4 — 限流区块（bucket / interval / multi_interval / dimension）
□ 按格 5 — 建表（字段名不翻译，PRIMARY KEY = 幂等键）
□ 执行迁移 SQL
□ 在 config.yaml 加一段 endpoint
□ make build && supervisorctl restart lingxing-sync
□ curl -X POST .../api/sync/{name}  手动触发验证
□ 确认 sync_tasks 状态 = success
```

若接入的是正式报告，还必须检查：报告原始表与 API 原始表物理分离；解析/对账失败不会更新 `listing_daily_metrics`；成功报告值只在同字段上优先，API 原始值保持不变。

详细步骤见 [07-add-endpoint.md](07-add-endpoint.md)。
