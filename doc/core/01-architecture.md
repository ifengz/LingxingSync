# 领星同步机 — 架构设计（宪法层）

> 一句话：一个 Go 服务，按桶令牌把领星 OpenAPI 数据定时拉下来落库，给 polabel2 等项目当只读数据底座。

---

## 1. 六条设计原则（直接对应用户需求）

> 这六条是**目标层**。`CLAUDE.md §1` 在此之上还有 7–11「写码红线」（简洁优先 / 缺表列只炸自己 / 代码最短 / 少嵌套 / 写完核对宪法）与 12–14「工程纪律」（禁止空壳 / 动作≠验证 / 宪法层独立 commit），以及一张**规模上限表**。冲突时以 `CLAUDE.md` 为准。

| # | 需求 | 实现 |
|---|---|---|
| 1 | 各接口独立，互不影响 | 每个「账号+接口」一个独立 goroutine；panic 只 recover 自身，不传播 |
| 2 | 根据令牌桶控速 | 每个 `(quota_group, path)` 一个 `rate.Limiter`，进程内；桶容量=1 时串行，>1 时可并发翻页 |
| 3 | 加新接口极简单 | 配置文件加一段 YAML + 建一张表 + 重启，零代码改动 |
| 4 | 报表数据校验 | 独立 ReconciliationWorker；上传 CSV → 比对 DB → 输出差异 |
| 5 | 轻量，不吃死服务器 | 单进程，goroutine 2KB 栈，10 Worker < 50 MB；无外部队列 |
| 6 | 结构化表，polabel2 直读 | 每接口一张结构化表，列名 = 领星字段名；只读账号直连 |

---

## 2. 为什么选 Go

| | Go | Python（原型） | Rust |
|---|---|---|---|
| 部署 | **单二进制，零依赖** | 需 Python + venv | 单二进制 |
| 并发 IO | goroutine，2KB 栈 | asyncio 可用 | tokio，复杂 |
| 开发速度 | 快 | 快 | 慢（所有权学习曲线） |
| 宝塔兼容 | SSH build → supervisor | 需 Python 环境 | SSH build，编译慢 |
| IO 性能 | 足够（瓶颈是网络/DB） | 足够 | 和 Go 相当 |

Rust 的性能优势只在 CPU 密集型场景。同步领星是 IO 密集型（90% 时间等网络），Go 和 Rust 无差别。

---

## 3. 系统架构图

```
┌──────────────────── lingxing-sync 进程（单进程）────────────────────┐
│                                                                       │
│  HTTP Server :7799                                                    │
│  ├─ GET  /              → 仪表盘（HTML + Alpine.js，embed 进二进制）  │
│  ├─ GET  /api/status    → 所有 Worker 状态 JSON                      │
│  ├─ POST /api/sync/:ep  → 触发手动同步                               │
│  └─ 详见 04-api.md                                                   │
│                                                                       │
│  Scheduler                                                            │
│  └─ robfig/cron → 按配置 spec 向 Worker 发触发信号                   │
│                                                                       │
│  EndpointWorker × N goroutine（每「账号+接口」一个）                 │
│  ├─ TokenHolder[app_key]  进程级单飞 token 刷新（同 key 共用）       │
│  ├─ rate.Limiter[(quota_group,path)]  桶令牌限流；bucket=1 → 串行                                 │
│  └─ 主循环：等触发 → 限流 → 分页拉 → 写表 → 更新状态               │
│                                                                       │
│  ReconciliationWorker（独立 goroutine，可选）                        │
│  └─ 接收 CSV 上传 → 比对 DB → 返回差异                              │
│                                                                       │
└───────────────────────────────────────────────────────────────────────┘
         │ MySQL 只读账号
         ▼
   polabel2 / 其他项目（直连读表，无需 API 层）
```

---

## 4. 项目目录结构

```
lingxing-sync/
├── cmd/main.go                 # 入口：加载配置 → 起 Worker → 起 HTTP Server
├── internal/
│   ├── config/config.go        # Config struct + YAML 加载 + 校验
│   ├── api/
│   │   ├── client.go           # 领星 HTTP client（AES 签名 + 超时 + 重试）
│   │   └── token.go            # TokenHolder：按 app_key 单飞刷新
│   ├── worker/
│   │   ├── worker.go           # EndpointWorker goroutine 主循环
│   │   └── registry.go         # 全局 worker 注册表（供 API 查状态）
│   ├── db/
│   │   ├── pool.go             # MySQL 连接池（sqlx）
│   │   └── tasks.go            # sync_tasks / sync_task_logs CRUD
│   └── server/
│       ├── server.go           # HTTP server（go:embed 静态文件）
│       └── handlers.go         # REST API handler
├── web/
│   ├── templates/              # Go html/template 文件（见 05-ui.md）
│   └── static/app.js           # Alpine.js 逻辑（CDN Tailwind，无 build step）
├── migrations/
│   ├── 001_system.sql          # sync_tasks, sync_task_logs
│   └── 002_data_tables.sql     # ls_* 数据表
├── config.yaml                 # 运行配置（不进 git，gitignore）
├── config.example.yaml         # 示例配置（进 git）
├── go.mod
└── Makefile                    # make build / make run / make migrate
```

---

## 5. EndpointWorker 核心循环（伪码）

```go
func (w *EndpointWorker) Run(ctx context.Context) {
    for {
        select {
        case <-w.trigger:   // cron 或手动触发
        case <-ctx.Done():
            return
        }

        taskID := db.InsertTask(w.endpoint, "running")

        page, total := 1, 0
        var syncErr error
        for {
            w.limiter.Wait(ctx)                   // 限流器 key=(quota_group, path)，bucket=1 时串行
            resp, err := w.client.Get(w.endpoint, page, w.params)
            if err != nil || resp.Code != 0 {
                syncErr = fmt.Errorf("API error: %v %v", err, resp.Msg)
                break
            }
            if err := validateResponse(resp); err != nil {  // fail-loud 契约校验
                syncErr = err
                break
            }
            db.Upsert(w.table, resp.List)         // 写结构化数据表
            total += len(resp.List)
            if !resp.HasMore { break }
            page++
        }

        status := "success"
        if syncErr != nil { status = "error" }
        db.UpdateTask(taskID, status, total, syncErr)

        // panic recovery 在 goroutine 外层，只 recover 本 Worker
    }
}
```

**单写者原则**：只有 Worker 自己写自己的 `sync_tasks` 状态行。HTTP handler 只能 INSERT 新 pending 行或发 channel 信号，永远不 UPDATE status 列。

---

## 6. 并发控制：(quota_group, path) 维度的 rate.Limiter

领星按 **（账号维度, path）** 共享配额，同账号下所有 appId 共用一个桶。  
运行时限流器 key = `(quota_group, path)`，`quota_group` 默认等于 `accounts[].id`。

```go
// key = (quota_group, path)，而不是 endpoint 名
// 同一 (quota_group, path) 的所有 Worker 共用同一个 Limiter
key := fmt.Sprintf("%s|%s", endpoint.QuotaGroup, endpoint.Path)
limiter := getLimiter(key, endpoint.Rate.Bucket, endpoint.Rate.IntervalMs)
limiter.Wait(ctx)  // 自动等待，不忙轮询；bucket=1 时等于串行
```

| 桶容量（bucket） | 翻页模式 | 配置来源 |
|---|---|---|
| **1** | **强制串行**：请求返回后 + interval 等待，才能发下一个 | 领星接口文档 Rate Limit 区块原样抄 |
| **> 1** | **可并发翻页**：最多同时 bucket 个请求在途 | 同上 |

**并发只在以下情况发生：**
- 不同 path（各自独立 Limiter）
- 不同 quota_group（各自独立 Limiter）
- 同 (quota_group, path) 且 bucket > 1

**限流器三个优势（对比 polabel2）：**
- 无 DB 行 → 无锁序 → 不会死锁
- 进程重启 → 资源自动归零，无需 watchdog 回收
- 一个 endpoint panic → 只影响自己的 goroutine，recover 后重启

限流档案字段含义见 [09-endpoint-contract.md §格4](09-endpoint-contract.md)。

---

## 7. TokenHolder：同一 app_key 共用，单飞刷新

```go
// key = app_key，同账号的所有 Worker 共用同一个 TokenHolder
type TokenHolder struct {
    mu    sync.Mutex
    token string
    exp   time.Time
    singleFlight singleflight.Group
}

func (h *TokenHolder) Get() (string, error) {
    if h.token != "" && time.Now().Before(h.exp) {
        return h.token, nil
    }
    // 多个并发请求只发一次刷新
    v, err, _ := h.singleFlight.Do("refresh", h.refresh)
    return v.(string), err
}
```

多 Worker 同一 app_key → 不各自刷新踩踏 → 无 token 竞争。

---

## 7.5 配置写回与热加载 / 重启生命周期

> UI 可增删改 `config.yaml`（字段见 [03-config.md](03-config.md)）。写回与生效分两条路径，由「变更是否改变 Worker 集合结构」决定。

### 写回：整体重写 + 备份（原子）

```go
// internal/config/store.go
// Save 用互斥锁串行化所有写操作，避免并发写坏文件。
func (s *Store) Save(cfg *Config) error {
    s.mu.Lock()
    defer s.mu.Unlock()
    // 1. 校验新配置（复用 Validate，非法直接拒绝，不落盘）
    // 2. 备份现有 config.yaml → config.yaml.bak（保留最近一份）
    // 3. marshal 新配置 → 写临时文件 config.yaml.tmp
    // 4. os.Rename(tmp, config.yaml)  ← 原子替换，写一半崩溃不会坏原文件
}
```

**注释代价（已知）：** Go YAML marshal 不保留注释。`config.yaml` 经 UI 写一次后，教学注释丢失；完整注释永久保留在 `config.example.yaml`（进 git，不被 UI 触碰）。凭证块（app_secret / db password）由 UI 表单显式提交，GET 返回时脱敏。

### 生效：热加载 vs 重启

| 变更类型 | 判定 | 生效方式 |
|---|---|---|
| **非结构性** | 已有 endpoint 的 `enabled` / `cron` / `rate` / `window_days` / `extra_params` / `store_sids` 变化 | `POST /api/settings/reload` 热加载，不重启 |
| **结构性** | 增删 account、增删 endpoint、改 endpoint 的 `path`/`method`/`table`/`account`、改 `database.*`/`server.port` | `POST /api/settings/restart` 重启进程 |

判定逻辑集中在一处：

```go
// internal/config/store.go
// RequiresRestart 对比新旧配置，返回 true 表示必须重启才能生效。
func RequiresRestart(old, new *Config) bool {
    // account 集合变化（增删/改 app_key/app_secret/quota_group）→ true
    // endpoint 集合变化（增删）→ true
    // 任一 endpoint 的 path/method/table/account 变化 → true
    // database.* 或 server.port 变化 → true
    // 其余（enabled/cron/rate/window/extra/store_sids）→ false
}
```

### 热加载实现：更新已有 Worker，不重建集合

```go
// internal/worker/registry.go
// ApplyReload 把新配置套到已存在的 Worker 上（集合不变前提下）。
func (r *Registry) ApplyReload(cfg *config.Config) {
    // 对每个 Worker：w.UpdateEndpoint(newEp) —— 加锁替换 endpoint 快照
    // rate 变化时：重建该 (quota_group, path) 的 Limiter
    // 重新注册 scheduler 的 cron entries（enabled/cron 可能变）
}
```

```go
// internal/worker/worker.go
// UpdateEndpoint 加锁替换 Worker 持有的 endpoint 配置。
// 下次触发时读到的就是新值；不影响正在跑的那一次同步。
func (w *EndpointWorker) UpdateEndpoint(ep config.Endpoint) {
    w.mu.Lock()
    defer w.mu.Unlock()
    w.ep = ep
}
```

### 重启实现：syscall.Exec 原地替换

```go
// internal/server/handlers.go — POST /api/settings/restart
// 用 syscall.Exec 替换当前进程映像：PID 不变，重新加载 config.yaml、重跑迁移、
// 重建全部 Worker。宝塔 Supervisor 下同样工作（Supervisor 靠 PID 守护，PID 不变）。
exe, _ := os.Executable()
syscall.Exec(exe, os.Args, os.Environ())
```

> 重启会中断正在进行的同步请求，属预期行为——结构性变更本就需要重建 Worker 池。

---

## 8. 禁止引入的东西（防止滑回 polabel2）

| 禁止 | 原因 |
|---|---|
| admission 表 / 资源占用表 / lease 列 | 共享 DB 计数器 = 锁序问题根源（polabel2 `df3abc7f` 教训） |
| watchdog goroutine 回收资源 | 进程内资源进程死自动归零，watchdog 是多写者（polabel2 60s 死循环教训） |
| 父子任务 / partial 状态 | 聚合状态导致状态机爆炸（polabel2 `ADMISSIBLE_INTENT_STATUSES` 6 处渗漏） |
| BullMQ / 外部任务队列 | 多进程竞争同队列（polabel2 `6cbdefa8` 死锁教训） |
| staging → canonical 三层 | 本系统不服务高一致性场景，一张结构化表够 |
| Docker | 宝塔直接跑 Go 二进制 + Supervisor 守护，Docker 反而增加复杂度 |
| Redis / 任何常驻第三方服务 | 部署形态是「单二进制 + MySQL」，多一个常驻件就多一处用户看不懂的故障点 |
| 多进程 / 多实例 | 令牌器是进程内的；多实例即失去限流语义，且必然引出跨进程协调 = 锁 |
| K8s / 编排 / 微服务拆分 | 2 账号 12 接口 24 goroutine，拆分没有收益，只有运维成本 |
| 对象存储 | 本项目不存文件，只落结构化行 |
| 外部调度器（crontab / Airflow 等） | 已有进程内 robfig/cron，外部调度会和它抢触发权 |
| 连接池以外的 DB 中间件 | 直连 MySQL 即可，中间件会掩盖真实错误、破坏 fail-loud |

**默认答案是「不做」，举证责任在提议加东西的一方。** 领星 OpenAPI 的全部难度是：AES 签名 → 换 token → GET/POST 翻页 → Upsert 落库；GitHub 上多个同类项目都用最朴素的方式跑通，没有一个需要上面这些。任何「必须引入 X 才能解决」的说法，**先怀疑方案写错了，不是规模到了**。

**同样禁止在代码里自造复杂度**（不引入任何外部依赖也能违规）：分布式锁、租约、状态机、事件总线、插件系统、多层抽象、泛型框架、为「将来可扩展」预留的接口层。polabel2 的 admission 表和父子任务状态机就是这么长出来的，一个第三方服务都没装。宁可代码笨一点、重复一点，也不要让用户看不懂自己的系统。

规模依据见 `CLAUDE.md §1「规模上限」`：账号 ≤10、去重接口 ≤40、Worker ≤400、单表千万行内走索引不分库分表。判断该不该加东西时以这些数字为准，不以「将来可能会大」为准。

---

## 9. 通用 Upsert 实现（`internal/db/upsert.go`）

零代码新增接口的核心机制：Worker 不关心字段名，只靠 MySQL schema 自动推导列。

### 9.1 启动时：读表结构，缓存在 Worker 里

```go
// GetTableColumns 查 INFORMATION_SCHEMA，取目标表的所有列名。
// Worker 启动时调用一次，结果存到 endpoint.Columns []string。
//
// 缺表 → 返回 error → worker.New 把原因记进该 worker 的 fatalErr，**按接口粒度降级**：
// 该「账号+接口」永久 status=error、拒绝同步、每次触发写一条带可读原因的 error 任务行，
// 但仍然注册、仍在 UI 可见；其余 Worker 与 HTTP :7799 照常运行，进程不退出。
// 依据 CLAUDE.md §1.1「任一接口挂掉不影响其他接口」+ §8「缺表/缺列只炸自己」。
//
// 注意：这里**不是** os.Exit(1)。早期版本在 main.go 里 log.Fatalf，后果是一张表没建
// 就连带停掉其余所有接口、7799 打不开、连排查界面都没有 —— 违反 §1.1，已废弃。
// fail-loud 的落点是「这一个接口大声失败」，不是「整个进程自杀」。
func GetTableColumns(db *sqlx.DB, table string) ([]string, error) {
    const q = `SELECT COLUMN_NAME
               FROM INFORMATION_SCHEMA.COLUMNS
               WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
               ORDER BY ORDINAL_POSITION`
    var cols []string
    if err := db.Select(&cols, q, table); err != nil {
        return nil, fmt.Errorf("getTableColumns %s: %w", table, err)
    }
    if len(cols) == 0 {
        return nil, fmt.Errorf("table %q not found — run migrations first", table)
    }
    return cols, nil
}
```

### 9.2 每次写入：动态构造 INSERT … ON DUPLICATE KEY UPDATE

```go
// UpsertRows 把 API 返回的 []map[string]any 写进目标表。
//
// 规则：
//   - 列 = allowedCols（表实际列）∩ row 里存在的 key
//   - API 新增的字段若表里没有 → 静默忽略（不报错）
//   - 表里有但 API 没返回的列 → 传 nil → 写 NULL（表列需允许 NULL）
//   - account_id 固定注入，不来自 API
//   - synced_at 由 MySQL ON UPDATE CURRENT_TIMESTAMP 管理，不插入
func UpsertRows(db *sqlx.DB, table string, rows []map[string]any,
    allowedCols []string, accountID string) error {
    if len(rows) == 0 {
        return nil
    }

    skip := map[string]bool{"synced_at": true}
    cols := []string{"account_id"}
    for _, c := range allowedCols {
        if !skip[c] && c != "account_id" {
            cols = append(cols, c)
        }
    }

    quoted := make([]string, len(cols))
    for i, c := range cols {
        quoted[i] = "`" + c + "`"
    }
    ph := "(" + strings.Repeat("?,", len(cols)-1) + "?)"
    allPH := strings.Repeat(ph+",", len(rows)-1) + ph

    var updates []string
    for _, c := range cols {
        if c != "account_id" {
            updates = append(updates, fmt.Sprintf("`%s`=VALUES(`%s`)", c, c))
        }
    }

    stmt := fmt.Sprintf(
        "INSERT INTO `%s` (%s) VALUES %s ON DUPLICATE KEY UPDATE %s",
        table, strings.Join(quoted, ","), allPH, strings.Join(updates, ","),
    )

    vals := make([]any, 0, len(rows)*len(cols))
    for _, row := range rows {
        vals = append(vals, accountID)
        for _, c := range cols[1:] {
            vals = append(vals, row[c]) // nil → SQL NULL，正常
        }
    }
    _, err := db.Exec(stmt, vals...)
    return err
}
```

### 9.3 字段类型漂移时的 fail-loud

MySQL `Exec` 遇到类型不兼容（如字符串插入 INT 列）会返回 error → Worker 把这次任务标记为 `error` 并记录原始 error 文字 → 不入库脏数据，不静默兜底。

---

## 10. 候选迭代机制

### 10.1 设计

部分接口（如产品表现）要求每次只传一个 `sid`，需要对账号下每个店铺各跑一遍。两个角色：

| 角色 | 配置字段 | 职责 |
|---|---|---|
| **店铺来源接口** | `is_store_source: true` | 同步后把 `sid` 列表写入 `ls_stores`；其他接口从这里读 |
| **迭代接口** | `iterate_by_store: true` | 每次触发时查 `ls_stores`，对每个 sid 跑一次完整同步 |

### 10.2 启动时初始化

```go
// main.go 启动顺序：
// 1. 先启动所有 is_store_source=true 的 Worker（sc_stores / vc_stores）
// 2. 检查 ls_stores 是否有数据；若为空，立即同步一次（阻塞等待完成）
// 3. 再启动 iterate_by_store=true 的 Worker
// 这样店铺列表保证在依赖它的 Worker 运行前就绪。
```

### 10.3 迭代 Worker 主循环

```go
func (w *EndpointWorker) runIterateByStore(ctx context.Context) error {
    sids, err := db.QuerySIDsForAccount(w.accountID)
    if err != nil || len(sids) == 0 {
        return fmt.Errorf("no stores found for account %s — is sc_stores synced?", w.accountID)
    }

    taskID := db.InsertTask(w.endpoint, w.accountID, "running")
    total := 0
    var syncErr error

    for i, sid := range sids {
        params := copyParams(w.extraParams)
        params[w.storeParamName] = sid   // e.g. params["sid"] = "12345"

        records, err := w.fetchAllPages(ctx, params)
        if err != nil {
            syncErr = fmt.Errorf("sid %s: %w", sid, err)
            break
        }
        if err := db.UpsertRows(w.table, records, w.columns, w.accountID); err != nil {
            syncErr = fmt.Errorf("sid %s upsert: %w", sid, err)
            break
        }
        total += len(records)

        // 多店铺间隔（非最后一个才等待）
        if i < len(sids)-1 {
            time.Sleep(time.Duration(w.rate.MultiIntervalMs) * time.Millisecond)
        }
    }

    status := "success"
    if syncErr != nil { status = "error" }
    db.UpdateTask(taskID, status, total, syncErr)
    return syncErr
}
```

### 10.4 VC PO detail 的单一例外

`/basicOpen/platformOrder/vcOrderPo/detail` 每次只接受一个 `local_po_number`，且响应不返回店铺身份。已验证线路使用 `iterate_by_vc_orders: true`：同一账号从 `ls_vc_orders` 按 `gmt_modified` 窗口读取 `(vc_store_id, local_po_number)`，请求体只发送 `local_po_number`，落库前强制写回两个身份字段。

该模式仍是一个 endpoint Worker、一张 `ls_vc_po_details` raw 表和一条 `sync_tasks` 任务；首个请求、解析或 Upsert 错误立即终止本 endpoint。禁止把它扩展成队列、父子任务、staging 或任意跨表工作流。

### 10.5 config.yaml 新增字段

```yaml
# 店铺来源接口（每个账号配一个，优先启动）
- name: sc_stores
  display: "SC 店铺列表（基础）"
  account: "sc_us"
  path: "/erp/sc/data/seller/lists"
  method: "GET"
  table: "ls_stores"
  record_id_fields: ["sid"]
  is_store_source: true          # ← 标记：启动时优先同步，结果供其他接口使用
  rate:
    bucket: 5
    interval_ms: 200
    multi_interval_ms: 0
    dimension: "account+path"
  cron: "0 */6 * * *"            # 每 6 小时刷新一次
  enabled: true
  window_days: 0

# 需要迭代店铺的接口
- name: sc_product_perf
  display: "产品表现（ASIN）"
  account: "sc_us"
  path: "/bd/productPerformance/openApi/asinList"
  method: "POST"
  table: "ls_product_perf"
  record_id_fields: ["asin", "sid"]
  iterate_by_store: true         # ← 启用后 Worker 对每个 sid 循环一次
  store_param_name: "sid"        # ← 迭代时注入到请求的参数名
  rate:
    bucket: 1
    interval_ms: 1000
    multi_interval_ms: 10000
    dimension: "account+path"
  cron: "0 4 * * *"
  enabled: true
  window_days: 30
```
