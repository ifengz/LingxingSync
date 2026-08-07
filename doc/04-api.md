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
重新加载 config.yaml（热加载非结构性变更，如 enabled/cron 修改）。

```json
{ "ok": true, "data": { "message": "配置已重载，Worker 将在下次触发时生效" } }
```

---

### POST /api/settings/test-db
测试数据库连接。

```json
{ "ok": true,  "data": { "latency_ms": 2 } }
{ "ok": false, "error": "dial tcp 127.0.0.1:3306: connection refused" }
```
