# 领星同步机 — HTTP API 规范（宪法层）

> 后端 Go 服务提供 REST API，供前端 Alpine.js 调用；已授权的数据集发布也由同一 Go 进程提供。所有接口返回 JSON，路径前缀 `/api`。面向内部消费者的数据集端点必须经 Nginx HTTPS 暴露。

---

## 公共响应格式

```json
{ "ok": true,  "data": {...} }
{ "ok": false, "error": "错误说明" }
```

HTTP 状态码：`200` = ok；`400` = 参数错误；`500` = 内部错误。

---

## 接口清单

### GET /api/status
返回所有 Worker 当前状态，用于仪表盘实时刷新（前端每 5 秒轮询）。

**响应**
```json
{
  "ok": true,
  "data": {
    "workers": [
      {
        "name": "sc_inventory",
        "display": "SC FBA 库存",
        "account_id": "sc_us",
        "status": "idle",           // idle | running | error | disabled
        "last_run_at": "2026-08-06T10:30:00Z",
        "last_status": "success",
        "last_records": 1234,
        "next_run_at": "2026-08-06T10:40:00Z",
        "today_records": 5678,
        "today_errors": 0
      }
    ],
    "summary": {
      "total": 5,
      "running": 1,
      "error": 0,
      "disabled": 0
    }
  }
}
```

---

### POST /api/sync/:name
手动触发指定接口立即同步一次。

**路径参数**：`name` = endpoint 标识（如 `sc_inventory`）

**请求体**（可选）
```json
{
  "force": true,
  "store_sids": ["store-1"],
  "date_from": "2026-08-01",
  "date_to": "2026-08-03"
}
```

`store_sids` 只对按店铺接口生效。`date_from` 与 `date_to` 必须同时传入，格式为
`YYYY-MM-DD` 且开始日期不晚于结束日期；它们只覆盖本次手动任务，不会修改接口默认配置。
只有已声明日期范围合同的接口接受范围；快照接口传日期返回 `400`。单日接口只能传同一天，
映射到其配置的 `date_field`；单日接口的跨日同步需另行按日执行。

**响应**
```json
{ "ok": true, "data": { "message": "任务已加入队列，请在同步日志查看结果: sc_inventory", "endpoint": "sc_inventory", "queued": true } }
```

---

### POST /api/sync/:name/cancel
取消正在运行的任务（发信号，最终态由 Worker 决定）。

**响应**
```json
{ "ok": true, "data": { "message": "取消信号已发送" } }
```

---

### GET /api/tasks
查询同步任务历史列表。

**查询参数**
| 参数 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `endpoint` | string | - | 过滤接口名 |
| `status` | string | - | pending/running/success/error/cancelled |
| `date_from` | string | - | ISO8601 日期，含 |
| `date_to` | string | - | ISO8601 日期，含 |
| `page` | int | 1 | 页码 |
| `page_size` | int | 20 | 每页数，最大 100 |

**响应**
```json
{
  "ok": true,
  "data": {
    "items": [
      {
        "id": 456,
        "endpoint": "sc_inventory",
        "display": "SC FBA 库存",
        "account_id": "sc_us",
        "status": "success",
        "trigger_type": "cron",
        "started_at": "2026-08-06T10:30:00Z",
        "finished_at": "2026-08-06T10:31:23Z",
        "duration_sec": 83,
        "records_upserted": 1234,
        "pages_fetched": 13,
        "error_message": null
      }
    ],
    "total": 200,
    "page": 1,
    "page_size": 20
  }
}
```

---

### GET /api/tasks/:id/logs
查询某次任务的逐页请求日志。

**响应**
```json
{
  "ok": true,
  "data": {
    "task_id": 456,
    "logs": [
      {
        "page": 1,
        "http_status": 200,
        "api_code": 0,
        "records_count": 100,
        "duration_ms": 350,
        "error_raw": null
      }
    ]
  }
}
```

---

### GET /api/endpoints
返回配置中所有 endpoint 定义，供下拉选择。

**响应**
```json
{
  "ok": true,
  "data": [
    { "name": "sc_inventory", "display": "SC FBA 库存", "account_id": "sc_us", "enabled": true }
  ]
}
```

---

### POST /api/reconcile
上传报表文件，与数据库比对。Content-Type: `multipart/form-data`。

**表单字段**
| 字段 | 说明 |
|---|---|
| `file` | CSV 或 XLSX 文件 |
| `endpoint` | 接口名（用于确定比对表和主键字段）|
| `account_id` | 账号 ID |

**响应**（同步执行，文件较小时直接返回结果）
```json
{
  "ok": true,
  "data": {
    "matched": 980,
    "missing_in_db": 5,
    "extra_in_report": 2,
    "diffs": [
      { "record_id": "xxx", "field": "quantity", "db_value": "10", "report_value": "12" }
    ]
  }
}
```

---

### GET /api/settings
返回系统设置和连接状态。

**响应**
```json
{
  "ok": true,
  "data": {
    "version": "1.0.0",
    "uptime_sec": 3600,
    "db_connected": true,
    "accounts": [
      { "id": "sc_us", "name": "自营-美国", "token_valid": true, "token_expires_in_sec": 7000 }
    ]
  }
}
```

---

### POST /api/settings/reload
应用配置变更。**热加载语义**：
- **可热加载**（不重启，就下次触发生效）：已有接口的 `enabled` / `cron` / `rate` / `window_days` / `date_offset_days` / `extra_params` / `store_sids`。
- **需重启**（结构性变更）：增删账号、增删接口、改 `path` / `method` / `table` / `database.*`。

```json
{ "ok": true, "data": { "message": "配置已热加载", "need_restart": false } }
{ "ok": true, "data": { "message": "结构性变更已保存，需重启生效", "need_restart": true } }
```

---

### POST /api/settings/restart
优雅重启进程（`syscall.Exec` 原地替换，PID 不变；宝塔 Supervisor 无感）。用于结构性变更后生效。

```json
{ "ok": true, "data": { "message": "正在重启…" } }
```

---

### POST /api/settings/test-db
测试数据库连接。

```json
{ "ok": true,  "data": { "latency_ms": 2 } }
{ "ok": false, "error": "dial tcp 127.0.0.1:3306: connection refused" }
```

---

### POST /api/settings/test-connection?account=:id
用指定账号真实取一次 token，验证 app_key/app_secret 是否可用。

```json
{ "ok": true,  "data": { "token_valid": true, "expires_in_sec": 7200 } }
{ "ok": false, "error": "lingxing token: api code=... msg=..." }
```

---

## 配置读写接口（受 X-Sync-Secret 中间件保护）

> 以下接口支持 UI 增删改配置。所有写操作：解析 body → 改配置快照 → 校验 → 备份 `config.yaml.bak`
> → 原子写 config.yaml → 判定热加载/重启。返回体统一带 `need_restart` 标志。

### GET /api/config
返回完整配置供 UI 编辑。**app_secret 脱敏**（`abcd****wxyz`），绝不明文回传。

```json
{
  "ok": true,
  "data": {
    "accounts": [
      { "id": "sc_us", "name": "自营-美国", "quota_group": "sc_us", "app_key": "ak_123", "app_secret": "abcd****wxyz", "connection_check": { "cron": "*/20 * * * *", "enabled": true } }
    ],
    "endpoints": [ { "name": "sc_stores", "display": "SC 店铺列表", "account": "sc_us", "path": "...", "method": "GET", "table": "ls_stores", "record_id_fields": ["sid"], "rate": {...}, "cron": "...", "enabled": true, "store_sids": [], "window_days": 0, "window_start_field": "", "window_end_field": "", "date_field": "", "date_offset_days": 0, "date_range_capable": false } ]
  }
}
```

每个 endpoint 还可能带两个**只读运行态**字段（取自 worker 快照，健康接口两者都不出现）：

| 字段 | 含义 |
|---|---|
| `fatal_error` | 非空 = 该接口启动断言未过（最常见：目标表没建），永久不可同步。`/sync` 页据此整行标红并显示原因；手动触发返回 409。 |
| `warnings[]` | 不阻断同步但需人看见的问题，当前唯一来源：配置声明的列（`record_id_fields` / `field_paths` 目标）在目标表里不存在，这些字段会被 Upsert 静默丢弃。 |

两者**永远写不回 `config.yaml`**：它们在 DTO 里只为满足前端整行回传（`saveRow` 用 `Object.assign` 回传全部键，而 PUT 侧开了 `DisallowUnknownFields`，字段不在 DTO 就会 400），后端 `dtoToEndpoint` 故意不映射它们。

### POST /api/accounts
新增账号。body = 单个 account 对象（含 app_secret 明文）。id 全局唯一，否则 400。

### PUT /api/accounts/:id
更新账号。app_secret 传空串则保留原值（避免脱敏值覆盖真值）。

### PUT /api/accounts/:id/connection-check
保存该账号的自动测试连接/Token 续租计划。body 为 `{ "cron": "*/20 * * * *", "enabled": true }`；Cron 必须为合法 5 段表达式，成功后热重建调度。

### POST /api/accounts/:id/stores/sync
触发该账号**全部** `is_store_source: true` 店铺目录接口，逐个独立入队。SC 与 VC 店铺来自两个不同上游接口却写同一张 `ls_stores`，必须同时触发才能刷全目录，故不限制「唯一」。

单个接口不可用时**只跳过它**，其余照常入队（对应 CLAUDE.md §1.8「缺表/缺列只炸自己」）；跳过原因：`未就绪` / `已禁用` / `目标表未就绪`（启动断言未过）/ `运行中`。仅当该账号**一个店铺来源接口都没有**时返回 409。接口 Cron、限流与启停仍由 `/sync` 管理。

```json
{ "ok": true, "data": {
    "message": "店铺目录刷新已加入队列: sc_stores；跳过: vc_stores(目标表未就绪)",
    "endpoints": ["sc_stores"],
    "queued": true } }
{ "ok": false, "error": "该账号未配置店铺目录接口: sc_us" }
```

### DELETE /api/accounts/:id
删除账号。**若仍有 endpoint 引用该账号 → 409 拦截**，返回引用列表，不级联删除。

```json
{ "ok": false, "error": "账号 sc_us 仍被 2 个接口引用，请先删除接口: [sc_stores, sc_inventory]" }
```

### POST /api/endpoints
新增接口。body = 单个 endpoint 对象。name 全局唯一、account 必须存在、table 必须已建表，否则 400。

### PUT /api/endpoints/:name
更新接口（改 path/method/table 属结构性变更，返回 need_restart:true）。`window_days` 与
`date_offset_days` 属热加载字段；前者调整范围接口的滚动窗口，后者调整单日接口的 T-N 日期。

### DELETE /api/endpoints/:name
删除接口（停止其调度，不删已同步数据）。

### GET /api/datasources/:table/columns
读目标表真实列结构（替换旧占位）。

```json
{
  "ok": true,
  "data": {
    "table": "ls_stores",
    "columns": [
      { "name": "account_id", "type": "varchar(32)", "is_primary": true },
      { "name": "sid", "type": "varchar(32)", "is_primary": true },
      { "name": "store_name", "type": "varchar(128)", "is_primary": false }
    ]
  }
}
```

## 版本化只读数据集 API（已实现）

当前唯一数据集标识是 `listing-daily-v1`，只开放以下两个固定 `POST` 端点：

```text
POST /api/v1/datasets/listing-daily-v1/snapshot
POST /api/v1/datasets/listing-daily-v1/changes
```

不得实现 `/:table`、自定义 path、SQL、排序表达式或任意字段表达式入口。新增数据集必须先修改宪法 allowlist，不能仅靠数据库中出现一张表自动暴露。

### 认证与 scope

- 使用 `Authorization: Bearer <project-token>`；服务端只保存明文 token 的小写 SHA-256，不得复用领星 OpenAPI token、ERP `auth-token` 或 UI 的 `X-Sync-Secret`。
- 每个 token 固定绑定一个 `project_id`，每个项目当前只登记一个自动生成的 `token_id`；token 必须同时具备 `listing-daily-v1` dataset scope 和请求店铺的 store scope。业务字段由数据表统一定义，不按项目裁剪。
- 请求超出 dataset、store 或字段 scope 返回 `403`/`400`，不能静默裁剪为部分结果。token 已撤销或过期返回 `401`。
- 消费者只经 HTTPS 调用；不得直连 MySQL、提交远程 SQL 或从 7799 明文公网访问。

### `snapshot`

按店铺和业务日期范围返回当前日维快照。请求参数全部放 JSON body：

```json
{
  "store": "store-a",
  "date_from": "2026-08-01",
  "date_to": "2026-08-07",
  "fields": ["sales_units", "sales_amount", "inventory_sellable"],
  "page_size": 100,
  "cursor": "上一页返回的 next_cursor，可省略"
}
```

- `store`、`date_from`、`date_to` 必填；日期格式固定为 `YYYY-MM-DD`，范围受 `dataset_api.max_date_span_days` 限制。
- `fields` 可省略；省略时返回该数据表全部已登记业务字段。传入时每项必须属于该数据表的字段目录。
- `page_size` 可省略，默认 100，且不得超过 `dataset_api.max_page_size`。
- `cursor` 只用于继续读取同一次 snapshot；它绑定 token、店铺和日期范围，不能跨 scope 或改作 changes cursor。
- 最后一页返回独立的 `changes_cursor`，消费者保存它，后续交给 `changes` 获取此次快照之后的变化。

### `changes`

返回 `changes_cursor` 之后 LingxingSync **已经写入**的日维变化。请求参数同样全部放 JSON body：

```json
{
  "store": "store-a",
  "fields": ["sales_units", "sales_amount", "inventory_sellable"],
  "page_size": 100,
  "cursor": "snapshot 最后一页或上一页 changes 返回的 cursor"
}
```

- `store`、`cursor` 必填；`changes` 不接受日期范围。
- 分页使用服务端签名的不透明 keyset cursor，排序键固定为事实行 `updated_at + listing_dimension_id + business_date`，禁止 offset 分页。
- 每次成功都返回可保存的 `next_cursor`：有变化时推进到本页最后一行；空页保持输入 cursor。下一次请求原样回传，不解析、不修改 cursor。
- `listing_daily_metrics.updated_at` 是本项目日维事实行每次 INSERT/UPDATE 自动刷新的内部发布时间，不是领星接口字段；微秒精度用于避免同一秒多次更新被严格游标漏掉。
- `changes` 不能发现领星侧尚未被本项目重拉的历史修正，也不能替代各 endpoint 的更新时间增量、日期重拉或正式报告对账。

### 固定响应

```json
{
  "ok": true,
  "data": {
    "schema_version": "listing-daily-v1",
    "rows": [
      {
        "account_id": "",
        "store": "store-a",
        "channel": "SC",
        "asin": "B000000001",
        "sku": "SKU-1",
        "business_date": "2026-08-01",
        "updated_at": "2026-08-12T10:20:30.123456Z",
        "is_provisional": true,
        "verification_status": "provisional",
        "deleted_at": null,
        "sales_units": 3
      }
    ],
    "next_cursor": "changes 每次成功返回；snapshot 仅有下一页时返回",
    "changes_cursor": "仅 snapshot 最后一页返回",
    "has_more": false
  }
}
```

当前日维 schema 尚无 `account_id` 和 `deleted_at` 来源，响应必须如实返回空值/`null`，不得猜补来源或删除标记。

### 值语义

- `listing_daily_metrics` 粒度固定为 store/channel/ASIN/SKU/business-date。
- 成功解析、对账后的正式报告字段值优先；没有报告覆盖时可返回 API 暂定值，且两份原始证据都不被修改。
- 来源覆盖未知时返回 `null`/未验证状态，不得填 `0`。
- 无 ASIN/SKU 的 HSA 只可返回店铺级记录，或返回带明确 `allocated` 标识的分摊记录。
- PO 等不同粒度域不从这两个 listing 端点返回。

## 数据集字段 allowlist 管理（已实现）

管理员可在“新增项目”切卡创建 `listing-daily-v1` 的读取凭证：`POST /api/datasources/datasets/listing-daily-v1/projects`，请求只需 `project_id` 和非空 `store_scopes`。系统独立生成稳定的 `token_id`，并自动给新项目绑定该数据表的全部已登记业务字段。响应只返回一次随机明文 `token`，配置文件仅保存其 SHA-256 `token_hash`，并返回 `need_restart: true`；明文不进入日志或后续 GET。没有已验证字段时创建会明确失败。

独立 `/dataset-fields` 页面使用以下固定管理端点，仍受 `X-Sync-Secret` 中间件保护：

```text
GET /api/datasources/datasets/listing-daily-v1/fields
```

字段管理属于现有管理面，继续受 `X-Sync-Secret` 中间件保护，不使用消费者 Bearer token：

- 无查询参数的 `GET` 返回数据表 `dataset_id`、`dataset_name`、不可删除的 `fixed_fields`、全部 `available_fields`，以及项目/Token/店铺范围列表；没有任何项目时固定返回 `projects: []`，不能省略字段。
- 数据表字段不随项目变化。项目只标识读取方、Token 与可读取店铺范围，不能修改表字段、表名、SQL、路径或 token hash。

该设置只描述 `listing-daily-v1` 已映射的指标字段，绝不能创建、修改、删除 MySQL 表或列，也不能接受表名、SQL 或动态 path。

## 正式报表检验管理（已实现）

`/sync` 的“报表检验”二级切卡使用固定管理端点，仍受 `X-Sync-Secret` 中间件保护：

```text
GET /api/report-exports/config
PUT /api/report-exports/config
GET /api/report-exports/status?type={report_type}&account={account_id}&store_id={store_id}
```

- `GET config` 返回 `report_exports[]`、`available_types[]` 和空配置默认值；当前 `available_types` 固定为已实现的 `fba_customer_returns`、`fba_customer_shipment_sales`。
- `PUT config` 一次提交完整 `report_exports[]`。每项粒度固定为 `type + account + store_id`，重复项整批拒绝；写盘和现有 Scheduler 热重建沿用配置保存原子边界。
- Seller ID、store ID 和 Marketplace ID 由 UI 从已选账号的本地店铺资料带入；服务端仍按正式配置合同校验，缺失时不得猜值。
- `GET status` 按 `type + account + store_id` 返回指定报表最近一次任务及其三类对账差异；旧调用省略 `type` 时只兼容 Customer Returns。临时下载 URL、签名凭证、token 或 hash 不得返回。
- 新报表类型只有在创建、查询、下载、解析、原始表和对账合同均实现并测试后，才能加入 `available_types`；管理 API 不接受动态 report type 或表名。
