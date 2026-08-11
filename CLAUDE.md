# CLAUDE.md — 领星同步机（LingxingSync）

> 本文件是给协作 agent 和后续开发者的**最高优先级指引**。
> 内容冲突时的优先级：**本文件「用户意图」段 > `doc/core/`（宪法层）> 现有代码**。
> 维护原则：只写**已实现的事实**和**不可破坏的约束**，不写计划。新增能力落地后回头更新本文件，计划性内容放 `doc/core/progress.md`。

---

## 1. 用户意图（项目为什么存在，不可妥协）

用户要一个**独立、轻量、跑在云服务器上的专用同步机**。核心仍是把领星 OpenAPI 数据按接口独立落入本项目 `ls_*` 原始表；在不改变单进程 Go + MySQL 模型的前提下，还允许正式报表证据、唯一的 listing 日维事实集，以及供内部项目通过 HTTPS 读取的固定数据集 API。授权边界见下文「四项能力边界」，未落地的能力不得写成已实现事实。

本项目的验收红线共 14 条：**1–6 目标、7–11 写码**（均为用户原话），**12–14 工程纪律**（自 aftersale 引入，用户已确认）。另有「规模上限」表，是判断架构该不该加东西的数字依据。

1. **各接口同步独立**：一个「账号+接口」一个独立 goroutine；任一接口挂掉、报错、被限流，**不影响其他接口**，也不会因别的接口断了而连带中断。
2. **按桶令牌并发**：每个 `(quota_group, path)` 一个 `rate.Limiter`；`bucket=1` 强制串行翻页，`bucket>1` 可并发翻页，并发度 = 桶容量。令牌器**进程内**，不入库、不跨进程。
3. **加接口极简单、不伤架构**：新增领星接口 = 填接入合同五格 → 建一张表 → `config.yaml` 加一段 → 重编重启。**零代码改动，零回归风险**，不会重蹈 polabel2「加一个接口全盘崩」的覆辙。
4. **能用领星正式报表校验并纠正日维结果**：已有独立对账能力（`internal/server/reconcile.go`）可上传 CSV 与本地库比对并输出差异；正式 Amazon 报告的导出/导入只能走领星 OpenAPI 线路，必须作为独立证据保留，不能覆盖对应 API 原始行。
5. **轻量、不锁**：单进程、goroutine、无外部队列、无 DB 行级锁序、无 admission/lease/watchdog。「这锁那锁」是被明确禁止的。
6. **原始表结构化、与领星一致**：每个已验证的接口或正式报告合同各自对应一张 `ls_*` 原始表，列名 = 领星字段名，不翻译、不加工。除唯一允许的 `listing_daily_metrics` 日维事实集外，不做其他跨表派生、宽表拼接或消费者页面适配。

### 写码红线（同为用户原话，与上面 6 条同级）

7. **简洁优先**：不造复杂逻辑、不造多余抽象层、不加「这锁那锁」。一层函数能解决就不要拆三层；新需求先问「能不能不加东西」。
8. **缺表/缺列只炸自己**：DDL 不匹配（表不存在、列缺失）→ 只让该「账号+接口」的任务标 error 并给出可读提示（缺哪张表、缺哪几列），**进程不退出、其他 Worker 不受影响、网站照常 200**。启动期的表结构探测同样按接口粒度降级，不得因一张表缺失阻断整个进程或页面。
9. **代码最短且不违宪**：优先最短实现；短与宪法冲突时守宪法，并在回复里说明取舍。
10. **少嵌套**：用 early return / guard clause / map 表驱动替代深层 if-else；嵌套超过 2 层就重写。
11. **写完核对宪法**：每次改完代码逐条对 `doc/core/` 检查是否偏移（01 架构 / 02 DB / 03 配置 / 04 API / 05 UI），偏了就改回来，改不回来先报备。

### 规模上限（数字即依据，用来堵架构膨胀）

本项目按下表规模设计。**判断「要不要加东西」时以这些数字为依据，不以「将来可能会大」为依据。**

| 维度 | 当前实际 | 设计上限 |
|---|---|---|
| 账号数（`accounts[]`） | 2（sc_us_1 / sc_us_2） | 10 |
| 去重接口数（不同 path） | 12 | 40 |
| Worker goroutine 数（账号 × 接口） | 24 | 400 |
| 单桶并发（`rate.bucket`） | 最大 5，14/24 个接口是 1 | 10，且不得超过领星实际配额 |
| `ls_*` 数据表 | 14 | 一接口/正式报告合同一表，单表千万行内用索引解决，**不分库分表** |

**下列东西禁止引入**：Redis / 外部消息队列 / 多进程或多实例 / Docker-K8s 编排 / 对象存储 / 微服务拆分 / 外部调度器 / 连接池以外的 DB 中间件 / 任何常驻第三方服务 / 动态 schema builder / 远程 SQL / 消费方直连数据库 / ERP 内部页面浏览器自动化。

**这些东西在本项目没有使用场景，所以是「禁止」，不是「需要审批」。** 原始同步主链仍只是：AES 签名 → 换 token → GET/POST 翻页 → Upsert 落库；新增的报表证据、日维事实集和只读 API 也必须留在同一进程和 MySQL 内，不能成为引入外部基础设施的理由。任何声称「必须引入 X 才能解决」的提案，**先怀疑是方案写错了，不是规模到了**。默认答案是不做，不必给理由 —— 要给理由的是提议加东西的一方。

同理**禁止在代码里自造复杂度**：分布式锁、租约、状态机、事件总线、插件系统、多层抽象、泛型框架、为「将来可扩展」预留的接口层 —— 一律不要。宁可代码笨一点、重复一点，也不要让用户看不懂自己的系统。理由见 §2。

### 工程纪律（自 aftersale 项目引入，用户已确认）

12. **禁止空壳**：不提前堆空接口、空表、空页面、无调用方的抽象层。**接口未 probe 实证不得进 `config.yaml`，字段未 probe 实证不得进 `migrations/`** —— 仓里已有成批臆造的 path / 唯一键 / 列名，这是复发过的问题。按真实需要新增，不按想象新增。
13. **动作 ≠ 验证**：严禁把「做过动作 A」当成「结果 B 已验证」。具体地：
    - `make build` 通过 ≠ 接口能拉到数；
    - `go test ./...` 全绿 ≠ 同步跑通；
    - 任务标 `success` ≠ 数据落对了（要看 `ls_*` 真实行数与 `sync_task_logs`）；
    - 页面 200 ≠ 页面数据正确；
    - 进程重启了 ≠ 新配置生效了（结构性改动要确认走的是 restart 而非 reload）。

    没实测过的就说「没测」，不说「应该没问题」。
14. **宪法层改动独立 commit**：宪法层 = `CLAUDE.md` + `doc/core/00-09` 这十份编号文档 + `doc/core/05-ui-components.md`。改它必须单独一个 commit，不与业务代码、测试、UI、迁移、过程文件混提。任务执行中发现宪法层需要调整 → **停下来先报冲突和待改文件，等用户确认**，不自行顺手改。

    `doc/core/` 下的其余文件**不是宪法**，随业务改动一起提即可：`progress.md`（过程记录）、`findings.md`（调查结论）、`lessons.md`、`task_plan.md`、`10-frontend-rework-flow.md`（执行规格，含已废弃条目）、`sync-field-source-map.md`、`otherlingxinggithub.md`、`LINGXING_API_INTEGRATION.md`。把过程文件当宪法保护只会让「记一笔进展」也要单独 commit，纯属自我设障。

### 四项能力边界（已授权，不等于已实现）

1. **原始同步**：保留「一个接口/报告合同 → 一张 `ls_*` 原始表」。接口响应与正式报告行是两份独立证据，不互相改写。
2. **正式报表证据**：Amazon 正式报告导出走 OpenAPI 的创建任务、异步状态查询、下载链接续期线路；不得混用 ERP `auth-token` 或页面登录态。正式报告只有在解析和对账成功后才覆盖 `listing_daily_metrics` 的同字段有效值；没有正式报告值时可暂用 API 原始值，API 原始表始终不变。
3. **唯一日维事实集**：只允许一个 `listing_daily_metrics`，粒度固定为 `store/channel/ASIN/SKU/business_date`；可用一对一 listing 维度键压缩索引，但不得改变粒度。PO 等不同粒度域必须留在独立数据集。来源没有覆盖的值保持 `NULL`/未验证，禁止造零；没有 ASIN/SKU 的 HSA 只能保留为店铺级数据，或明确标记为分摊值。
4. **固定只读发布**：内部项目只能通过版本化 HTTPS `snapshot` / `changes` 数据集端点读取，使用每项目 token、dataset/store scope、keyset cursor 和分页。端点仅接受固定 dataset 与字段 allowlist，不接受任意表名、路径或 SQL；消费者不得直连本库。`/datasources` 可在现有 Alpine.js + Tailwind 页面内选择某数据集允许返回的字段，但该 UI 永不创建、修改或删除 MySQL 表。

以上能力都必须物理留在本 Go 进程与 MySQL 中；不得借机增加 Redis、对象存储、队列、微服务、React/npm 或通用动态数据平台。`changes` 只发布 LingxingSync 已经落库的变化，不能替代上游重拉或正式报表对账。

**已授权的唯一候选迭代例外**：VC PO detail 必须先从同账号 `ls_vc_orders` 取得 `local_po_number` 与 `vc_store_id`。它只使用 `iterate_by_vc_orders` 在本 endpoint 内串行逐单请求并直接写唯一的 `ls_vc_po_details`，不引入队列、父子任务、staging 或通用工作流引擎；其他接口不得借此扩展跨表编排。

### 与 polabel2 的关系（边界）

- LingxingSync 与 polabel2 **完全独立**，不存在数据库直连或页面接入关系。polabel2 如需消费数据，只能按本节固定 HTTPS 数据集合同读取，不得把自身表结构或页面合同反向带入本项目。
- polabel2 只用于学习其已经跑通的领星接口对接证据：method/path/body、候选账号与店铺上下文、字段处理和错误处理。
- 参考结论必须在 LingxingSync 自身按真实账号、真实响应和原始表合同重新验证；不得修改 polabel2，也不得把其业务逻辑、事实表或页面合同带入本项目。

---

## 2. 这是全新项目，没有 polabel2 历史包袱

> 明确回答用户：「没有旧 polabel2 的历史东西。」

**为什么要立这份禁止清单**：polabel2 就是被下面这些东西搞死的 —— 锁、状态机、外部队列、三层数据流一层层叠上去，最后加一个接口全盘崩，而用户不懂编程、修不了。所以这里禁的不是「不好的技术」，是**用户维护不了的复杂度**。下表左列每一项都在 polabel2 上真实出过事，不是假想风险。

以下 polabel2 的概念**本项目中一律不存在、禁止引入**（宪法 `doc/core/01-architecture.md §8`）：

| polabel2 的东西 | 本项目的替代 |
|---|---|
| admission 表 / 资源占用表 / lease 列 | 进程内 `rate.Limiter`，进程死即归零 |
| watchdog goroutine 回收资源 | 不需要，进程内资源无残留 |
| 父子任务 / partial / `ADMISSIBLE_INTENT_STATUSES` 状态机 | 单层 `sync_tasks`：一次触发一条任务 |
| BullMQ / 外部任务队列 | 进程内 channel + goroutine |
| 通用 staging → canonical 三层数据流 | 原始接口/报告各自直落 `ls_*`；仅允许一个固定 `listing_daily_metrics` 日维事实集 |
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
    ├─ ls_*                          领星接口/正式报告原始证据表（结构化）
    └─ listing_daily_metrics         唯一允许的日维事实集（授权合同；未实现前不得声称存在）
```

**单写者原则**：只有 EndpointWorker 写自己的 `sync_tasks` 状态行。HTTP handler 只能 INSERT 新 pending 行或发 channel 信号，**永不 UPDATE status 列**。

**fail-loud**：领星返回格式异常 → 抛错记日志、任务标 error，**不静默兜底、不猜字段、不写脏数据**。

**通用 Upsert（`internal/db/upsert.go`）**：Worker 不关心字段名，启动时读 `INFORMATION_SCHEMA` 缓存表列，运行时动态构造 `INSERT ... ON DUPLICATE KEY UPDATE`。这是「加接口零代码」的核心机制。

---

## 4. 关键约定（动代码前必读）

1. **端口 7799** 固定，不漂移。
2. **限流键 = `(quota_group, path)`**，默认 `quota_group = accounts[].id`；同账号同 path 的所有 Worker 共用一个桶。
3. **表结构 = 领星字段名**，不翻译、不承担跨项目映射。
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

**改完代码的收口动作**：`make fmt && make vet && go test ./...` 全绿 → `make build` → 重启 7799 上的进程 → 抽查 4 个页面 HTTP 200 非空 → **对照 `doc/core/` 逐条核对是否偏移宪法（见 §1.11）**。这套流程记录在 `doc/core/progress.md`，沿用即可。

---

## 6. UI 技术栈红线（重要）

宪法 `doc/core/05-ui.md §1` 当前规定：**禁止 React / Vue / Webpack / npm / Node.js**，服务器上只需要 Go 和 MySQL。现有 4 个页面（API 配置 / 同步配置 / 同步日志 / 数据源）用 `html/template` + Alpine.js + Tailwind CDN 实现，`//go:embed web/` 打包进单二进制。

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
│   ├── 05-ui-components.md    ← 共享 UI 组件与逐页验收（宪法层）
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
    │   ├── templates/         ← Go html/template（4 个页面）
    │   └── static/app.js      ← Alpine.js 逻辑
    └── migrations/            ← 001_system / 002_data_tables / 003_fix_nullable
```

**改代码时的落点直觉**：
- 改同步行为 → `internal/worker/`
- 改/加接口配置 → `internal/config/` + `config.example.yaml`
- 改 API/页面 → `internal/server/` + `web/`
- 加新领星接口 → 只动 `migrations/`（建表）+ `config.yaml`（加段），**不碰 internal 代码**
- 改数据落库 → `internal/db/upsert.go`（通用，通常不用改）
