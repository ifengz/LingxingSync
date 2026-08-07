# CLAUDE.md — 领星同步机（LingxingSync）

> 本文件是给协作 agent 和后续开发者的**最高优先级指引**。
> 内容冲突时的优先级：**本文件「用户意图」段 > `doc/core/`（宪法层）> 现有代码**。
> 维护原则：只写**已实现的事实**和**不可破坏的约束**，不写计划。新增能力落地后回头更新本文件，计划性内容放 `doc/core/progress.md`。

---

## 1. 用户意图（项目为什么存在，不可妥协）

用户要一个**独立、轻量、跑在云服务器上的专用同步机**：把领星 OpenAPI 数据定时拉下来落库，后面所有项目（首先是 polabel2）统一从这个同步机的数据表里读数，不再各自同步。

用户的 6 条原话要求，是本项目的验收红线：

1. **各接口同步独立**：一个「账号+接口」一个独立 goroutine；任一接口挂掉、报错、被限流，**不影响其他接口**，也不会因别的接口断了而连带中断。
2. **按桶令牌并发**：每个 `(quota_group, path)` 一个 `rate.Limiter`；`bucket=1` 强制串行翻页，`bucket>1` 可并发翻页，并发度 = 桶容量。令牌器**进程内**，不入库、不跨进程。
3. **加接口极简单、不伤架构**：新增领星接口 = 填接入合同五格 → 建一张表 → `config.yaml` 加一段 → 重编重启。**零代码改动，零回归风险**，不会重蹈 polabel2「加一个接口全盘崩」的覆辙。
4. **能用领星表格数据校验**：有独立对账能力（`internal/server/reconcile.go`），上传领星导出的 CSV 与本地库比对，输出差异。
5. **轻量、不锁**：单进程、goroutine、无外部队列、无 DB 行级锁序、无 admission/lease/watchdog。「这锁那锁」是被明确禁止的。
6. **数据表结构化、与领星一致，给 polabel2 直读**：每接口一张 `ls_*` 表，列名 = 领星字段名，不翻译、不加工。polabel2 砍掉自身同步，配只读 MySQL 账号直连 `lingsync` 库取数、合并、展示。

### 与 polabel2 的关系（边界）

- 同步机是领星数据的**唯一同步入口**和**唯一数据源**。
- polabel2 及其他项目是**只读消费者**：直连 `lingsync` 库读 `ls_*` 表，不回写、不触发同步。
- **同步机不承担 polabel2 的业务逻辑、聚合计算、店铺矩阵编排**——那些留在消费侧。

---

## 2. 这是全新项目，没有 polabel2 历史包袱

> 明确回答用户：「没有旧 polabel2 的历史东西。」

以下 polabel2 的概念**本项目中一律不存在、禁止引入**（宪法 `doc/core/01-architecture.md §8`）：

| polabel2 的东西 | 本项目的替代 |
|---|---|
| admission 表 / 资源占用表 / lease 列 | 进程内 `rate.Limiter`，进程死即归零 |
| watchdog goroutine 回收资源 | 不需要，进程内资源无残留 |
| 父子任务 / partial / `ADMISSIBLE_INTENT_STATUSES` 状态机 | 单层 `sync_tasks`：一次触发一条任务 |
| BullMQ / 外部任务队列 | 进程内 channel + goroutine |
| staging → canonical 三层数据流 | 一张结构化 `ls_*` 表，Upsert 直接落 |
| self/affiliate/spotterio 三数据源 + Channel 店铺矩阵 | 账号(accounts) + 接口(endpoints) 模型 |

前端层面同样**不继承 polabel2 的同步中心副本**的数据契约：`DataSourceRow` 三源、`ChannelRow` 渠道矩阵、`SyncRunRow` 父子/lease/segments、按 endpoint URL 的限流覆盖——这些是 polabel2 专属概念，本项目不沿用。

---

## 3. 架构事实（已实现，作为约束）

```
Go 单二进制（module: lingxing-sync, Go 1.23）
├─ HTTP Server :7799（固定端口，不漂移）
│   ├─ html/template + Alpine.js + Tailwind CDN，//go:embed web/ 打包
│   └─ REST API（见 doc/core/04-api.md）
├─ Scheduler（robfig/cron）→ 按 cron spec 向 Worker 发触发信号
├─ EndpointWorker × N（每「账号+接口」一个 goroutine）
│   ├─ TokenHolder[app_key]   同 key 单飞刷新，防踩踏
│   ├─ rate.Limiter[(quota_group,path)]  桶令牌限流
│   └─ 主循环：等触发 → 限流 → 翻页 → Upsert → 更新状态
└─ MySQL（lingsync 库）
    ├─ sync_tasks / sync_task_logs   系统表（单写者：Worker 自己）
    └─ ls_*                          数据表（结构化，给消费方只读）
```

**单写者原则**：只有 EndpointWorker 写自己的 `sync_tasks` 状态行。HTTP handler 只能 INSERT 新 pending 行或发 channel 信号，**永不 UPDATE status 列**。

**fail-loud**：领星返回格式异常 → 抛错记日志、任务标 error，**不静默兜底、不猜字段、不写脏数据**。

**通用 Upsert（`internal/db/upsert.go`）**：Worker 不关心字段名，启动时读 `INFORMATION_SCHEMA` 缓存表列，运行时动态构造 `INSERT ... ON DUPLICATE KEY UPDATE`。这是「加接口零代码」的核心机制。

---

## 4. 关键约定（动代码前必读）

1. **端口 7799** 固定，不漂移。
2. **限流键 = `(quota_group, path)`**，默认 `quota_group = accounts[].id`；同账号同 path 的所有 Worker 共用一个桶。
3. **表结构 = 领星字段名**，不翻译；消费方靠只读账号直读。
4. **加接口 = 加配置 + 建表 + 重编重启**，零代码改动。流程见 `doc/core/07-add-endpoint.md`。
5. **配置写回**：`config.yaml` 整体重写 + `.bak` 备份（原子 rename）。
6. **配置生效分两条路**（判定在 `internal/config/store.go RequiresRestart`）：
   - 非结构性（enabled/cron/rate/window/extra_params/store_sids）→ `POST /api/settings/reload` 热加载
   - 结构性（增删 account、增删 endpoint、改 path/method/table、改 database.*/server.port）→ `POST /api/settings/restart` 用 `syscall.Exec` 原地重启（PID 不变）

---

## 5. 常用命令

在 `code/` 目录下执行：

| 命令 | 作用 |
|---|---|
| `make tidy` | 拉取/更新 Go 依赖 |
| `make build` | 编译单二进制 `./lingxing-sync`（约 15MB） |
| `make run` | 编译并前台运行（读 `config.yaml`） |
| `make fmt` / `make vet` | 格式化 / 静态检查 |
| `make migrate` | 提示：迁移由程序启动时自动执行（幂等 `CREATE TABLE IF NOT EXISTS`），无需单独命令 |
| `go test ./...` | 跑全部测试（改完代码必须过） |
| `node --check web/static/app.js` | 前端 JS 语法检查（无构建步骤） |
| `node web/static/app.test.js` | 前端逻辑测试 |

**本地开发（Docker MySQL）**：见 `code/README.md`，`docker run mysql:8.0` → `cp config.example.yaml config.yaml` → `make build && ./lingxing-sync` → 开 `http://127.0.0.1:7799`。

**改完代码的收口动作**：`make fmt && make vet && go test ./...` 全绿 → `make build` → 重启 7799 上的进程 → 抽查 5 个页面 HTTP 200 非空。这套流程记录在 `doc/core/progress.md`，沿用即可。

---

## 6. UI 技术栈红线（重要）

宪法 `doc/core/05-ui.md §1` 当前规定：**禁止 React / Vue / Webpack / npm / Node.js**，服务器上只需要 Go 和 MySQL。现有 5 个页面（API 配置 / 同步中心 / 同步管理 / 日志 / 数据源）用 `html/template` + Alpine.js + Tailwind CDN 实现，`//go:embed web/` 打包进单二进制。

> ⚠️ 如果后续决定引入 React 前端（例如把外部的同步中心 UI 套进来），属于**修改宪法**，必须：
> 1. 先与用户确认接入方式（静态 SPA 托管 / Alpine 重写 / 独立 Node 服务三选一）；
> 2. 更新本节与 `doc/core/05-ui.md`，注明对「单二进制零依赖部署」红线的影响；
> 3. 不得在未更新宪法前，直接把 React 依赖塞进 `go.mod` 或 `web/`。

---

## 7. 目录地图

```
LingxingSync/
├── CLAUDE.md                  ← 本文件
├── doc/core/                  ← 宪法层文档（架构/DB/配置/API/UI/部署/加接口/接口合同）
│   ├── 00-overview.md
│   ├── 01-architecture.md     ← 6 条原则 + 禁止清单（对应本文件 §1）
│   ├── 02-database.md
│   ├── 03-config.md
│   ├── 04-api.md
│   ├── 05-ui.md               ← UI 红线（对应本文件 §6）
│   ├── 06-deployment.md
│   ├── 07-add-endpoint.md     ← 加接口五步（对应本文件 §4.4）
│   ├── 08-api-reference.md    ← 领星 OpenAPI 接入参考
│   ├── 09-endpoint-contract.md ← 接入合同五格模板
│   ├── progress.md            ← 过程记录（追加式）
│   └── findings.md            ← 调查结论（追加式）
└── code/
    ├── main.go                ← 入口
    ├── go.mod / go.sum
    ├── Makefile
    ├── config.yaml            ← 运行配置（gitignore，不进 git）
    ├── config.example.yaml    ← 示例配置（进 git，注释永久保留）
    ├── internal/
    │   ├── config/            ← Config + Store（写回/校验/RequiresRestart）
    │   ├── api/               ← 领星 HTTP client（AES 签名）+ TokenHolder
    │   ├── worker/            ← EndpointWorker + Registry + Limiter + Scheduler
    │   ├── db/                ← pool + tasks + upsert + migrate
    │   └── server/            ← HTTP server + handlers + reconcile
    ├── web/
    │   ├── templates/         ← Go html/template（5 个页面）
    │   └── static/app.js      ← Alpine.js 逻辑
    └── migrations/            ← 001_system / 002_data_tables / 003_fix_nullable
```

**改代码时的落点直觉**：
- 改同步行为 → `internal/worker/`
- 改/加接口配置 → `internal/config/` + `config.example.yaml`
- 改 API/页面 → `internal/server/` + `web/`
- 加新领星接口 → 只动 `migrations/`（建表）+ `config.yaml`（加段），**不碰 internal 代码**
- 改数据落库 → `internal/db/upsert.go`（通用，通常不用改）
