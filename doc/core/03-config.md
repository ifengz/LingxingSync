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

  - name: sc_inventory
    display: "SC FBA 库存"
    account: "sc_us"
    path: "/erp/sc/routing/fba/fbaStock/fbaList"
    method: "GET"
    table: "ls_fba_inventory"
    record_id_fields: ["sid", "fnsku"]   # 同一 fnsku 多店铺各一行，必须带 sid
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

  # 注：这里原有一段 sc_ads_daily「SC 广告日报」示例，已删除。
  # 其 path "/openapi/erp/sc/ads/daily" 与唯一键 report_id 都不存在于领星 OpenAPI
  # （凭空写的），照抄即 404。领星真实广告报表是一族接口（见 08-api-reference.md §6.5），
  # 且需先取 profile_id，要接须按 07-add-endpoint.md 逐个 probe、各建一张表。

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
| `accounts[].id` | string | 是 | slug 规范（见下）；**大小写不敏感全局唯一**，写入每条数据的 account_id 列 |
| `accounts[].app_key/app_secret` | string | 是 | 领星 OpenAPI 凭证 |
| `accounts[].quota_group` | string | 否 | 限流分组；默认 = id；同公司多 key 设同值共享桶 |
| `endpoints[].name` | string | 是 | 任务标识，字母+下划线，全局唯一 |
| `endpoints[].account` | string | 是 | 必须匹配某个 `accounts[].id` |
| `endpoints[].path` | string | 是 | 领星接口路径（从文档"API Path"原样抄） |
| `endpoints[].method` | string | 是 | `GET` 或 `POST`（从文档"请求方式"原样抄） |
| `endpoints[].table` | string | 是 | 目标数据表名（必须已建表） |
| `endpoints[].record_id_fields` | array | 是 | 唯一键字段数组（单键 `["field"]`；复合键 `["f1","f2"]`）|
| `endpoints[].response_shape` | string | 否 | `list`（默认，分页列表）或 `object`（`data` 为单个业务对象时包装为一行） |
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
| `endpoints[].iterate_by_vc_orders` | bool | 否 | 仅用于已验证的 VC PO detail：按同账号 `ls_vc_orders.gmt_modified` 窗口读取 `vc_store_id/local_po_number`，逐单请求；必须为 POST、`response_shape: object`、`window_days > 0`，并强制回注两个身份字段。不得与其他迭代模式同时启用 |
| `endpoints[].store_param_name` | string | 否 | 迭代时注入店铺ID 的参数名，默认 `sid` |
| `endpoints[].store_type` | string | 否 | 限定 `iterate_by_store` 只迭代该类型店铺：`SC` / `VC` / 空（不过滤）。对齐 `ls_stores.store_type`。SC 接口喂 VC 店铺 sid 会拉到错数据，故 SC 迭代接口应填 `SC` |
| `endpoints[].store_sids` | array | 否 | 店铺白名单；空 = 同步该账号全部 sid，非空 = 只同步列出的 sid。仅对 `iterate_by_store: true` 生效 |
| `retention.task_logs_days` | int | 否 | 默认 90 |

---

## 账号 ID 规范（slug + 大小写不敏感唯一，参考 GitHub）

`accounts[].id` 是写入每张原始表 `account_id` 列的机器标识符，用于隔离不同领星账号的数据，因此有硬约束：

1. **字符集（slug）**：只允许 `[A-Za-z0-9_-]`，首尾必须是字母或数字，长度 1–32（对齐 `account_id VARCHAR(32)` 列宽）。校验正则见 `internal/config/config.go` 的 `accountIDPattern`。
2. **大小写不敏感全局唯一**：判重以 `NormID(id) = ToLower(TrimSpace(id))` 为准（参考 GitHub username 规则）。`sc_us` 与 `Sc_us` 归一化后相同，视为撞名，`validate()` 直接 fail-loud。DB `account_id` 列排序规则本就是 `*_ci`，此口径与存储层一致。
3. **查找也大小写不敏感**：`FindAccount` 用 `NormID` 匹配，URL/API 传 `SC_US` 也能命中 `sc_us`。因规则 2 保证不存在仅大小写不同的 ID，归一化匹配至多命中一个，无歧义。
4. **新增账号自动配 ID**：`POST /api/accounts` 时，若填入 ID 与现有账号大小写不敏感撞名，后端以它为 base 自动往后找可用的 `base_2`/`base_3`… 落定（如 `sc_us` 占用 → 配 `sc_us_2`），响应回显最终 `account_id`。系统自动区分，不靠人眼防撞。

> 这套规则是对早期「仅大小写不同的两个账号」历史脏账的收敛（迁移见 `migrations/009_rename_account.sql`），也是「加账号极简单、不伤架构」原则在账号维度的落地。
