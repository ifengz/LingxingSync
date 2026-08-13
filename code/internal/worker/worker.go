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
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

// WorkerStatus 是 worker 的状态快照，由 Status() 返回。
// 现唯一消费者是 server 层的禁用闸门（手动触发时挡 status=="disabled" 的接口）；
// 原 /api/status 概览接口已随概览页一并删除。所有字段都是值拷贝，调用方修改不影响 worker 内部状态。
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

	// FatalError 非空 = 该接口启动断言未过（最常见：目标表没建），永久不可同步。
	// 此时 Status=="error"。它只影响这一个接口，其余接口与 HTTP 服务照常运行
	// （CLAUDE.md §1.1「任一接口挂掉不影响其他接口」）。
	FatalError string `json:"fatal_error,omitempty"`

	// Warnings 是不阻断同步、但需要人看见的问题。当前唯一来源：配置声明了某些列
	// （record_id_fields / field_paths 目标）而目标表里没有，这些字段会被 Upsert
	// 静默丢弃。同步照跑、状态照常，只是把静默故障摆到明面上（§8「缺列给出可读提示」）。
	Warnings []string `json:"warnings,omitempty"`
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
	Columns  []string        // 启动时 GetTableColumns 缓存，避免每页查表
	JSONCols map[string]bool // 启动时缓存 JSON 列，避免把空字符串写入 JSON

	// fatalErr 记录启动断言失败的原因（最常见：目标表未建）。非 nil 时该 worker
	// 永久拒绝同步，但仍然注册、仍然在 UI 上可见，不拖垮进程。
	// 只在 New 里赋值一次（goroutine 启动之前），之后只读，故无需加锁。
	fatalErr error

	// warnings 记录不阻断同步的配置/表结构不一致（当前：声明了列但表里没有）。
	// 与 fatalErr 同样只在 New 里赋值一次、之后只读，故无需加锁。
	warnings []string

	// 触发与取消信号
	trigger  chan triggerReq // 非阻塞触发，携带类型与可选按次店铺，缓冲 1
	cancelCh chan int64      // 带 task id 的取消信号

	// 运行态
	running       atomic.Bool  // 是否正在同步，避免同 worker 重入
	currentTaskID atomic.Int64 // 当前任务 id（panic 兜底时用来标 error）

	// 状态快照（由 Status() 读出，供禁用闸门判定），mu 保护
	mu           sync.RWMutex
	lastStatus   string    // 上次同步结果：success|error|cancelled
	lastRunAt    time.Time // 上次同步开始时刻
	lastRecords  int       // 上次同步落库行数
	nextRunAt    time.Time // scheduler 设置的下次触发时刻
	todayRecords int       // 当日累计落库行数
	todayErrors  int       // 当日累计错误次数
	today        string    // 当日日期（YYYY-MM-DD），用于跨天重置 today*

	// dailyProject runs only after the complete raw endpoint task succeeds.
	// It is injected before Run starts and remains process-local.
	dailyProject DailyProjector
}

// DailyProjector validates every target before publishing one atomic batch.
type DailyProjector func(context.Context, string, []DailyProjectionTarget, time.Time) error

// New 构造一个 EndpointWorker。
//
// 启动断言：调 db.GetTableColumns 缓存 Columns。缺表**不再返回 error 顶掉进程**——
// 那会让一个没建表的接口连带停掉其余所有接口和 HTTP 服务，违反 CLAUDE.md §1.1
// 「任一接口挂掉、报错、被限流，不影响其他接口」。改为把原因记进 w.fatalErr：
// 该接口永久 status=error、拒绝同步、每次触发写一条带原因的 error 任务行，
// 但仍然注册、仍在 UI 可见，其余 worker 与 :7799 照常运行。
// fail-loud 的落点从「进程自杀」下沉到「这一个接口大声失败」。
//
// 返回的 error 仅保留给「连 INFORMATION_SCHEMA 都查不动」这类全局 DB 故障之外的
// 未来用途；当前实现恒为 nil，调用方仍应检查（签名稳定，便于以后收紧）。
//
// 例外：endpoint.Probe=true 时跳过建表断言（探测模式，表尚未建，仅摸字段名）。
// 限流器从 Limiters 注册表取（同 (quota_group, path) 共享）。
func New(ep config.Endpoint, acc config.Account, client *api.Client, dbx *sqlx.DB, reg *LimiterRegistry) (*EndpointWorker, error) {
	var cols []string
	var jsonCols map[string]bool
	var fatalErr error
	if !ep.Probe {
		c, err := db.GetTableColumns(dbx, ep.Table)
		switch {
		case err != nil:
			fatalErr = fmt.Errorf("表 %s 不可用（建表了吗？）: %w", ep.Table, err)
		case len(c) == 0:
			fatalErr = fmt.Errorf("表 %s 无列定义（建表了吗？）", ep.Table)
		default:
			cols = c
		}
		if fatalErr == nil {
			jsonCols, err = db.GetJSONColumns(dbx, ep.Table)
			if err != nil {
				fatalErr = fmt.Errorf("表 %s 的 JSON 列元数据不可用: %w", ep.Table, err)
			}
		}
	}
	if fatalErr == nil && !ep.Probe {
		if ep.Table == "ls_vc_orders" {
			if !vcOrdersRecordIDsValid(ep.RecordIDFields) {
				fatalErr = fmt.Errorf("表 %s 的 record_id_fields 必须是 [vc_store_id local_po_number]", ep.Table)
			} else if err := db.ValidateVCOrdersStoreScope(dbx, ep.Table); err != nil {
				fatalErr = err
			}
		}
	}
	if fatalErr == nil && !ep.Probe {
		conflict, err := db.AccountMigrationConflict(dbx, ep.Table, ep.Account)
		if err != nil {
			// 冲突记录表只由 015 创建；旧库尚未完成该迁移时不把查询失败扩大成全站故障。
			log.Printf("[worker:%s] 读取账号迁移冲突告警失败，同步继续: %v", ep.Name, err)
		} else if conflict != "" {
			fatalErr = fmt.Errorf("%s", conflict)
		}
	}
	// 声明了列但表里没有 → 告警（不阻断）。缺表时 cols 为空，谈不上列差集，跳过。
	var warnings []string
	if fatalErr == nil && len(cols) > 0 {
		if miss := missingDeclaredColumns(ep, cols); len(miss) > 0 {
			warnings = append(warnings, fmt.Sprintf(
				"表 %s 缺少配置声明的列 %v：这些字段会被静默丢弃（同步仍会执行）",
				ep.Table, miss))
		}
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
		JSONCols: jsonCols,
		fatalErr: fatalErr,
		warnings: warnings,
		trigger:  make(chan triggerReq, 1), // 缓冲 1：非阻塞 Trigger
		cancelCh: make(chan int64, 1),
		today:    time.Now().Format("2006-01-02"),
	}
	return w, nil
}

func vcOrdersRecordIDsValid(fields []string) bool {
	return len(fields) == 2 && fields[0] == "vc_store_id" && fields[1] == "local_po_number"
}

// FatalError 返回启动断言失败的原因，nil = 该接口可正常同步。
// server 层用它挡手动触发并回显原因；main 用它打启动告警。
func (w *EndpointWorker) FatalError() error { return w.fatalErr }

// SetDailyProjector wires the one allowed listing daily fact publisher. Main
// calls it before worker goroutines start, so no runtime synchronization is needed.
func (w *EndpointWorker) SetDailyProjector(project DailyProjector) { w.dailyProject = project }

// missingDeclaredColumns 返回配置声明过、但目标表实际不存在的列名（已排序去重）。
//
// 覆盖两类声明：
//   - record_id_fields：唯一键字段。Upsert 的去重完全依赖表的真实主键，config 里的
//     record_id_fields 运行期并不参与 SQL。两者对不上时同步会「成功」但去重语义
//     可能是错的（重复行堆积或整表被当成一行覆盖），且不报任何错。
//   - field_paths 的键（目标列名）：辛苦从嵌套结构里捞出来提升到顶层，表里却没这列，
//     Upsert 直接丢掉——纯属白干，同样不报错。
//
// 为什么是告警而不是 fatal：这两类声明运行期都不参与 SQL，声明过时并不代表实际
// 落库是错的（表的真实主键可能完全正确）。做成 fatal 会因为一处元数据不一致，
// 拒绝同步一个本来跑得好的接口——那是拿修复的名义弄坏在跑的东西。所以只把它从
// 「静默」变成「可见」，落实 §8「缺列给出可读提示（缺哪几列）」。
//
// Probe 接口与缺表接口不会走到这里（New 里已用 len(cols)>0 挡掉）。
func missingDeclaredColumns(ep config.Endpoint, cols []string) []string {
	if ep.Probe || len(cols) == 0 {
		return nil
	}
	have := make(map[string]struct{}, len(cols))
	for _, c := range cols {
		have[c] = struct{}{}
	}
	seen := make(map[string]struct{})
	var missing []string
	add := func(f string) {
		if f == "" {
			return
		}
		if _, ok := have[f]; ok {
			return
		}
		if _, dup := seen[f]; dup {
			return
		}
		seen[f] = struct{}{}
		missing = append(missing, f)
	}
	for _, f := range ep.RecordIDFields {
		add(f)
	}
	for target := range ep.FieldPaths {
		add(target)
	}
	sort.Strings(missing)
	return missing
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
	dateFrom  string   // 仅支持日期范围的 manual endpoint 使用；空=沿用默认窗口
	dateTo    string
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

// TriggerManualWithRange queues one manual run with a transient date range.
// The range is carried only by this request and never writes endpoint defaults.
func (w *EndpointWorker) TriggerManualWithRange(storeSids []string, dateFrom, dateTo string) bool {
	return w.send(triggerReq{kind: "manual", storeSids: storeSids, dateFrom: dateFrom, dateTo: dateTo})
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
	if w.fatalErr != nil {
		// 启动断言未过（最常见：目标表没建）。不碰领星、不写数据表，
		// 只落一条 error 任务行，让「同步日志」页看得见原因。
		// 绝不 panic、绝不退进程——爆炸半径限定在这一个接口。
		w.recordFatalTask(req.kind)
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
	dailyTargets := make([]DailyProjectionTarget, 0)

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
		if status == "success" {
			if projectErr := w.projectDaily(syncCtx, dailyTargets); projectErr != nil {
				status = "error"
				finalErr = projectErr
				_ = db.InsertTaskLog(w.DB, taskID, totalPages+1, 0, 0, 0, 0, "daily projection: "+projectErr.Error())
				log.Printf("[worker:%s] 日维投影失败: %v", w.Endpoint.Name, projectErr)
			}
		}
		if status == "error" {
			if finalErr == nil {
				finalErr = fmt.Errorf("同步过程出错，见 task_logs")
			}
		}
		if uerr := db.UpdateTask(w.DB, taskID, status, totalRecords, totalPages, finalErr); uerr != nil {
			log.Printf("[worker:%s] UpdateTask(%d)=%s 失败: %v", w.Endpoint.Name, taskID, status, uerr)
		}
		w.finishSnapshot(status, totalRecords, status == "error")
	}()

	// 4. 单店铺 or 多店铺
	if w.Endpoint.IterateByVCOrders {
		records, pages, syncErr := w.syncVCPODetails(syncCtx, taskID, req)
		totalRecords = records
		totalPages = pages
		if syncErr == nil {
			return
		}
		if syncCtx.Err() != nil {
			status = "cancelled"
			return
		}
		status = "error"
		return
	}

	if w.Endpoint.IterateByAdAccount {
		accounts, qerr := db.QueryEnabledAdAccountsForAccount(w.DB, w.Account.ID, w.Endpoint.AdAccountType)
		if qerr != nil {
			log.Printf("[worker:%s] QueryEnabledAdAccountsForAccount 失败: %v", w.Endpoint.Name, qerr)
			status = "error"
			return
		}
		if len(accounts) == 0 {
			log.Printf("[worker:%s] 没有可用的 seller 广告账号，拒绝把未同步误报为 success", w.Endpoint.Name)
			status = "error"
			return
		}
		if req.kind == "manual" && len(req.storeSids) > 0 {
			accounts = filterAdAccountsByStoreSIDs(accounts, req.storeSids)
		}
		for i, account := range accounts {
			if syncCtx.Err() != nil {
				status = "cancelled"
				return
			}
			sets, paramErr := w.paramSetsFor(req)
			if paramErr != nil {
				log.Printf("[worker:%s] 请求日期参数无效: %v", w.Endpoint.Name, paramErr)
				status = "error"
				return
			}
			for _, params := range sets {
				params["sid"] = account.SID
				params["profile_id"] = account.ProfileID
			}
			rec, pages, ok := forEachParamSet(sets, func(params map[string]any) (int, int, bool) {
				return w.fetchAllPages(syncCtx, taskID, params)
			})
			totalRecords += rec
			totalPages += pages
			if !ok {
				status = "error"
				return
			}
			dailyTargets = append(dailyTargets, projectionTargets(w.Endpoint, account.SID, sets, time.Now())...)
			if i < len(accounts)-1 && w.Endpoint.Rate.MultiIntervalMs > 0 {
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

	if w.Endpoint.IterateByStore {
		// 账号级同步闸门（migrations/004）：只迭代 store_sync_selection 里 enabled=1 的店铺；
		// 该账号从未保存过选择时退回全放行（向后兼容）。此闸门在 endpoint.StoreSids 白名单之上游。
		sids, qerr := db.QueryEnabledSIDsForAccount(w.DB, w.Account.ID, w.Endpoint.StoreType)
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
			sets, paramErr := w.paramSetsFor(req)
			if paramErr != nil {
				log.Printf("[worker:%s] 请求日期参数无效: %v", w.Endpoint.Name, paramErr)
				status = "error"
				return
			}
			for _, params := range sets {
				params[paramName] = sid
			}
			rec, pages, ok := forEachParamSet(sets, func(params map[string]any) (int, int, bool) {
				return w.fetchAllPages(syncCtx, taskID, params)
			})
			totalRecords += rec
			totalPages += pages
			if !ok {
				status = "error"
				return // 任一 sid 失败则 break（宪法 §10）
			}
			dailyTargets = append(dailyTargets, projectionTargets(w.Endpoint, sid, sets, time.Now())...)
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
	sets, paramErr := w.paramSetsFor(req)
	if paramErr != nil {
		log.Printf("[worker:%s] 请求日期参数无效: %v", w.Endpoint.Name, paramErr)
		status = "error"
		return
	}
	rec, pages, ok := forEachParamSet(sets, func(params map[string]any) (int, int, bool) {
		return w.fetchAllPages(syncCtx, taskID, params)
	})
	totalRecords = rec
	totalPages = pages
	if !ok {
		status = "error"
		return
	}
	dailyTargets = append(dailyTargets, projectionTargets(w.Endpoint, stringParam(sets[0], w.Endpoint.StoreParamName), sets, time.Now())...)
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

		// 注入分页参数（拷贝避免复用底层 map）
		pageParams := make(map[string]any, len(params)+2)
		for k, v := range params {
			pageParams[k] = v
		}
		pageParams["offset"] = offset
		pageParams["length"] = pageSize

		// 抓取（含限流 + 可恢复失败重试）：每次尝试（含每次重试）都先过同一个
		// (quota_group,path) 桶；网络抖动/429/5xx 指数退避重试，4xx/业务错/取消
		// 不重试（见 fetchPageWithRetry / retryableFetchFailure），fail-loud 交由
		// 下方错误分支处理。整个重试内包在本 goroutine，绝不牵动别的接口。
		result, httpStatus, apiCode, durationMs, err := w.fetchPageWithRetry(ctx, limiter, w.Endpoint.Method, w.Endpoint.Path, pageParams)

		// 错误处理：记日志后中止本接口
		if err != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, 0, durationMs, err.Error())
			log.Printf("[worker:%s] Fetch 出错 offset=%d: %v (http=%d code=%d)",
				w.Endpoint.Name, offset, err, httpStatus, apiCode)
			return totalRecords, pages, false
		}

		// 探测模式：不落库，把原始响应 JSON 存进 task_logs.error_raw 供读字段名。
		// 首页样本足够（字段名前后页一致），存一页即停。
		if w.Endpoint.Probe {
			sample := probeSample(result)
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(result.List), durationMs, sample)
			log.Printf("[worker:%s] 探测模式 offset=%d: 抓到 %d 行，原始 JSON 已存 task_logs",
				w.Endpoint.Name, offset, len(result.List))
			totalRecords += len(result.List)
			pages++
			return totalRecords, pages, true
		}

		// 落库（空列表跳过）
		list := result.List
		// 行整形（宪法 §1.3 零代码：全部由 field_paths / inject_params 配置驱动）：
		//   - field_paths：把嵌套里的身份字段提到顶层（如 asins[0].asin → asin）
		//   - inject_params：把请求参数补进行（如迭代的 sid，领星不回显）
		//   - force_inject_params：对已确认会发生大整数精度丢失的字段强制使用请求值
		// 两者都不配则原样透传，行为与改动前完全一致。
		if uerr := shapeRows(list, w.Endpoint.FieldPaths, w.Endpoint.InjectParams, w.Endpoint.ForceInjectParams, pageParams); uerr != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(list), durationMs, "shape: "+uerr.Error())
			log.Printf("[worker:%s] 行整形出错 offset=%d: %v", w.Endpoint.Name, offset, uerr)
			return totalRecords, pages, false
		}
		if uerr := injectRowDate(list, w.Endpoint.RowDateField, w.Endpoint.WindowStartFieldOrDefault(), pageParams); uerr != nil {
			_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(list), durationMs, "row date: "+uerr.Error())
			return totalRecords, pages, false
		}
		if len(list) > 0 {
			if uerr := db.UpsertRows(w.DB, w.Endpoint.Table, list, w.Columns, w.JSONCols, w.Account.ID); uerr != nil {
				_ = db.InsertTaskLog(w.DB, taskID, pages+1, httpStatus, apiCode, len(list), durationMs, "upsert: "+uerr.Error())
				log.Printf("[worker:%s] UpsertRows 出错 offset=%d: %v", w.Endpoint.Name, offset, uerr)
				return totalRecords, pages, false
			}
		}

		totalRecords += len(list)
		pages++
		_ = db.InsertTaskLog(w.DB, taskID, pages, httpStatus, apiCode, len(list), durationMs, "")

		// 翻页判定（宪法 §4 doc/core/08-api-reference.md：has_more==false 或
		// offset+length>=total 终止）。fetched = 旧 offset + 本页行数 = 已累计记录数。
		fetched := offset + len(list)
		if !shouldContinuePaging(result, len(list), fetched) {
			return totalRecords, pages, true
		}

		// 运行时安全阀：领星若忽略 offset 或谎报 total 会导致死循环——达上限主动
		// fail-loud 中止（ok=false → task 置 error），不静默截断也不空转。
		if pages >= maxPagesPerSync {
			msg := fmt.Sprintf("翻页超过安全上限 %d 页仍未终止（offset=%d total=%d），疑似 API 不认 offset 或 total 异常，主动中止",
				maxPagesPerSync, offset, result.Total)
			_ = db.InsertTaskLog(w.DB, taskID, pages, httpStatus, apiCode, len(list), durationMs, msg)
			log.Printf("[worker:%s] %s", w.Endpoint.Name, msg)
			return totalRecords, pages, false
		}

		// offset 是「行游标」（宪法 §4 offset+length），按实际取得行数前进，
		// 兼容短页；对满页接口等价于 offset+=pageSize，行为不变。
		offset = fetched
	}
}

// maxPagesPerSync 是单次（单店铺）翻页的安全上限，防止领星忽略 offset 或谎报
// total 导致死循环。200 行/页 × 上限 = 数百万行容量，远超任何报表单次实际量；
// 触顶视为异常，fail-loud 中止（宪法 §5）。
const maxPagesPerSync = 10000

// shouldContinuePaging 依据领星分页契约（宪法 §4）判定是否继续翻页。纯函数，
// 便于单测（worker 持有具体 *api.Client，无接口 seam，无法在沙箱内跑 HTTP 全链路）。
//
// 终止（满足任一即停）：
//   - 本页空（pageLen==0）：无论 has_more/total 一律停，防呆杜绝死循环。
//   - has_more 字段「存在且为 false」：领星显式说没有更多，以它为准。
//   - has_more 字段「不存在」但 total>0 且已取满（fetched>=total）。
//
// 继续：
//   - has_more 字段「存在且为 true」。
//   - has_more 字段「不存在」、total>0 且 fetched<total（报表类接口只给 total 不给 has_more）。
//
// 无分页信号（has_more 不存在且 total<=0，如裸数组）→ 停在首页，绝不盲翻撞限流
// （对应 parse 层 TestParseFetchResultNoHasMoreInference 的谨慎原则）。
//
// fetched = 含本页在内已累计的记录数（旧 offset + 本页行数）。
func shouldContinuePaging(r *api.FetchResult, pageLen, fetched int) bool {
	if pageLen == 0 {
		return false
	}
	if r.HasMorePresent {
		return r.HasMore
	}
	if r.Total > 0 {
		return fetched < r.Total
	}
	return false
}

// probeSample 把探测模式抓到的结果拼成一段可读字符串存进 task_logs.error_raw，
// 便于离线读出领星返回的真实字段名。包含：首行样本 + 全量字段名清单 + 原始 JSON。
// 整体截断到 probeSampleMaxBytes：探测的用途是读字段名，fields 和 first_row
// 已经足够，原始 JSON 被截断不影响该用途；不截断则整行写不进库（见常量注释）。
func probeSample(result *api.FetchResult) string {
	var sb strings.Builder
	sb.WriteString("PROBE sample\n")
	if len(result.List) > 0 {
		keys := make([]string, 0, len(result.List[0]))
		for k := range result.List[0] {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		sb.WriteString("fields=" + strings.Join(keys, ",") + "\n")
		if b, err := json.Marshal(result.List[0]); err == nil {
			sb.WriteString("first_row=" + string(b) + "\n")
		}
	}
	sb.WriteString("raw=" + string(result.Raw))
	return truncateUTF8(sb.String(), probeSampleMaxBytes)
}

// probeSampleMaxBytes 是探测样本写入 sync_task_logs.error_raw 的字节上限。
// error_raw 是 MySQL TEXT（硬上限 65535 字节），超限 INSERT 直接报
// "Data too long"，而 worker 对写日志的错误是 `_ =` 忽略的 —— 结果是
// 「日志说已存 task_logs，库里却没有这一行」的静默丢样本。
// 宽响应接口（如 /erp/sc/data/mws/orders，200 行 × 40 字段带 item_list）
// 原始 JSON 轻易几百 KB，必须先截断。
// 留 5535 字节余量给 fields/first_row 前缀和多字节字符边界。
const probeSampleMaxBytes = 60000

// truncateUTF8 把 s 截断到不超过 max 字节，且不切断多字节字符
// （切断会产生非法 UTF-8，utf8mb4 列会拒收整行）。
func truncateUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	const mark = "\n...[truncated]"
	cut := max - len(mark)
	if cut < 0 {
		cut = 0
	}
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut-- // 退到 rune 边界
	}
	return s[:cut] + mark
}

// baseParams 构造一个店铺无关的基础参数集合：
//   - 复制 endpoint.ExtraParams（string 化值）
//   - 非 single-day 且 WindowDays>0 时，注入窗口起止日期（近 N 天，YYYY-MM-DD）
//   - single-day 的逐日窗口由 paramSetsForAt 构造
func (w *EndpointWorker) baseParams() map[string]any {
	params := make(map[string]any, len(w.Endpoint.ExtraParams)+2)
	for k, v := range w.Endpoint.ExtraParams {
		params[k] = stringifyParam(v)
	}
	if w.Endpoint.WindowDays > 0 && !w.Endpoint.SingleDayWindow {
		now := time.Now()
		start := now.AddDate(0, 0, -w.Endpoint.WindowDays)
		// 参数名可配：领星各接口命名不统一——SC/VC 订单用蛇形 start_date/end_date，
		// VC 报表族（sales/realtimeSales/traffic/inventory）用驼峰 startDate/endDate。
		// 曾经这里硬编码蛇形，导致 4 个 VC 报表始终 400「参数有误」。
		params[w.Endpoint.WindowStartFieldOrDefault()] = start.Format("2006-01-02")
		params[w.Endpoint.WindowEndFieldOrDefault()] = now.Format("2006-01-02")
	}
	// 单日期注入（报表类接口，如销量统计 event_date）：今天往前 DateOffsetDays 天。
	// 与上面的 window 范围互补；DateField 空则不生效。通用机制，不给单接口写死代码。
	if w.Endpoint.DateField != "" {
		d := time.Now().AddDate(0, 0, -w.Endpoint.DateOffsetDays)
		params[w.Endpoint.DateField] = d.Format("2006-01-02")
	}
	return params
}

func (w *EndpointWorker) baseParamsFor(req triggerReq) (map[string]any, error) {
	params := w.baseParams()
	if req.kind == "manual" && req.dateFrom != "" && req.dateTo != "" {
		if w.Endpoint.DateField != "" {
			params[w.Endpoint.DateField] = req.dateFrom
		} else {
			params[w.Endpoint.WindowStartFieldOrDefault()] = req.dateFrom
			params[w.Endpoint.WindowEndFieldOrDefault()] = req.dateTo
		}
	}
	return params, nil
}

func (w *EndpointWorker) paramSetsFor(req triggerReq) ([]map[string]any, error) {
	return w.paramSetsForAt(req, time.Now())
}

const maxSingleDayManualRangeDays = 92

func (w *EndpointWorker) paramSetsForAt(req triggerReq, now time.Time) ([]map[string]any, error) {
	if !w.Endpoint.SingleDayWindow {
		params, err := w.baseParamsFor(req)
		return []map[string]any{params}, err
	}
	if req.kind == "manual" && (req.dateFrom == "" || req.dateTo == "") {
		return nil, fmt.Errorf("single-day endpoint 手动同步必须明确提供 date_from/date_to")
	}
	var dates []time.Time
	if req.kind == "manual" {
		from, err := time.Parse("2006-01-02", req.dateFrom)
		if err != nil {
			return nil, fmt.Errorf("date_from 非法: %w", err)
		}
		to, err := time.Parse("2006-01-02", req.dateTo)
		if err != nil {
			return nil, fmt.Errorf("date_to 非法: %w", err)
		}
		if from.After(to) {
			return nil, fmt.Errorf("date_to 不能早于 date_from")
		}
		if to.After(from.AddDate(0, 0, maxSingleDayManualRangeDays-1)) {
			return nil, fmt.Errorf("single-day endpoint 手动同步范围不能超过 %d 个自然日", maxSingleDayManualRangeDays)
		}
		for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
			dates = append(dates, date)
		}
	} else {
		for offset := 0; offset < w.Endpoint.WindowDays; offset++ {
			dates = append(dates, now.AddDate(0, 0, -offset))
		}
	}
	sets := make([]map[string]any, 0, len(dates))
	for _, date := range dates {
		params := w.baseParams()
		value := date.Format("2006-01-02")
		params[w.Endpoint.WindowStartFieldOrDefault()] = value
		params[w.Endpoint.WindowEndFieldOrDefault()] = value
		sets = append(sets, params)
	}
	return sets, nil
}

func forEachParamSet(sets []map[string]any, fetch func(map[string]any) (int, int, bool)) (int, int, bool) {
	totalRecords, totalPages := 0, 0
	for _, params := range sets {
		records, pages, ok := fetch(params)
		totalRecords += records
		totalPages += pages
		if !ok {
			return totalRecords, totalPages, false
		}
	}
	return totalRecords, totalPages, true
}

type DailyProjectionTarget struct {
	Store   string
	Channel string
	Date    time.Time
}

func dailyProjectionChannel(table string) (string, bool) {
	channels := map[string]string{
		"ls_sc_sales_report":      "sc_fba",
		"ls_sc_sales_revenue":     "sc_fba",
		"ls_sc_refunds":           "sc_fba",
		"ls_fba_inventory":        "sc_fba",
		"ls_sc_performance_daily": "sc_fba",
		"ls_ad_sp_product":        "sc_fba",
		"ls_ad_sd_product":        "sc_fba",
		"ls_vc_sales_report":      "vc",
		"ls_vc_inventory":         "vc",
		"ls_ad_hsa_campaign":      "hsa",
	}
	channel, ok := channels[table]
	return channel, ok
}

func dailyProjectionDates(endpoint config.Endpoint, params map[string]any, now time.Time) ([]time.Time, error) {
	if endpoint.Table == "ls_fba_inventory" {
		return []time.Time{calendarDate(now)}, nil
	}
	if endpoint.DateField != "" {
		date, err := parseProjectionDate(stringParam(params, endpoint.DateField), endpoint.DateField)
		return oneDate(date, err)
	}
	startField := endpoint.WindowStartFieldOrDefault()
	endField := endpoint.WindowEndFieldOrDefault()
	start, err := parseProjectionDate(stringParam(params, startField), startField)
	if err != nil {
		return nil, err
	}
	end, err := parseProjectionDate(stringParam(params, endField), endField)
	if err != nil {
		return nil, err
	}
	if start.After(end) {
		return nil, fmt.Errorf("daily projection: %s is after %s", startField, endField)
	}
	dates := make([]time.Time, 0, int(end.Sub(start).Hours()/24)+1)
	for date := start; !date.After(end); date = date.AddDate(0, 0, 1) {
		dates = append(dates, date)
	}
	return dates, nil
}

func projectionTargets(endpoint config.Endpoint, store string, sets []map[string]any, now time.Time) []DailyProjectionTarget {
	channel, ok := dailyProjectionChannel(endpoint.Table)
	if !ok {
		return nil
	}
	targets := make([]DailyProjectionTarget, 0, len(sets))
	for _, params := range sets {
		dates, err := dailyProjectionDates(endpoint, params, now)
		if err != nil {
			return []DailyProjectionTarget{{Store: strings.TrimSpace(store), Channel: channel}}
		}
		for _, date := range dates {
			targets = append(targets, DailyProjectionTarget{Store: strings.TrimSpace(store), Channel: channel, Date: date})
		}
	}
	return targets
}

func (w *EndpointWorker) projectDaily(ctx context.Context, targets []DailyProjectionTarget) error {
	if _, ok := dailyProjectionChannel(w.Endpoint.Table); !ok {
		return nil
	}
	if w.dailyProject == nil {
		return fmt.Errorf("daily projection: publisher is not configured")
	}
	seen := make(map[string]struct{}, len(targets))
	unique := make([]DailyProjectionTarget, 0, len(targets))
	today := time.Now()
	for _, target := range targets {
		if target.Store == "" || target.Date.IsZero() {
			return fmt.Errorf("daily projection: store/date target is missing for table %s", w.Endpoint.Table)
		}
		key := target.Store + "\x00" + target.Channel + "\x00" + target.Date.Format("2006-01-02")
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, target)
	}
	if err := w.dailyProject(ctx, w.Account.ID, unique, today); err != nil {
		return fmt.Errorf("daily projection batch: %w", err)
	}
	return nil
}

func stringParam(params map[string]any, field string) string {
	if field == "" {
		field = "sid"
	}
	return strings.TrimSpace(fmt.Sprint(params[field]))
}

func parseProjectionDate(value, field string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("daily projection: missing %s", field)
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("daily projection: invalid %s %q: %w", field, value, err)
	}
	return date, nil
}

func oneDate(date time.Time, err error) ([]time.Time, error) {
	if err != nil {
		return nil, err
	}
	return []time.Time{date}, nil
}

func calendarDate(value time.Time) time.Time {
	year, month, day := value.Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
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
	case []any, map[string]any:
		// 数组/对象参数（如 VC 订单必填的 purchase_order_type:["1"]、vc_store_ids）原样透传，
		// 交给 POST body 的 json.Marshal 编成真正的 JSON 数组/对象。绝不能走下面的 %v 分支
		// 弄成 "[1]" 字符串——那样领星收到的是字符串而非数组，报「参数有误」。
		return x
	default:
		return fmt.Sprintf("%v", x)
	}
}

// recordFatalTask 为「启动断言未过」的接口落一条 error 任务行。
//
// 目的是可见性：缺表的接口不再静默躺着，也不再拖垮进程，而是每次被触发就在
// 「同步日志」页留下一条带原因的 error 记录，运维不用 SSH 看 stdout 就能定位。
// 全程不碰领星 API、不碰 ls_* 数据表。DB 本身也故障时只打日志，不升级为 panic。
func (w *EndpointWorker) recordFatalTask(triggerKind string) {
	log.Printf("[worker:%s] 拒绝同步（启动断言未过）: %v", w.Endpoint.Name, w.fatalErr)

	taskID, err := db.InsertTask(w.DB, w.Endpoint.Name, w.Account.ID, triggerKind)
	if err != nil {
		log.Printf("[worker:%s] InsertTask 失败（缺表告警未能落库）: %v", w.Endpoint.Name, err)
		w.finishSnapshot("error", 0, true)
		return
	}
	if err := db.UpdateTask(w.DB, taskID, "error", 0, 0, w.fatalErr); err != nil {
		log.Printf("[worker:%s] UpdateTask(%d)=error 失败: %v", w.Endpoint.Name, taskID, err)
	}
	w.finishSnapshot("error", 0, true)
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

// filterAdAccountsByStoreSIDs 是广告账号迭代的按次店铺过滤。
// 输入 accounts 已由 DB 限定为当前账号的有效广告账号；这里仅按手动请求再缩小范围。
func filterAdAccountsByStoreSIDs(accounts []db.AdAccountParams, storeSIDs []string) []db.AdAccountParams {
	allow := make(map[string]struct{}, len(storeSIDs))
	for _, sid := range storeSIDs {
		allow[sid] = struct{}{}
	}
	filtered := make([]db.AdAccountParams, 0, len(accounts))
	for _, account := range accounts {
		if _, ok := allow[account.SID]; ok {
			filtered = append(filtered, account)
		}
	}
	return filtered
}

// Status 返回当前 worker 的状态快照拷贝。线程安全。
// status 字段派生：enabled=false → disabled；running → running；否则按 lastStatus。
func (w *EndpointWorker) Status() WorkerStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()

	st := "idle"
	switch {
	// fatalErr 优先于 disabled：表没建是硬故障，即使接口被禁用也该显示原因，
	// 否则用户启用后才发现同步永远跑不起来。
	case w.fatalErr != nil:
		st = "error"
	case !w.Endpoint.Enabled:
		st = "disabled"
	case w.running.Load():
		st = "running"
	case w.lastStatus == "error":
		st = "error"
	case w.lastStatus == "success":
		st = "idle"
	}

	var fatal string
	if w.fatalErr != nil {
		fatal = w.fatalErr.Error()
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
		FatalError:   fatal,
		Warnings:     w.warnings, // 启动期一次性算好，之后只读
	}
}
