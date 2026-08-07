# 领星同步机 — HTTP API 规范（宪法层）

> 后端 Go 服务提供 REST API，供前端 Alpine.js 调用。所有接口返回 JSON，路径前缀 `/api`。

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
        "name": "sc_sales_orders",
        "display": "SC 销售订单",
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

**路径参数**：`name` = endpoint 标识（如 `sc_sales_orders`）

**请求体**（可选）
```json
{ "force": true }    // force=true：忽略 enabled=false，强制触发
```

**响应**
```json
{ "ok": true, "data": { "task_id": 123 } }
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
        "endpoint": "sc_sales_orders",
        "display": "SC 销售订单",
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
    { "name": "sc_sales_orders", "display": "SC 销售订单", "account_id": "sc_us", "enabled": true }
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
- **可热加载**（不重启，就下次触发生效）：已有接口的 `enabled` / `cron` / `rate` / `window_days` / `extra_params` / `store_sids`。
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
      { "id": "sc_us", "name": "自营-美国", "quota_group": "sc_us", "app_key": "ak_123", "app_secret": "abcd****wxyz" }
    ],
    "endpoints": [ { "name": "sc_stores", "display": "SC 店铺列表", "account": "sc_us", "path": "...", "method": "GET", "table": "ls_stores", "record_id_fields": ["sid"], "rate": {...}, "cron": "...", "enabled": true, "store_sids": [] } ]
  }
}
```

### POST /api/accounts
新增账号。body = 单个 account 对象（含 app_secret 明文）。id 全局唯一，否则 400。

### PUT /api/accounts/:id
更新账号。app_secret 传空串则保留原值（避免脱敏值覆盖真值）。

### DELETE /api/accounts/:id
删除账号。**若仍有 endpoint 引用该账号 → 409 拦截**，返回引用列表，不级联删除。

```json
{ "ok": false, "error": "账号 sc_us 仍被 3 个接口引用，请先删除接口: [sc_stores, sc_inventory, sc_ads_daily]" }
```

### POST /api/endpoints
新增接口。body = 单个 endpoint 对象。name 全局唯一、account 必须存在、table 必须已建表，否则 400。

### PUT /api/endpoints/:name
更新接口（改 path/method/table 属结构性变更，返回 need_restart:true）。

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
