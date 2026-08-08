// Package main 是领星同步机的入口。
//
// 启动顺序（宪法 doc/01-architecture.md §10.2）：
//  1. 加载配置（fail-loud 校验）
//  2. 连 MySQL + 跑迁移（CREATE TABLE IF NOT EXISTS，幂等）
//  3. 构造每个 Worker（缺表 → GetTableColumns 报错 → FATAL 退出，启动断言）
//  4. 先启动 is_store_source=true 的 Worker（店铺列表就绪后，iterate_by_store 才有 sid）
//  5. 再启动其余 Worker
//  6. 启动 cron 调度 + retention 清理
//  7. 启动 HTTP :7799（go:embed web/ 资源注入 server）
//
// flags:
//
//	-config   配置文件路径（默认 config.yaml）
//	-base-url 领星 OpenAPI 根，默认 https://openapi.lingxing.com
package main

import (
	"context"
	"embed"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
	"lingxing-sync/internal/server"
	"lingxing-sync/internal/worker"
)

// webFS 整体 embed web/ 目录（templates/ + static/ 都是它的子目录）。
// 入口层在仓库根 code/ 下，可直接 embed web/（宪法 §4：web/ 是唯一真值源）。
//
//go:embed all:web
var webFS embed.FS

func main() {
	configPath := flag.String("config", "config.yaml", "配置文件路径")
	baseURL := flag.String("base-url", "https://openapi.lingxing.com", "领星 OpenAPI 根地址")
	flag.Parse()

	// 1. 加载配置（启动断言式校验，缺字段直接 FATAL）
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[main] 加载配置失败: %v", err)
	}
	log.Printf("[main] 配置加载完成：%d 账号，%d 接口", len(cfg.Accounts), len(cfg.Endpoints))


	// 2. 连 MySQL + 迁移
	dbx, err := db.NewPool(cfg.Database)
	if err != nil {
		log.Fatalf("[main] 连接 MySQL 失败: %v", err)
	}
	defer dbx.Close()
	if err := db.RunMigrations(dbx, "migrations"); err != nil {
		log.Fatalf("[main] 数据库迁移失败: %v", err)
	}
	log.Printf("[main] 数据库迁移完成")

	// 3. 构造 Worker（每「账号+接口」一个）
	registry := worker.NewRegistry()
	clients := api.NewClientRegistry(cfg.Accounts, *baseURL)
	limiterReg := worker.NewLimiterRegistry() // 限流器按 (quota_group, path) 共享
	var storeSourceWorkers []*worker.EndpointWorker
	degraded := 0 // 降级为不可同步的接口数（缺表等），仅用于启动摘要日志
	warned := 0   // 有告警但仍可同步的条目数（缺声明列等），仅用于启动摘要日志

	for i := range cfg.Endpoints {
		ep := cfg.Endpoints[i]
		acc := cfg.FindAccount(ep.Account)
		if acc == nil {
			log.Fatalf("[main] endpoint %s 的 account %q 找不到（config 校验本应拦住，这里兜底 FATAL）", ep.Name, ep.Account)
		}
		client := clients.Get(acc.ID)
		w, err := worker.New(ep, *acc, client, dbx, limiterReg)
		if err != nil {
			// 走到这里说明 worker.New 出了「非缺表」的真实构造错误（目前不存在这种路径）。
			// 缺表不再走 error：New 会把 worker 降级成 fatalErr 态并正常返回，
			// 只让这一个接口不可同步，绝不顶掉整个进程（CLAUDE.md §1.1）。
			log.Printf("[main] ⚠️ 构造 Worker %s 失败，已跳过该接口，其余接口照常启动: %v", ep.Name, err)
			degraded++
			continue
		}
		registry.Register(w)
		if ep.IsStoreSource {
			storeSourceWorkers = append(storeSourceWorkers, w)
		}
		st := w.Status()
		if st.FatalError != "" {
			// 最常见：目标表没建。接口标 error 挂在 UI 上，用户能看见原因并去建表，
			// 而不是面对一个打不开的 7799。
			log.Printf("[main] ⚠️ Worker 降级（不可同步，其余接口不受影响）：%s（table=%s）: %s", ep.Name, ep.Table, st.FatalError)
			degraded++
			continue
		}
		// 告警不阻断同步：表建了但缺配置声明的列，那些字段会被静默丢弃。
		// 只打出来给人看，不改变同步行为（见 worker.missingDeclaredColumns 的取舍说明）。
		for _, wn := range st.Warnings {
			log.Printf("[main] ⚠️ Worker 告警（同步仍会执行）：%s: %s", ep.Name, wn)
			warned++
		}
		log.Printf("[main] Worker 就绪：%s（account=%s, table=%s）", ep.Name, ep.Account, ep.Table)
	}
	if degraded > 0 {
		log.Printf("[main] ⚠️ 共 %d/%d 个接口降级为不可同步（多半是没建表），请在同步配置页查看红色错误原因后建表并重启",
			degraded, len(cfg.Endpoints))
	}
	if warned > 0 {
		log.Printf("[main] ⚠️ 共 %d 条表结构告警：配置声明的列在表里不存在，这些字段会被静默丢弃（同步仍在跑，补列后重启即可）", warned)
	}

	// 4 & 5. 启动所有 Worker goroutine（ctx 统一管理生命周期）
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, w := range registry.All() {
		go w.Run(ctx)
	}
	log.Printf("[main] %d 个 Worker 已启动", len(registry.All()))

	// 启动后立即同步一次店铺来源接口，保证 ls_stores 有数据（宪法 §10.2）
	// 用 channel 等待「启动首同步」完成，非阻塞触发但给一点时间落库
	if len(storeSourceWorkers) > 0 {
		log.Printf("[main] 触发 %d 个店铺来源接口首同步", len(storeSourceWorkers))
		for _, w := range storeSourceWorkers {
			w.Trigger() // 非阻塞；iterate_by_store 的 Worker 会从 ls_stores 读
		}
	}

	// 6. cron 调度（按 endpoint.Cron 发触发信号 + retention 清理）
	sched := worker.NewScheduler(cfg, registry, dbx, func(ctx context.Context, accountID string) error {
		client := clients.Get(accountID)
		if client == nil {
			return fmt.Errorf("未找到账号 Client: %s", accountID)
		}
		return client.TokenHolder().ForceRefresh(ctx)
	})
	if err := sched.Start(ctx); err != nil {
		log.Fatalf("[main] 启动调度器失败: %v", err)
	}
	defer sched.Stop()
	log.Printf("[main] 调度器已启动")

	// 7. HTTP 服务（web/ 资源从入口层注入）
	// //go:embed all:web 使 FS 内路径前缀为 "web/..."，故子目录根用 web/templates、web/static。
	assets := server.Assets{
		FS:         webFS,
		TemplateFS: "web/templates",
		StaticFS:   "web/static",
	}
	// 配置写入层：UI 增删改 config.yaml 经它做校验+备份+原子写（宪法 §7.5）
	store := config.NewStore(*configPath, cfg)
	srv := server.New(cfg, dbx, registry, clients, *baseURL, assets, store, sched, limiterReg, *configPath)

	// HTTP 在后台跑；主 goroutine 等信号优雅退出。
	// 监听失败（如端口被占）必须 FATAL：否则进程留在「Worker 照跑、UI 打不开」的
	// 僵尸态，还在继续写库，排障时极易误判。ErrServerClosed 是正常关停，不算失败。
	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("[main] HTTP 服务启动失败: %v", err)
		}
	}()

	log.Printf("[main] ✅ 领星同步机已启动，浏览器访问 http://127.0.0.1:%d", cfg.Server.Port)

	// 优雅退出
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("[main] 收到信号 %v，正在关闭...", sig)
	cancel()
	time.Sleep(500 * time.Millisecond) // 给 Worker 一点时间收尾
	log.Printf("[main] 已退出")
}
