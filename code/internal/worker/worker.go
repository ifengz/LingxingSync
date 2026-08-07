// Package worker 的 worker.go 实现单个「账号+接口」的同步执行单元 EndpointWorker。
//
// 这是「各接口互不影响」宪法硬要求的落地核心：
//   - §2/§5：每个 worker 一个独立 goroutine 跑 Run，独立限流器。
//   - §5 主循环：select trigger/cancel/ctx.Done，每次执行 doSync。
//   - §5 panic 隔离：doSync 外层 defer recover，只 recover 自己，绝不传播，
//     recover 后 worker 继续等下一次触发（不让一个接口的崩溃拖垮全局）。
//   - §6 限流：每次 client.Fetch 前 limiter.Wait，按 (quota_group, path) 共享桶。
//   - §10 多店铺迭代：IterateByStore 时对每个 sid 跑一次，店铺间 sleep multi_interval_ms。
//   - 单写者原则：只有 worker 自己写它的 sync_tasks 状态行（通过 db.UpdateTask）。
//
// 依赖（由其他 agent 实现，按既定签名调用）：
//   - internal/db：InsertTask / UpdateTask / CancelTask / InsertTaskLog / UpsertRows /
//     GetTableColumns / QuerySIDsForAccount。
//   - internal/api：NewClient / Client.Fetch / FetchResult / DefaultPageSize。
package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

// WorkerStatus 是 worker 的状态快照，给 /api/status 序列化用。
// 所有字段都是值拷贝，调用方修改不影响 worker 内部状态。
type WorkerStatus struct {
	Name         string     `json:"name"`
	Display      string     `json:"display"`
	AccountID    string     `json:"account_id"`
	Status       string     `json:"status"` // idle|running|error|disabled
	LastRunAt    *time.Time `json:"last_run_at,omitempty"`
	LastStatus   string     `json:"last_status,omitempty"` // 上次同步结果：success|error|cancelled
	LastRecords  int        `json:"last_records"`
	NextRunAt    *time.Time `json:"next_run_at,omitempty"`
	TodayRecords int        `json:"today_records"`
	TodayErrors  int        `json:"today_errors"`
}

// EndpointWorker 是单个「账号+接口」的同步执行单元。
//
// 字段分两类：
//   - 启动期不变量：Endpoint / Account / Client / DB / Limiters / Columns。
//   - 运行时状态：trigger / cancelCh 通道、running 标志、currentTaskID、
//     状态快照（mu 保护）。
type EndpointWorker struct {
	Endpoint config.Endpoint
	Account  config.Account
	Client   *api.Client
	DB       *sqlx.DB
	Limiters *LimiterRegistry
	Columns  []string // 启动时 GetTableColumns 缓存，避免每页查表

	// 触发与取消信号
	trigger  chan triggerReq // 非阻塞触发，携带类型与可选按次店铺，缓冲 1
	cancelCh chan int64      // 带 task id 的取消信号

	// 运行态
	running       atomic.Bool  // 是否正在同步，避免同 worker 重入
	currentTaskID atomic.Int64 // 当前任务 id（panic 兜底时用来标 error）

	// 状态快照（给 /api/status），mu 保护
	mu           sync.RWMutex
	lastStatus   string    // 上次同步结果：success|error|cancelled
	lastRunAt    time.Time // 上次同步开始时刻
	lastRecords  int       // 上次同步落库行数
	nextRunAt    time.Time // scheduler 设置的下次触发时刻
	todayRecords int       // 当日累计落库行数
	todayErrors  int       // 当日累计错误次数
	today        string    // 当日日期（YYYY-MM-DD），用于跨天重置 today*
}

// New 构造一个 EndpointWorker。
// 启动断言：调 db.GetTableColumns 缓存 Columns，缺表返回 error（fail-loud）。
// 限流器从 Limiters 注册表取（同 (quota_group, path) 共享）。
func New(ep config.Endpoint, acc config.Account, client *api.Client, dbx *sqlx.DB, reg *LimiterRegistry) (*EndpointWorker, error) {
	cols, err := db.GetTableColumns(dbx, ep.Table)
	if err != nil {
		return nil, fmt.Errorf("worker %s: 读表 %s 列定义失败: %w", ep.Name, ep.Table, err)
	}
	if len(cols) == 0 {
		return nil, fmt.Errorf("worker %s: 表 %s 无列定义（建表了吗？）", ep.Name, ep.Table)
	}
	// 预热限流器（同 key 共享桶，Get 即创建）
	reg.Get(acc.QuotaGroupOrID(), ep.Path, ep.Rate.Bucket, ep.Rate.IntervalMs)

	w := &EndpointWorker{
		Endpoint: ep,
		Account:  acc,
		Client:   client,
		DB:       dbx,
		Limiters: reg,
		Columns:  cols,
		trigger:  make(chan triggerReq, 1), // 缓冲 1：非阻塞 Trigger
		cancelCh: make(chan int64, 1),
		today:    time.Now().Format("2006-01-02"),
	}
	return w, nil
}

// triggerReq 是一次触发请求。kind 区分 cron/manual；
// storeSids 仅 manual 且用户按次指定时非空（nil/空 = 沿用配置白名单）。
//
// 无锁说明（10-frontend-rework-flow.md §3.1）：storeSids 随 trigger 通道发给
// 该接口自己的 goroutine，在它的 doSync 里消费，全程只有 owning goroutine 触碰。
// 不要为它加任何 mutex、不要放进共享 map 再加锁——channel 传值本身就是同步边界。
type triggerReq struct {
	kind      string   // "cron" | "manual"
	storeSids []string // 仅 manual 且用户按次指定时非空；nil/空 = 沿用配置白名单
}

// Trigger 非阻塞发送定时调度信号。返回 false 表示已有任务在队列中。
func (w *EndpointWorker) Trigger() bool {
	return w.send(triggerReq{kind: "cron"})
}

// TriggerManual 非阻塞发送手动同步信号。storeSids 为按次指定的店铺子集，
// 空则沿用 endpoint 配置的 StoreSids 白名单。返回 false 表示已有任务在队列中。
func (w *EndpointWorker) TriggerManual(storeSids []string) bool {
	return w.send(triggerReq{kind: "manual", storeSids: storeSids})
}

func (w *EndpointWorker) send(req triggerReq) bool {
	select {
	case w.trigger <- req:
		return true
	default:
		return false
	}
}

// Cancel 发取消信号，携带要取消的 task id。
// server 层调 db.CancelTask 后，调本方法让正在跑的循环尽快 ctx.Done。
func (w *EndpointWorker) Cancel(taskID int64) {
	select {
	case w.cancelCh <- taskID:
	default:
	}
}

// SetNextRun 由 scheduler 设置下次 cron 触发时刻，仅写快照。
func (w *EndpointWorker) SetNextRun(t time.Time) {
	w.mu.Lock()
	w.nextRunAt = t
	w.mu.Unlock()
}

// UpdateEndpoint 热更新 worker 的 Endpoint 快照（配置热加载用，由 Registry.ApplyHotReload 调用）。
// 用现有 mu 加锁保护，整体替换：结构性字段（table/account/path 等）变了本该走完整
// 重启，调用方不负责区分，这里只整体替换；运行中的同步只会看到 rate/cron/enabled/
// store_sids 等「可热加载」字段的差异生效于下一次触发。
func (w *EndpointWorker) UpdateEndpoint(ep config.Endpoint) {
	w.mu.Lock()
	w.Endpoint = ep
	w.mu.Unlock()
}

// Run 是 worker 的主循环，应在独立 goroutine 中调用。
// select trigger / cancel / ctx.Done；每次 trigger 执行一次 doSync。
// 外层 defer recover 保证即使 doSync 内部 panic 也不传播（宪法 §5 硬要求）。
//
// 注意：recover 只 recover 自己这个 goroutine——这正是「互不影响」的实现。
func (w *EndpointWorker) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case taskID := <-w.cancelCh:
			// 取消信号：如果当前正在跑且 taskID 匹配，靠 ctx 完成；
			// 这里只做一次状态更新尝试（实际取消由正在跑的 doSync 通过 ctx 感知）。
			// 由于本实现单 worker 串行，cancel 主要靠触发 ctx 之上的派生 ctx；
			// 简化处理：记录一条日志，并尝试置 task 为 cancelled（若已结束则无影响）。
			_ = db.CancelTask(w.DB, taskID)
			log.Printf("[worker:%s] 收到取消信号 task=%d", w.Endpoint.Name, taskID)
			continue
		case req := <-w.trigger:
			// 收到触发：执行同步，带 panic 隔离
			w.runOnceSafely(ctx, req)
		}
	}
}

// runOnceSafely 执行一次 doSync，外层 defer recover 作为兜底防线。
//
// panic 处理分两层（宪法 §5：panic 只 recover 自己，不传播）：
//   - 主层：doSync 内部的 defer recover——doSync 注册了 defer，绝大多数 panic
//     在那里就地吸收并写 task/快照（含已累计的 records/pages）。
//   - 兜底层（本函数）：覆盖「doSync 的 defer 尚未注册就 panic」的极小窗口
//     （如 startSnapshot / CompareAndSwap 阶段崩）。此时 currentTaskID 多半还是 0，
//     故只更新快照，不强写 task。
//
// 两层 recover 共同保证：本函数永远不 panic 出去，worker 继续等下一次触发。
func (w *EndpointWorker) runOnceSafely(ctx context.Context, req triggerReq) {
	if !w.Endpoint.Enabled {
		// 禁用的接口：触发来了一律跳过（status 标 disabled 由 Status() 派生）
		return
	}
	defer func() {
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v", r)
			log.Printf("[worker:%s] 兜底 recover（doSync 入口前崩溃）: %s", w.Endpoint.Name, msg)
			if tid := w.currentTaskID.Load(); tid > 0 {
				_ = db.UpdateTask(w.DB, tid, "error", 0, 0, fmt.Errorf("%s", msg))
			}
			w.finishSnapshot("error", 0, true)
		}
	}()
	w.doSync(ctx, req)
}

// doSync 执行一次完整同步。宪法 §5 伪码落地。
//
// triggerType: "cron" | "manual"（手动触发时 server 层会调一个带 manual 的入口）。
// 失败不 return error：所有错误都写进 task/日志 + 更新快照，保证 worker 继续存活。
//
// 注意：本方法不 panic 出去（runOnceSafely 兜底），但内部仍可能 panic（API bug），
// 那正是 recover 的价值。
func (w *EndpointWorker) doSync(ctx context.Context, req triggerReq) {
	// 防重入：同一 worker 同一时刻只跑一个同步
	if !w.running.CompareAndSwap(false, true) {
		log.Printf("[worker:%s] 上一次同步还在跑，本次触发跳过", w.Endpoint.Name)
		return
	}
	defer w.running.Store(false)

	w.startSnapshot() // 标记 running + 记录开始时刻

	// 1. 建任务行
	taskID, err := db.InsertTask(w.DB, w.Endpoint.Name, w.Account.ID, req.kind)
	if err != nil {
		// 连任务都建不了，多半是 DB 故障：记快照后返回，等下次触发。
		log.Printf("[worker:%s] InsertTask 失败: %v", w.Endpoint.Name, err)
		w.finishSnapshot("error", 0, false)
		return
	}
	w.currentTaskID.Store(taskID)

	// 派生一个可被 cancel 通道取消的 ctx：监听 cancelCh 把 ctx 取消掉
	syncCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		select {
		case <-syncCtx.Done():
		case tid := <-w.cancelCh:
			if tid == 0 || tid == taskID {
				cancel()
			} else {
				// 不是给我的，放回去（best-effort）
				select {
				case w.cancelCh <- tid:
				default:
				}
			}
		}
	}()

	status := "success"
	totalRecords := 0
	totalPages := 0

	defer func() {
		// 收尾：写 task 最终状态 + 更新快照。
		// 若正在展开 panic（doSync 内部某行 panic），先 recover 防止继续传播，
		// 把状态改写为 error，并由本 defer 统一收尾——这样 runOnceSafely 的兜底
		// recover 不会触发（panic 在这里就被吸收）。
		// 宪法 §5：panic 只 recover 自己，不传播。
		if r := recover(); r != nil {
			msg := fmt.Sprintf("panic: %v", r)
			log.Printf("[worker:%s] doSync 内部崩溃，已就地 recover: %s", w.Endpoint.Name, msg)
			status = "error"
			finalErr := fmt.Errorf("%s", msg)
			_ = db.UpdateTask(w.DB, taskID, status, totalRecords, totalPages, finalErr)
			w.finishSnapshot(status, totalRecords, true)
			return
		}
		var finalErr error
		if status == "error" {
			finalErr = fmt.Errorf("同步过程出错，见 task_logs")
		}
		if uerr := db.UpdateTask(w.DB, taskID, status, totalRecords, totalPages, finalErr); uerr != nil {
			log.Printf("[worker:%s] UpdateTask(%d)=%s 失败: %v", w.Endpoint.Name, taskID, status, uerr)
		}
		w.finishSnapshot(status, totalRecords, status == "error")
	}()

	// 4. 单店铺 or 多店铺
	if w.Endpoint.IterateByStore {
		// 账号级同步闸门（migrations/004）：只迭代 store_sync_selection 里 enabled=1 的店铺；
		// 该账号从未保存过选择时退回全放行（向后兼容）。此闸门在 endpoint.StoreSids 白名单之上游。
		sids, qerr := db.QueryEnabledSIDsForAccount(w.DB, w.Account.ID)
		if qerr != nil {
			log.Printf("[worker:%s] QueryEnabledSIDsForAccount 失败: %v", w.Endpoint.Name, qerr)
			status = "error"
			return
		}
		// 按次覆盖（10-frontend-rework-flow.md §3.1）：
		//   manual 且 req.storeSids 非空 → 与账号真实店铺取交集（杜绝越权同步别的店铺）；
		//   否则走老路径 effectiveStoreSIDs（配置级 StoreSids 白名单，空=不过滤）。
		if req.kind == "manual" && len(req.storeSids) > 0 {
			sids = intersectSIDs(sids, req.storeSids)
		} else {
			sids = w.effectiveStoreSIDs(sids)
		}
		paramName := w.Endpoint.StoreParamName
		if paramName == "" {
			paramName = "sid"
		}
		for i, sid := range sids {
			if syncCtx.Err() != nil {
				status = "cancelled"
				return
			}
			params := w.baseParams()
			params[paramName] = sid
			rec, pages, ok := w.fetchAllPages(syncCtx, taskID, params)
			totalRecords += rec
			totalPages += pages
			if !ok {
				status = "error"
				return // 任一 sid 失败则 break（宪法 §10）
			}
			// 多店铺间隔（非最后一个）
			if i < len(sids)-1 && w.Endpoint.Rate.MultiIntervalMs > 0 {
				select {
				case <-syncCtx.Done():
					status = "cancelled"
					return
				case <-time.After(time.Duration(w.Endpoint.Rate.MultiIntervalMs) * time.Millisecond):
				}
			}
		}
		return
	}

	// 单店铺：直接一次 fetchAllPages
	rec, pages, ok := w.fetchAllPages(syncCtx, taskID, w.baseParams())
	totalRecords = rec
	totalPages = pages
	if !ok {
		status = "error"
		return
	}
	if syncCtx.Err() != nil {
		status = "cancelled"
		return
	}
}

// fetchAllPages 翻页拉取一个（店铺的）全量数据并落库。
// 返回 (records, pages, ok)；ok=false 表示中途出错（已记日志，调用方据此把 task 置 error）。
//
// 每页流程（宪法 §5）：
//  1. limiter.Wait（同 (quota_group, path) 共享桶）
//  2. client.Fetch
//  3. InsertTaskLog（每页一行，含 http_status/api_code/records/duration/err_raw）
//  4. UpsertRows（account_id 注入由 db 层负责）
//  5. 累计 pages/records；HasMore 则 offset+=pageSize 继续
func (w *EndpointWorker) fetchAllPages(ctx context.Context, taskID int64, params map[string]any) (int, int, bool) {
	limiter := w.Limiters.Get(w.Account.QuotaGroupOrID(), w.Endpoint.Path, w.Endpoint.Rate.Bucket, w.Endpoint.Rate.IntervalMs)
	pageSize := api.DefaultPageSize
	if pageSize <= 0 {
		pageSize = 200
	}

	totalRecords := 0
	pages := 0
	offset := 0

	for {
		// 取消检查
		if ctx.Err() != nil {
			return totalRecords, pages, false
		}

		// 限流（每页前等）
		if err := limiter.Wait(ctx); err != nil {
			// ctx 取消
			log.Printf("[worker:%s] 限流等待被取消: %v", w.Endpoint.Name, err)
			return totalRecords, pages, false
		}

		// 注入分页参数（拷贝避免复用底层 map）
		pageParams := make(map[string]any, len(params)+2)
		for k, v := range params {
			pageParams[k] = v
		}
		pageParams["offset"] = offset
		pageParams["length"] = pageSize

		start := time.Now()
		result, httpStatus, apiCode, err := w.Client.Fetch(ctx, w.Endpoint.Method, w.Endpoint.Path, pageParams)
		durationMs := int(time.Since(start).Milliseconds())

		// 错误处理：记日志后中止本接口
		if err != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, 0, durationMs, err.Error())
			log.Printf("[worker:%s] Fetch 出错 offset=%d: %v (http=%d code=%d)",
				w.Endpoint.Name, offset, err, httpStatus, apiCode)
			return totalRecords, pages, false
		}

		// 落库（空列表跳过）
		list := result.List
		if len(list) > 0 {
			if uerr := db.UpsertRows(w.DB, w.Endpoint.Table, list, w.Columns, w.Account.ID); uerr != nil {
				_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(list), durationMs, "upsert: "+uerr.Error())
				log.Printf("[worker:%s] UpsertRows 出错 offset=%d: %v", w.Endpoint.Name, offset, uerr)
				return totalRecords, pages, false
			}
		}

		totalRecords += len(list)
		pages++
		_ = db.InsertTaskLog(w.DB, taskID, pages, httpStatus, apiCode, len(list), durationMs, "")

		// 翻页判定：HasMore 且本页有数据则继续
		if !result.HasMore || len(list) == 0 {
			return totalRecords, pages, true
		}
		offset += pageSize
	}
}

// baseParams 构造一个店铺无关的基础参数集合：
//   - 复制 endpoint.ExtraParams（string 化值）
//   - 若 WindowDays>0：注入 start_date/end_date（近 N 天，YYYY-MM-DD）
func (w *EndpointWorker) baseParams() map[string]any {
	params := make(map[string]any, len(w.Endpoint.ExtraParams)+2)
	for k, v := range w.Endpoint.ExtraParams {
		params[k] = stringifyParam(v)
	}
	if w.Endpoint.WindowDays > 0 {
		now := time.Now()
		start := now.AddDate(0, 0, -w.Endpoint.WindowDays)
		params["start_date"] = start.Format("2006-01-02")
		params["end_date"] = now.Format("2006-01-02")
	}
	return params
}

// stringifyParam 把 yaml 解析出来的 any（可能是 int/float64/bool）转成字符串，
// 因为领星 API 的 extra_params 多为字符串枚举。已经是 string 的原样返回。
func stringifyParam(v any) any {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%g", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		return fmt.Sprintf("%v", x)
	}
}

// startSnapshot 同步开始时更新快照：status=running，记录开始时刻。
func (w *EndpointWorker) startSnapshot() {
	w.mu.Lock()
	w.lastRunAt = time.Now()
	w.lastStatus = ""
	w.mu.Unlock()
}

// finishSnapshot 同步结束时更新快照：lastStatus/lastRecords/nextRun 清空，today* 累计。
// 跨天重置 today*。
func (w *EndpointWorker) finishSnapshot(status string, records int, isErr bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastStatus = status
	w.lastRecords = records
	w.nextRunAt = time.Time{} // 已执行完，下次触发时刻由 scheduler 重新设置

	today := time.Now().Format("2006-01-02")
	if w.today != today {
		// 跨天：重置当日计数
		w.today = today
		w.todayRecords = 0
		w.todayErrors = 0
	}
	w.todayRecords += records
	if isErr {
		w.todayErrors++
	}
}

// effectiveStoreSIDs 用 endpoint.StoreSids 白名单过滤 all（保持 all 的顺序）。
// StoreSids 为空表示不限制，直接返回 all（不拷贝，热加载前后行为一致）；
// 非空则只保留同时出现在 all 与 StoreSids 中的 sid（交集），顺序以 all 为准。
func (w *EndpointWorker) effectiveStoreSIDs(all []string) []string {
	w.mu.RLock()
	whitelist := w.Endpoint.StoreSids
	w.mu.RUnlock()

	if len(whitelist) == 0 {
		return all
	}
	allow := make(map[string]struct{}, len(whitelist))
	for _, sid := range whitelist {
		allow[sid] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for _, sid := range all {
		if _, ok := allow[sid]; ok {
			out = append(out, sid)
		}
	}
	return out
}

// intersectSIDs 求 all 与 wanted 的交集，顺序以 all（账号真实店铺集）为准。
// 用于手动同步按次指定店铺：传入的 wanted 必须先与账号真实存在的 sid 取交集，
// 杜绝越权同步别的账号/不存在的店铺。纯函数，不触碰任何共享状态。
func intersectSIDs(all []string, wanted []string) []string {
	if len(wanted) == 0 {
		return nil
	}
	allow := make(map[string]struct{}, len(wanted))
	for _, sid := range wanted {
		allow[sid] = struct{}{}
	}
	out := make([]string, 0, len(all))
	for _, sid := range all {
		if _, ok := allow[sid]; ok {
			out = append(out, sid)
		}
	}
	return out
}

// Status 返回当前 worker 的状态快照拷贝。线程安全。
// status 字段派生：enabled=false → disabled；running → running；否则按 lastStatus。
func (w *EndpointWorker) Status() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	st := "idle"
	switch {
	case !w.Endpoint.Enabled:
		st = "disabled"
	case w.running.Load():
		st = "running"
	case w.lastStatus == "error":
		st = "error"
	case w.lastStatus == "success":
		st = "idle"
	}

	var lastRun, nextRun *time.Time
	if !w.lastRunAt.IsZero() {
		t := w.lastRunAt
		lastRun = &t
	}
	if !w.nextRunAt.IsZero() {
		t := w.nextRunAt
		nextRun = &t
	}

	return WorkerStatus{
		Name:         w.Endpoint.Name,
		Display:      w.Endpoint.Display,
		AccountID:    w.Account.ID,
		Status:       st,
		LastRunAt:    lastRun,
		LastStatus:   w.lastStatus,
		LastRecords:  w.lastRecords,
		NextRunAt:    nextRun,
		TodayRecords: w.todayRecords,
		TodayErrors:  w.todayErrors,
	}
}
