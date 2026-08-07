// Package worker 的 scheduler.go 实现 cron 调度与日志留存清理。
//
// 职责（宪法 §3）：
//   - 对每个 enabled 的 endpoint，按 ep.Cron 注册一个 cron 任务，
//     触发时向对应 Worker 发 Trigger 信号（非阻塞）。
//   - 注册成功后，用 cron Entry.Next 计算下次触发时刻，调 worker.SetNextRun，
//     供 /api/status 展示 next_run_at。
//   - 注册一个 retention cron（cfg.Retention.CleanupCron），周期调 db.CleanupOld
//     清理过期 task_logs / tasks。
//
// 宪法对应：§3（cron 触发，互相独立）、§4（留存策略）。
//
// 注意：scheduler 只「发信号」，不「执行同步」。执行仍在各 worker 自己的 goroutine 里
// （§5/§2 互不影响的保证在 worker 层，不在 scheduler 层）。
package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/robfig/cron/v3"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

// Scheduler 包装 robfig/cron，负责按配置驱动 worker。
type Scheduler struct {
	cron *cron.Cron
	reg  *Registry
	dbx  *sqlx.DB // retention 清理用（db.CleanupOld 需要 db 句柄）

	// mu 保护 cfg 与 entries：Start 时单线程写入本无需加锁，但 Rebuild（热加载）
	// 之后 entries 可能被并发重建/回填，加锁避免并发读写 map。
	mu  sync.Mutex
	cfg *config.Config
	// entryID → worker，用于回填 nextRunAt（robfig/cron v3 的 Entry 不带自定义 tag，
	// 无法反查 endpoint name，故自己维护映射）。
	entries map[cron.EntryID]*EndpointWorker
}

// NewScheduler 构造调度器。不立即启动 cron。
//
// 参数：
//   - cfg：配置（取 Endpoints 的 Cron、Retention 的 CleanupCron）
//   - reg：worker 注册表（按 endpoint.Name 取 worker）
//   - dbx：数据库句柄，retention cron 调 db.CleanupOld 时用
func NewScheduler(cfg *config.Config, reg *Registry, dbx *sqlx.DB) *Scheduler {
	return &Scheduler{
		cron: cron.New(cron.WithChain(
			cron.Recover(cron.DefaultLogger), // 防止 cron job 内 panic 让整个调度器崩溃
		)),
		reg:     reg,
		cfg:     cfg,
		dbx:     dbx,
		entries: make(map[cron.EntryID]*EndpointWorker),
	}
}

// Start 注册所有 cron 任务并启动调度器。
// 调用方负责在退出时调 Stop()。
//
// 注册失败（cron 表达式非法）立即返回 error（启动期 fail-loud）。
func (s *Scheduler) Start(ctx context.Context) error {
	// 1. 每个 endpoint 一个 cron 任务（注册逻辑与 Rebuild 共用，见 registerEndpointJobsLocked）
	s.mu.Lock()
	err := s.registerEndpointJobsLocked(s.cfg)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	// 2. retention 清理 cron（始终注册，即便无 worker；不参与 Rebuild 热加载）
	s.mu.Lock()
	cleanupCron := s.cfg.Retention.CleanupCron
	s.mu.Unlock()
	if cleanupCron != "" {
		_, err := s.cron.AddFunc(cleanupCron, func() {
			s.runCleanup(ctx)
		})
		if err != nil {
			return fmt.Errorf("注册 retention cron 失败 spec=%q: %w", cleanupCron, err)
		}
	}

	s.cron.Start()

	// 3. Start 之后再回填每个 worker 的 nextRunAt（此时 cron 已算出 Next）
	s.backfillNextRuns()

	return nil
}

// registerEndpointJobsLocked 为 cfg 中每个 enabled 的 endpoint 注册一个 cron 任务，
// 写入 s.entries。Start 和 Rebuild 共用本逻辑，调用方必须已持有 s.mu。
func (s *Scheduler) registerEndpointJobsLocked(cfg *config.Config) error {
	for i := range cfg.Endpoints {
		ep := cfg.Endpoints[i]
		if !ep.Enabled {
			// 禁用的不注册（即便注册了 worker.Trigger 也会因 Enabled 跳过，这里省一次调度）
			continue
		}
		w := s.reg.Get(ep.Name)
		if w == nil {
			// worker 未注册：跳过并记录（启动顺序问题，不应发生）
			log.Printf("[scheduler] endpoint %s 未找到对应 worker，跳过 cron 注册", ep.Name)
			continue
		}

		var entryID cron.EntryID
		entryID, err := s.cron.AddFunc(ep.Cron, func() {
			// 闭包捕获 w / entryID。
			// 注意：触发后 Entry.Next 才会更新为「再下一次」，故这里读到的 Next 是
			// 本次触发后的下一次。
			w.Trigger()
			if next, ok := s.peekNext(entryID); ok {
				w.SetNextRun(next)
			}
		})
		if err != nil {
			return fmt.Errorf("注册 cron 失败 endpoint=%s spec=%q: %w", ep.Name, ep.Cron, err)
		}
		s.entries[entryID] = w
	}
	return nil
}

// Rebuild 用新配置重建所有 endpoint 的 cron 任务（配置热加载入口）。
//
// 步骤：先移除 entries 里记录的所有旧 cron 条目、清空映射，写入新 cfg，再按新
// cfg 重新注册 enabled 的 endpoint（复用 registerEndpointJobsLocked，与 Start
// 完全一致的注册逻辑）。retention 清理 cron 只在 Start 时注册一次，不在 Rebuild
// 范围内（cron 表达式变更需要完整重启）。
//
// 重新注册若失败（cron 表达式非法）直接返回 error；此时旧条目已被移除，出错的
// 那个 endpoint 会暂时没有调度，需再次 Rebuild 或重启修复（fail-loud，不掩盖）。
func (s *Scheduler) Rebuild(cfg *config.Config) error {
	s.mu.Lock()
	for entryID := range s.entries {
		s.cron.Remove(entryID)
	}
	s.entries = make(map[cron.EntryID]*EndpointWorker)
	s.cfg = cfg
	err := s.registerEndpointJobsLocked(cfg)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	// cron 早已在跑（Rebuild 只会在 Start 之后调用），AddFunc 内部经 channel 请求
	// run() goroutine 算出 Next，返回后即可读到，不必等下一次触发才回填。
	s.backfillNextRuns()

	return nil
}

// peekNext 取某个 cron entry 的下次触发时刻。entry 不存在 / Next 未计算返回 (zero, false)。
func (s *Scheduler) peekNext(id cron.EntryID) (time.Time, bool) {
	entry := s.cron.Entry(id)
	if entry.Next.IsZero() {
		return time.Time{}, false
	}
	return entry.Next, true
}

// Stop 停止调度器，等待正在执行的 job 完成。
func (s *Scheduler) Stop() {
	if s.cron != nil {
		<-s.cron.Stop().Done()
	}
}

// backfillNextRuns 在 cron.Start（或 Rebuild）后，遍历 entryID→worker 映射，把 Next
// 写进 worker。robfig/cron v3 在 Start 后才填充 Entry.Next，故必须 Start 后调用。
//
// 先在锁内把映射拷出来再遍历：避免和 Rebuild 并发重建 s.entries 时读写同一个 map；
// peekNext/SetNextRun 各自内部持有自己的锁（s.cron 的内部锁 / worker.mu），不是
// s.mu，拷贝出来后在锁外调用不会有嵌套锁问题。
func (s *Scheduler) backfillNextRuns() {
	s.mu.Lock()
	snapshot := make(map[cron.EntryID]*EndpointWorker, len(s.entries))
	for id, w := range s.entries {
		snapshot[id] = w
	}
	s.mu.Unlock()

	for entryID, w := range snapshot {
		if next, ok := s.peekNext(entryID); ok {
			w.SetNextRun(next)
		}
	}
}

// runCleanup 执行一次留存清理。db.CleanupOld 由 db 包提供：
//
//	db.CleanupOld(db *sqlx.DB, taskLogsDays, tasksDays int) error
//
// 参数顺序：taskLogsDays 在前、tasksDays 在后（与 db 包定义一致）。
func (s *Scheduler) runCleanup(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	if s.dbx == nil {
		log.Printf("[scheduler] CleanupOld 跳过：db 句柄未注入")
		return
	}
	s.mu.Lock()
	taskLogsDays, tasksDays := s.cfg.Retention.TaskLogsDays, s.cfg.Retention.TasksDays
	s.mu.Unlock()
	if err := db.CleanupOld(s.dbx, taskLogsDays, tasksDays); err != nil {
		log.Printf("[scheduler] CleanupOld 失败: %v", err)
	}
}
