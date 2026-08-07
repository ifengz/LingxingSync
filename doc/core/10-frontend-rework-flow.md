# 10 · 前端操作动线重建计划（交给 GLM 5.2 落地）

> 本文件是宪法之后的**执行规格**。读完 `00`~`09` 与本文件即可动手。
> 目标：让 Go 版同步机的 Alpine 前端，在**操作动线 / 配置方式 / 增删改方式**上
> 对齐参照前端（React 版 polabel2 同步中心）的使用流程。**UI 视觉不要求一致**，
> 但"完成一件事的动作序列"要一致。

---

## 0. 这份文档是什么 / 不是什么

- **是**：对 `code/web/`（templates + static/app.js）现有 Alpine 前端的**聚焦改造**规格，
  外加**一处**后端改动（`worker.go` + `handlers.go`）。
- **不是**：推倒重写。现有 5 页骨架、账号/接口 CRUD、定时调度内联编辑、日志筛选/分页、
  详情抽屉**大部分要保留**，只改本文件"任务清单"里列出的点。
- **不是**：把 React 代码搬过来。React 只是"操作动线"的参照物，**一行 JS/JSX 都不抄**。

关键判断（已核对现有代码得出）：**8 条流程决策里，一半现有前端已满足或接近满足；
真正需要改后端的只有 1 处（手动同步按次指定店铺）。**

---

## 1. 铁律：数据模型与既定事实（不可协商）

### 1.1 权威数据模型 = Go 侧配置 + 扁平任务表

- **根 = 领星账号**（`config.accounts[]`：`id / name / quota_group / app_key / app_secret`）。
- **叶 = 接口 endpoint**（`config.endpoints[]`：`name / display / account / path / method /
  table / record_id_fields / rate{bucket,interval_ms,multi_interval_ms,dimension} /
  cron / enabled / window_days / extra_params / is_store_source / iterate_by_store /
  store_param_name / store_sids[]`）。
- **店铺不是独立维度**：它是某个 endpoint 的 `iterate_by_store` + `store_sids[]` 白名单。
- **运行记录 = 单层** `sync_tasks` / `sync_task_logs`（BIGINT 主键）。
  **没有** parent_run_id / chunk_label / segments / lease —— 宪法明令禁止，别造。
- 配置真值在 `config.yaml`，经 `/api/config` 读、经账号/接口 CRUD 接口写；**不是数据库**。

### 1.2 禁止照搬 React 的字段名（会污染 Go 模型）

参照前端用的是另一套模型，**下列名字一个都不许出现在 Go 前端/接口里**：

| React 里的名字（❌ 禁用） | Go 里的对应物（✅ 用这个） |
|---|---|
| `run_id` / `SyncRunRow` / parent-child run | `task.id`（扁平 `sync_tasks` 一行） |
| `source` = self / affiliate / spotterio | `account_id`（领星账号 id，如 `sc_us`） |
| `CoverageDimension` / 覆盖矩阵按天×维度 | 无。概览用 `账号 × 接口` 状态格（见 T5） |
| `SyncScheduleRow` | endpoint 配置对象（`/api/config` 的 `endpoints[]`） |
| `chunk_label` / `segment` / `lease` | 无，禁止引入 |

字段命名一律 **snake_case**，与 `doc/core/04-api.md`、`handlers_config.go` 的 DTO 完全一致。

### 1.3 既定事实（宪法/架构锁死，不在本次决策范围）

- **SSE → 5 秒轮询**：Go 后端无 `/api/sync-events`，不做 SSE。所有"实时感"靠 `setInterval` 轮询。
- **1 页 5 Tab → 5 个独立路由页**：`/settings/api`、`/`、`/sync`、`/logs`、`/datasources`，
  服务端渲染、无前端路由。这是 B 方案硬约束，不改。
- **响应外壳**：`{"ok":true,"data":...}` / `{"ok":false,"error":"..."}`；时间 RFC3339 UTC。
- **鉴权**：`X-Sync-Secret` 头（`app.js` 的 `apiRequest` 已处理，沿用）。
- **单写者**：只有 EndpointWorker 更新自己的 `sync_tasks` 行；前端只读任务、只发触发/取消。
- **并发模型（务必照抄，别自创）**：每个接口 = 一个 goroutine 跑 `Run` 主循环，**接口之间互不影响**；
  接口内/间的并发节流**只靠 `(quota_group, path)` 的令牌桶** `limiter.Wait`。
  这是宪法 §5/§6 的核心，本次**不得改动**。
- **禁止加锁（这是硬红线）**：本次任何新功能**不许引入新的 mutex / RWMutex / 共享可变状态 / "这锁那锁"**。
  跨 goroutine 传数据一律走 **channel**（Go 的 "share memory by communicating"）。
  worker 里现存的唯一一把 `sync.RWMutex` 只护状态快照，是既有最小设计——**别动它、也别照它再加第二把**。

---

## 2. 决策矩阵：8 条流程差异 → 本次怎么做

（用户已逐条拍板。"跟 React"=对齐参照前端动线；"跟宪法"=保持 Go 简版。）

| # | 决策点 | 结论 | 现状 | 改动量 |
|---|---|---|---|---|
| ① | 手动同步·数据类型选择 | **多选**（跟 React） | 单选 `<select>` | 前端中 |
| ② | 手动同步·店铺选择 | **勾选网格+搜索+全选**（跟 React） | 无店铺选择 | **前端大 + 后端（唯一）** |
| ③ | 是否记住上次选择 | **不记住**（跟宪法） | 本就不记 | 零 |
| ④ | 列表刷新方式 | **5s 轮询 + 手动刷新按钮**（都要） | 有手动、缺轮询 | 前端小 |
| ⑤ | 日志页·取消/重试入口 | **日志表格行内逐条**（跟 React） | 重试在抽屉、取消在别页 | 前端中 |
| ⑥ | 是否多选批量取消/重试 | **仅单条**（跟宪法） | 本就单条 | 零 |
| ⑦ | 日志页·筛选维度 | **多维**（跟 React） | 已 5 维筛选 | 零（仅改 1 label） |
| ⑧ | 日志页·分页方式 | **固定条数+上/下页**（跟宪法） | 已实现 | 零 |

**据此，本次工作量分布**：后端仅 T2 一处；前端 T1/T2/T3/T4/T5 五个任务；T6 是校正。

---

## 3. 后端唯一改动（T-BE）：手动同步支持"按次指定店铺"

**为什么需要**：决策② 要求手动同步能像 React 那样"这一次只同步勾选的店铺"。
现状 `worker.go` 的多店铺 sid 来源是 `QuerySIDsForAccount(account)` → `effectiveStoreSIDs()`
用**配置级** `Endpoint.StoreSids` 白名单过滤，**没有"按次覆盖"通道**。

**为什么不用"改配置白名单"绕过**：那会把一次性的临时选择写成持久配置（`store_sids` 变更
虽是 ChangeHot 热加载、但语义错误：手动跑一次不该改掉定时任务的店铺范围）。必须走**运行期
按次参数**，不落 `config.yaml`。

### 3.1 `worker.go`：触发通道从 `chan string` 升级为携带可选 sids

改动集中在 3 处，**不动 doSync 的单写者/panic 隔离/限流逻辑**。

> **无锁提醒**：`store_sids` 是随 `trigger` 通道发给"该接口自己的 goroutine"、在它自己的
> `doSync` 里消费的**局部数据**，全程只有 owning goroutine 触碰。**不要**为它加任何 mutex、
> 不要放进什么共享 map 再加锁保护——那是反模式。channel 传值本身就是同步边界，够了。

1. **触发信号结构化**（约 worker.go:63 & :102 & :110-126）：
   ```go
   // 原： trigger chan string
   // 新：
   type triggerReq struct {
       kind      string   // "cron" | "manual"
       storeSids []string // 仅 manual 且用户按次指定时非空；nil/空 = 沿用配置白名单
   }
   trigger chan triggerReq  // 仍缓冲 1、仍非阻塞
   ```
2. **触发方法**（worker.go:110-126）：
   ```go
   func (w *EndpointWorker) Trigger() bool { return w.send(triggerReq{kind: "cron"}) }
   func (w *EndpointWorker) TriggerManual(storeSids []string) bool {
       return w.send(triggerReq{kind: "manual", storeSids: storeSids})
   }
   func (w *EndpointWorker) send(req triggerReq) bool {
       select { case w.trigger <- req: return true; default: return false }
   }
   ```
   > cron 调度器仍调 `Trigger()`（无参），行为不变。
3. **doSync 消费按次 sids**（worker.go:214 起，多店铺分支 :283）：
   - `Run` 的 `case req := <-w.trigger:` 把 `req` 透传给 `runOnceSafely(ctx, req)` → `doSync(ctx, req)`。
   - 多店铺分支里，`sids` 的确定顺序改为：
     ```go
     sids, _ := db.QuerySIDsForAccount(w.DB, w.Account.ID)
     if len(req.storeSids) > 0 {
         sids = intersect(sids, req.storeSids)   // 按次覆盖，但仍受账号真实店铺集约束
     } else {
         sids = w.effectiveStoreSIDs(sids)       // 老路径：配置白名单
     }
     ```
   - `triggerType` 字符串（写进 `sync_tasks.trigger_type`）取 `req.kind`。
   - **安全**：按次 sids 必须与"该账号真实存在的 sid"取交集，杜绝越权同步别的店铺。

### 3.2 `handlers.go`：`apiSyncTrigger` 读 body 的 `store_sids[]` 并透传

`syncTriggerIn`（handlers.go:356）加一个可选字段，`TriggerManual` 带上：
```go
type syncTriggerIn struct {
    Force     bool     `json:"force"`
    StoreSids []string `json:"store_sids"` // 可选：本次只同步这些店铺；空=按配置
}
// ...解析后：
if !w0.TriggerManual(in.StoreSids) { errJSON(w, 409, "...已在运行或队列...") }
```
> `POST /api/sync/{name}` 路由、鉴权、409 语义**全不变**，只是 body 多认一个可选键。
> 非 `iterate_by_store` 的接口即使传了 `store_sids` 也会被忽略（那分支不迭代店铺）——前端负责不展示网格，后端多一层无害兜底。

### 3.3 验收（后端）
- `POST /api/sync/{name}` body `{}` → 行为与改造前完全一致（回归不破）。
- body `{"store_sids":["A","B"]}` 且该接口 `iterate_by_store:true` → 本次只跑 A、B（且 A/B 必须是该账号真实店铺）。
- 传入不属于该账号的 sid → 被交集过滤掉，不报错、不同步。
- `config.yaml` 不因手动同步发生任何写入。

---

## 4. 前端任务清单

> 全部改 `code/web/templates/*.html` + `code/web/static/app.js`。
> 沿用现有 `apiGet/apiPost/apiPut/apiDelete`、`toast`、`syncConfirm`、`fmtRel/fmtTime` 等公共件。
> 不新增构建步骤、不引入新库（Tailwind CDN + Alpine CDN 不变）。

### T1 · 手动同步：数据类型改多选（决策①）

- **现状**：`sync_manage.html:32-38` 单选 `<select x-model="form.endpoint">`；
  `app.js` `syncManage.triggerSync()`（:280）只对一个 endpoint 打一次 POST。
- **目标**：像 React 那样一次选多个接口，点"立即同步"后**对每个选中接口并发触发**。
- **动线要求**（对齐 React，不要求视觉一致）：
  - 用可勾选列表/卡片（`<template x-for="e in endpoints">` + checkbox 绑 `form.endpoints[]`）。
  - 按账号分组展示接口（`e.account_id` 分组小标题），因为账号是根维度。
  - 顶部"全选/清空"快捷；已选数量提示。
- **改法**（`app.js` syncManage）：
  ```js
  form: { endpoints: [], storeSids: [] },   // 原 form.endpoint(单值) 改 endpoints(数组)
  async triggerSync() {
    if (!this.form.endpoints.length) { window.toast('warn','请至少选择一个数据类型'); return; }
    const sel = this.form.endpoints.slice();
    const results = await Promise.allSettled(sel.map(name =>
      window.apiPost('/api/sync/'+encodeURIComponent(name),
        this.form.storeSids.length ? { store_sids: this.form.storeSids } : {})));
    // 汇总：成功 N 个、失败 M 个（失败项用 toast 列名）；随后 setTimeout(()=>this.load(),500)
  }
  ```
  > 用 `Promise.allSettled` 而非 `all`：一个接口 409（已在跑）不该阻断其它接口触发。
- **验收**：勾 3 个接口→点同步→3 个都进队列（"最近同步任务"出现 3 行）；其中 1 个正在跑时其余仍入队，失败项有可读提示。
- **禁止**：不要为多选新造后端批量端点；就是前端 fan-out 调现有单路径 `POST /api/sync/{name}`。

### T2 · 手动同步：加"店铺勾选网格 + 搜索 + 全选"（决策②，配合 T-BE）

- **现状**：手动同步区**完全没有**店铺选择。
- **目标**：选了接口后，若接口 `iterate_by_store:true`，展示其账号的店铺网格，可搜索、可全选、可勾选子集；
  勾选结果作为 `store_sids` 随触发按次下发（见 §3）。
- **数据来源**：`GET /api/accounts/{id}/stores` → `{account_id,total,last_synced_at,items:[StoreSummary]}`。
  （`settingsApi.loadStores()` :645 已有同款调用，可参照。）
- **动线要求**：
  - 网格仅在"已选接口中存在 `iterate_by_store:true` 的接口"时出现；否则隐藏并提示"所选接口按接口配置全量同步"。
  - 多选接口可能跨账号 → **店铺网格按账号分区**（每账号一块，各自搜索/全选）。`store_sids` 下发时对每个接口只取"它自己账号"的勾选子集。
  - 搜索框按店铺名/sid 即时过滤；"全选/反选"作用于当前账号分区的**可见（过滤后）**项。
  - 不勾任何店铺 = 不传 `store_sids` = 后端按配置白名单（等价"全部"）。这就是决策③"不记住"的自然结果：每次进页面都是空选。
- **改法**：`syncManage` 加 `storesByAccount:{}`、`storeQuery:{}`；`watch` 选中接口变化时懒加载涉及账号的店铺；触发时按接口账号切分 `store_sids`。
- **验收**：选一个 `iterate_by_store` 接口→网格出现→搜索能过滤→全选后触发，后端只跑选中店铺；选一个非迭代接口→网格隐藏、提示全量；跨两账号各选部分店铺→各接口只收到本账号那份。
- **禁止**：不把勾选写回 `config.yaml`；不调 `PUT /api/endpoints`（那是改持久白名单，不是本次语义）。

### T3 · 列表刷新：5s 轮询 + 手动刷新按钮并存（决策④）

- **现状**：`syncCenter` 声明了 `polling:null` 但**没启动**；`syncManage` 只有手动"刷新"。
- **目标**：进入 `/`（同步中心）和 `/sync` 手动 Tab、`/logs` 后，每 5 秒自动拉一次；手动刷新按钮保留。
- **改法**：各组件 `init()` 里 `this.polling = setInterval(()=>this.load(), 5000)`；
  `destroy()`（Alpine 组件卸载）里 `clearInterval(this.polling)`。轮询失败静默（`catch(window.toastError)` 已吞错，不要弹窗刷屏）。
  - 同步中心：轮询 `/api/status`。
  - 同步管理手动 Tab：轮询"最近同步任务"（`loadRecentTasks`）+ 运行态；**切到定时调度 Tab 时可停轮询**（避免无谓请求）。
  - 日志页：轮询当前筛选条件下的 `load()`；**分页停留在用户所在页**，不要每次轮询跳回第 1 页。
- **验收**：触发一次同步后不手动刷新，5 秒内状态自动从"运行中→成功"；手动刷新按钮仍即时生效；离开页面无残留 timer（切页不报错、不泄漏）。
- **禁止**：不做 SSE；轮询间隔固定 5s，不要指数退避等过度设计。

### T4 · 同步日志：取消/重试下放到表格行（决策⑤，仅单条=决策⑥）

- **现状**：`logs.html` 表格行只有"查看"→抽屉；"重新触发"在抽屉底部（:137）；"取消"只在 `sync_manage` 的最近任务里。
- **目标**：日志表格**每行**按状态显示行内操作：
  - `status==='running'` → "取消"按钮：`@click.stop` 调 `POST /api/sync/{t.endpoint}/cancel` body `{task_id:t.id}`（先 `syncConfirm`）。
  - `status==='error'` → "重试"按钮：`@click.stop` 调 `POST /api/sync/{t.endpoint}`（复用触发；**后端零改动**）。
  - 其它状态该列留空。`@click.stop` 防止冒泡触发行的 `openDetail`。
- **抽屉**：保留详情抽屉与其中的"重新触发"（不冲突，是详情内的便捷入口）。
- **改法**：`logsPage` 加 `cancelRow(t)` / `retryRow(t)`；成功后 `setTimeout(()=>this.load(),500)`。
- **验收**：运行中的行能取消（→已取消）；失败行能重试（→新任务入队，5s 轮询里出现新行）；点按钮不误开抽屉。
- **禁止**：**不加多选框/批量条**（决策⑥仅单条）。不在日志页做筛选之外的复杂批处理。

### T5 · 同步中心概览：账号 × 接口 状态格（宪法简版；覆盖矩阵留待以后）

- **现状**：`syncCenter` 有 `cellClass/cellText/cellClick`（:211-238），已是"账号×接口"格思路。
- **目标**：**确认并保持宪法简版**——概览是 `账号（行）× 接口（列）` 或 `接口 × 账号` 的状态格，
  单元格显示该 worker 状态（运行/成功/失败/空闲/禁用，颜色见宪法 §5），点格子跳 `/logs?endpoint=&account=`（`cellClick` 已实现）。
- **明确不做**：React 那种"点店铺行→按天×维度覆盖率抽屉"。用户已决定**留待以后**。
  - 给后端留话：将来若要做覆盖矩阵，需新增按天统计 `ls_*` 表的 API（现无），不在本次范围。
- **数据来源**：`GET /api/status` 的 `workers[]` + `summary`（已有）。
- **验收**：概览能反映每个 账号×接口 的当前状态并跳转到对应筛选的日志；无覆盖率抽屉、无按天维度。
- **禁止**：不引入 `CoverageDimension`、不按 self/affiliate/spotterio 分组。

### T6 · 校正：日志筛选里"店铺"标签实为"账号"（决策⑦ 收尾）

- **现状**：`logs.html:26` 筛选项 label 写"店铺"，但 `x-model="filters.account"` 且选项来自 `accountIDs`，
  传给后端的是 `account` 参数 → 实际是**领星账号**，不是店铺。
- **目标**：把该 label 从"店铺"改为"账号"，消除概念污染（对齐 §1.2：`account_id` 才是账号）。
  同理检查表头 `logs.html:62`"店铺"列——它展示 `t.account_id`，应改表头为"账号"。
  `sync_manage.html:68` 最近任务表头"店铺"同样展示 `account_id`，一并改"账号"。
- **验收**：三处 label/表头都显示"账号"，与其真实内容一致。多维筛选功能本身**不动**（已支持 endpoint/account/status/date_from/date_to）。

---

## 5. 建议执行顺序（依赖优先）

1. **T2 后端**（`worker.go` + `handlers.go` 的 `store_sids[]` 按次穿透）——先把契约打通，前端才有得调。
2. **T1**（多选数据类型）——手动同步的骨架，T2 前端网格挂在它上面。
3. **T2 前端**（店铺网格）——依赖 T1 的选中态与账号推导。
4. **T4**（日志行取消/重试）——独立，可并行。
5. **T3**（轮询）——各页收尾统一加。
6. **T5 / T6**（概览确认 + label 校正）——最后扫尾。

每步做完**能编译、能手测**再进下一步（宪法：fail-loud，别攒着）。

## 6. 全局验收清单（Definition of Done）

- [ ] `go build ./...` 通过；`go vet ./...` 无新增告警。
- [ ] 手动同步：能多选 N 个接口，一次点击并发触发 N 个任务，逐个 toast。
- [ ] 手动同步：`iterate_by_store` 接口出现店铺网格，可搜索、可全选、可勾选子集；
      勾选子集后触发，该次只同步选中店铺（在日志逐页里可验证 sid 数量）。
- [ ] 非 `iterate_by_store` 接口：不显示店铺网格，触发行为不变。
- [ ] `store_sids` 按次覆盖**不写回 config.yaml**（同步完看 `/api/config`，`store_sids` 不变）。
- [ ] 5s 轮询在 `/`、`/sync`手动、`/logs` 生效；手动刷新按钮并存；切页无 timer 泄漏。
- [ ] 日志行：运行中可取消、失败可重试；点操作按钮不误开抽屉；无批量框。
- [ ] 日志多维筛选 + 固定分页仍正常；"账号"label/表头三处已校正。
- [ ] 概览为 账号×接口 状态格，点格跳筛选日志；无覆盖率抽屉。
- [ ] 全程无 `run_id`/`source`/`CoverageDimension`/`lease` 等 React 字段名混入。
- [ ] `code/web/static/app.test.js` 若含相关断言，同步更新并通过。

## 7. 给 GLM 的注意事项

- **改前先读**：`doc/core/04-api.md`（接口契约）、`05-ui.md`（页面/颜色/组件规范）、
  `handlers_config.go` 的 DTO（字段名真值）、`worker.go`（单写者/panic 隔离，别破坏）。
- **动线对齐、视觉自便**：按钮位置/配色以宪法 `05-ui.md` 为准；本文件只规定"动作序列"。
- **最小改动**：现有能满足的（筛选/分页/CRUD/抽屉/内联编辑）**不要重写**，只加/改本文件点名处。
- **后端只此一处**：除 T2 的 `store_sids[]` 穿透外，**不新增/不修改任何其它后端接口**。
  重试复用 `POST /api/sync/{name}`、取消复用 `/cancel`、筛选复用 `GET /api/tasks`——全是现成的。
- **fail-loud**：拿不到数据就 toast 报错，别静默兜底成空白页（宪法通则）。
- **不确定就停**：若发现本文件与宪法 `05-ui.md` 有硬冲突，**先记进 `doc/core/findings.md` 并停下问**，不要自行发挥。





