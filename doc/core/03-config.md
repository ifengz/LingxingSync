# 领星同步机 — 配置文件规范（宪法层）

> `config.yaml` 是唯一配置入口。不进 git（gitignore），`config.example.yaml` 进 git。
>
> **可人手改，也可由 UI 读写。** UI 保存时整体重写 `config.yaml` 并先生成 `config.yaml.bak`
> （原子写：临时文件 + rename）。Go 序列化不保留注释，故教学注释以带注释的
> `config.example.yaml` 为准，`config.yaml` 只存生效值。写入前一律走 config.validate 校验，
> 校验失败不落盘。

---

## 完整示例

```yaml
# ===== 服务配置 =====
server:
  port: 7799                     # UI 和 API 端口；宝塔 Nginx 反代到这里
  secret: "change_this_key"      # 管理界面登录密钥（未来可扩展）

# ===== 数据库 =====
database:
  host: "127.0.0.1"
  port: 3306
  user: "lingsync_rw"            # 读写账号
  password: "your_password"
  db: "lingsync"
  max_open: 20
  max_idle: 5
  conn_timeout_sec: 10

# ===== 领星账号（可多个，按 app_key 分组）=====
accounts:
  - id: "sc_us"                  # 本系统内部 ID，全局唯一，写入 account_id 列
    name: "自营-美国"
    quota_group: "sc_us"         # 限流分组（默认 = id；同公司多 app_key 设相同值共享配额）
    app_key: "your_app_key_sc"
    app_secret: "your_app_secret_sc"
    connection_check:             # 账号级 OpenAPI 连通性检查与 Token 主动续租
      cron: "*/20 * * * *"
      enabled: true

  - id: "vc_de"
    name: "联营-德国"
    quota_group: "vc_de"
    app_key: "your_app_key_vc"
    app_secret: "your_app_secret_vc"
    connection_check:
      cron: "*/20 * * * *"
      enabled: true

# ===== 接口同步任务 =====
endpoints:
  # 店铺来源接口（is_store_source=true，启动时优先同步）
  - name: sc_stores
    display: "SC 店铺列表（基础）"
    account: "sc_us"
    path: "/erp/sc/data/seller/lists"  # 从领星文档"API Path"原样抄
    method: "GET"                       # 从领星文档"请求方式"原样抄
    table: "ls_stores"
    record_id_fields: ["sid"]
    is_store_source: true               # 标记：启动时优先同步，供其他接口读取
    rate:
      bucket: 5
      interval_ms: 200
      multi_interval_ms: 0
      dimension: "account+path"
    cron: "0 */6 * * *"
    enabled: true
    window_days: 0

  - name: sc_sales_orders        # 任务标识，全局唯一，字母下划线
    display: "SC 销售订单"        # UI 展示名
    account: "sc_us"             # 对应 accounts[].id
    path: "/openapi/erp/sc/orders/list"
    method: "POST"
    table: "ls_sales_orders"     # 写入的目标表
    record_id_fields: ["order_id"]  # 唯一键字段（复合主键用数组）
    rate:                        # 从领星官方文档 Rate Limit 区块原样抄写
      bucket: 5                  # 令牌桶容量（=1 时强制串行）
      interval_ms: 200           # 单店铺调用最小间隔
      multi_interval_ms: 1000    # 多店铺调用最小间隔
      dimension: "account+path"  # 限流维度（领星按此维度共享配额）
    cron: "*/10 * * * *"         # 标准 5 段 cron（服务器本地时间）
    enabled: true
    window_days: 7               # 每次拉取过去 N 天数据（滚动窗口）
    extra_params:                # 接口特有参数（透传给 API）
      type: 1                    # FBA=1, FBM=2

  - name: sc_inventory
    display: "SC FBA 库存"
    account: "sc_us"
    path: "/openapi/erp/sc/inventory/list"
    method: "POST"
    table: "ls_inventory"
    record_id_fields: ["fnsku"]
    rate:
      bucket: 1                  # 桶=1 → 串行：请求返回后才能发下一个
      interval_ms: 1000
      multi_interval_ms: 10000
      dimension: "account+path"
    cron: "0 */2 * * *"          # 每 2 小时
    enabled: true
    window_days: 0               # 0 = 全量（不带日期参数）
    # iterate_by_store: true     # 若按店铺迭代，可用 store_sids 限定范围：
    # store_sids: ["1001","1002"] # 空/省略 = 该账号全部店铺；非空 = 只同步这些 sid

  - name: sc_ads_daily
    display: "SC 广告日报"
    account: "sc_us"
    path: "/openapi/erp/sc/ads/daily"
    method: "POST"
    table: "ls_ads_daily"
    record_id_fields: ["report_id"]
    rate:
      bucket: 3
      interval_ms: 333
      multi_interval_ms: 2000
      dimension: "account+path"
    cron: "30 1 * * *"           # 每天 01:30
    enabled: true
    window_days: 14

# ===== 留存 =====
retention:
  task_logs_days: 90             # sync_task_logs 保留天数
  tasks_days: 365                # sync_tasks 保留天数
  cleanup_cron: "0 3 * * *"     # 每天 03:00 清理
```

---

## 字段说明

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `server.port` | int | 是 | HTTP 监听端口，默认 7799 |
| `database.*` | 对象 | 是 | MySQL 连接参数 |
| `accounts[].id` | string | 是 | 全局唯一，写入每条数据的 account_id 列 |
| `accounts[].app_key/app_secret` | string | 是 | 领星 OpenAPI 凭证 |
| `accounts[].quota_group` | string | 否 | 限流分组；默认 = id；同公司多 key 设同值共享桶 |
| `endpoints[].name` | string | 是 | 任务标识，字母+下划线，全局唯一 |
| `endpoints[].account` | string | 是 | 必须匹配某个 `accounts[].id` |
| `endpoints[].path` | string | 是 | 领星接口路径（从文档"API Path"原样抄） |
| `endpoints[].method` | string | 是 | `GET` 或 `POST`（从文档"请求方式"原样抄） |
| `endpoints[].table` | string | 是 | 目标数据表名（必须已建表） |
| `endpoints[].record_id_fields` | array | 是 | 唯一键字段数组（单键 `["field"]`；复合键 `["f1","f2"]`）|
| `endpoints[].rate.bucket` | int | 是 | 令牌桶容量（领星文档原值；=1 时强制串行） |
| `endpoints[].rate.interval_ms` | int | 是 | 单店铺调用最小间隔（毫秒） |
| `endpoints[].rate.multi_interval_ms` | int | 是 | 多店铺调用最小间隔（毫秒；无多店铺场景填 0） |
| `endpoints[].rate.dimension` | string | 是 | 限流维度（通常 `account+path`，从领星文档抄） |
| `endpoints[].cron` | string | 是 | 5 段标准 cron，服务器本地时区 |
| `endpoints[].window_days` | int | 否 | 0 = 全量；>0 = 滚动 N 天窗口 |
| `endpoints[].extra_params` | map | 否 | 透传给领星 API 的附加参数 |
| `endpoints[].enabled` | bool | 否 | false 时暂停调度（不删数据） |
| `endpoints[].is_store_source` | bool | 否 | true = 店铺来源接口，启动时优先同步 |
| `endpoints[].iterate_by_store` | bool | 否 | true = Worker 对每个 sid 循环一次（需配合 `is_store_source` 接口） |
| `endpoints[].store_param_name` | string | 否 | 迭代时注入店铺ID 的参数名，默认 `sid` |
| `endpoints[].store_sids` | array | 否 | 店铺白名单；空 = 同步该账号全部 sid，非空 = 只同步列出的 sid。仅对 `iterate_by_store: true` 生效 |
| `retention.task_logs_days` | int | 否 | 默认 90 |
