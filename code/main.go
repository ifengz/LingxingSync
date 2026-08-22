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
//	-validate-config  只加载并校验配置，不连接数据库或启动服务
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
	"sort"
	"strings"
	"syscall"
	"time"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
	"lingxing-sync/internal/listingdaily"
	"lingxing-sync/internal/reportexport"
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
	validateConfig := flag.Bool("validate-config", false, "只校验配置，不连接数据库或启动服务")
	baseURL := flag.String("base-url", "https://openapi.lingxing.com", "领星 OpenAPI 根地址")
	reportReturns := flag.Bool("export-fba-customer-returns", false, "显式导出一份 Amazon FBA Customer Returns 正式报告后退出")
	reportExport := flag.Bool("export-amazon-report", false, "显式导出一份已支持的 Amazon 正式报告后退出；配合 -report-type")
	probeInventoryPlanning := flag.Bool("probe-fba-inventory-planning", false, "只创建/下载一份真实 FBA Inventory Planning 合同，不连接或写入本地数据库")
	resumeReportAudit := flag.Int64("resume-amazon-report-audit", 0, "复用已有正式报告 audit 的同一上游任务，不创建新任务")
	reportType := flag.String("report-type", reportexport.CustomerReturnsReportType, "Amazon report_type；默认 FBA Customer Returns")
	reportAccount := flag.String("report-account", "", "报告导出使用的本地 account id")
	reportSeller := flag.String("report-seller-id", "", "Amazon seller_id")
	reportStore := flag.String("report-store-id", "", "本地归属 store_id（必填，不传领星）")
	reportRegion := flag.String("report-region", "", "Amazon region: na/eu/fe")
	reportMarketplaces := flag.String("report-marketplace-ids", "", "逗号分隔的 Amazon marketplace_ids")
	reportDateFrom := flag.String("report-date-from", "", "RFC3339 data_start_time")
	reportDateTo := flag.String("report-date-to", "", "RFC3339 data_end_time")
	// The explicit CLI lane remains available for manual backfills; configured
	// report_exports are registered with the existing in-process scheduler.
	flag.Parse()
	reportMode, modeErr := formalReportMode(*reportReturns, *reportExport, *probeInventoryPlanning, *resumeReportAudit)
	if modeErr != nil {
		log.Fatalf("[main] %v", modeErr)
	}
	if *reportReturns && *reportType != reportexport.CustomerReturnsReportType {
		log.Fatalf("[main] -export-fba-customer-returns 不能与其他 report_type 同时使用")
	}
	if *resumeReportAudit > 0 && (*reportReturns || *reportExport) {
		log.Fatalf("[main] -resume-amazon-report-audit 不能与创建报告参数同时使用")
	}
	if *probeInventoryPlanning && *reportType != reportexport.CustomerReturnsReportType {
		log.Fatalf("[main] -probe-fba-inventory-planning 不接受 -report-type")
	}

	// 1. 加载配置（启动断言式校验，缺字段直接 FATAL）
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("[main] 加载配置失败: %v", err)
	}
	log.Printf("[main] 配置加载完成：%d 账号，%d 接口", len(cfg.Accounts), len(cfg.Endpoints))
	if *validateConfig {
		log.Printf("[main] 配置校验通过：%s", *configPath)
		return
	}
	if *probeInventoryPlanning {
		request := reportexport.Request{
			ReportType:     reportexport.FBAInventoryPlanningReportType,
			AccountID:      *reportAccount,
			SellerID:       *reportSeller,
			StoreID:        *reportStore,
			Region:         *reportRegion,
			MarketplaceIDs: splitNonEmpty(*reportMarketplaces),
			DateFrom:       *reportDateFrom,
			DateTo:         *reportDateTo,
		}
		account := cfg.FindAccount(request.AccountID)
		if account == nil {
			log.Fatalf("[main] Inventory Planning 合同探针账号不存在: %q", request.AccountID)
		}
		request.AccountID = account.ID
		result, err := reportexport.ProbeFBAInventoryPlanning(context.Background(), api.NewClient(account, *baseURL), worker.NewLimiter(1, 1000), request)
		if err != nil {
			log.Fatalf("[main] %s", formatInventoryPlanningProbeFailure(result, err))
		}
		log.Printf("[main] Inventory Planning 合同探针完成：task=%s document=%s rows=%d sha256=%s content_type=%q compression=%q header=%q",
			result.ReportTaskID, result.ReportDocumentID, result.Rows, result.DownloadSHA256, result.ContentType, result.CompressionAlgorithm, strings.Join(result.Header, "\t"))
		return
	}

	// 2. 连 MySQL + 迁移
	dbx, err := db.NewPool(cfg.Database)
	if err != nil {
		log.Fatalf("[main] 连接 MySQL 失败: %v", err)
	}
	defer dbx.Close()
	reportAuditStore := db.NewReportStore(dbx)
	var resumeAudit reportexport.Audit
	effectiveReportType := *reportType
	if *resumeReportAudit > 0 {
		resumeAudit, err = reportAuditStore.LoadReport(context.Background(), *resumeReportAudit)
		if err != nil {
			log.Fatalf("[main] 读取待恢复 Amazon report audit 失败: %v", err)
		}
		effectiveReportType = resumeAudit.Request.ReportType
	}
	if err := prepareDatabase(reportMode,
		func() error { return db.RunMigrations(dbx, "migrations") },
		func() error {
			return validateFormalReportSchema(effectiveReportType, func(table string) ([]string, error) {
				return db.GetTableColumns(dbx, table)
			})
		},
	); err != nil {
		log.Fatalf("[main] 数据库准备失败: %v", err)
	}
	if reportMode {
		log.Printf("[main] Amazon report 数据库结构只读校验通过 type=%s", effectiveReportType)
	} else {
		log.Printf("[main] 数据库迁移完成")
	}
	if reportMode {
		request := reportexport.Request{
			ReportType:     *reportType,
			AccountID:      *reportAccount,
			SellerID:       *reportSeller,
			StoreID:        *reportStore,
			Region:         *reportRegion,
			MarketplaceIDs: splitNonEmpty(*reportMarketplaces),
			DateFrom:       *reportDateFrom,
			DateTo:         *reportDateTo,
		}
		if *resumeReportAudit > 0 {
			request = resumeAudit.Request
		}
		account := cfg.FindAccount(request.AccountID)
		if account == nil {
			log.Fatalf("[main] 报告导出账号不存在: %q", request.AccountID)
		}
		if *resumeReportAudit == 0 {
			request.AccountID = account.ID
		}
		runner := reportexport.Runner{
			Client: api.NewClient(account, *baseURL),
			Store:  reportAuditStore,
			// Each formal report path has quota bucket=1. The explicit CLI lane
			// shares one serial limiter across create/query/renew calls.
			Limiter: worker.NewLimiter(1, 1000),
		}
		var result reportexport.Result
		if *resumeReportAudit > 0 {
			result, err = runner.Resume(context.Background(), *resumeReportAudit)
		} else {
			result, err = runner.Run(context.Background(), request)
		}
		if err != nil {
			log.Fatalf("[main] Amazon 正式报告导出失败 type=%s: %v", request.ReportType, err)
		}
		if reportRequiresDailyProjection(request.ReportType) {
			if err := projectFormalReport(context.Background(), listingdaily.SQLSourceReader{DB: dbx}, listingdaily.SQLStore{DB: dbx}, request, result, request.ReportType); err != nil {
				log.Fatalf("[main] Amazon 正式报告日维纠正失败 type=%s: %v", request.ReportType, err)
			}
		}
		log.Printf("[main] Amazon 正式报告完成：type=%s audit=%d task=%s document=%s rows=%d sha256=%s", request.ReportType, result.AuditID, result.ReportTaskID, result.ReportDocumentID, result.Rows, result.DownloadSHA256)
		return
	}

	// 3. 构造 Worker（每「账号+接口」一个）
	registry := worker.NewRegistry()
	clients := api.NewClientRegistry(cfg.Accounts, *baseURL)
	limiterReg := worker.NewLimiterRegistry() // 总桶按 quota_group、接口桶按 (quota_group, path) 共享
	dailyReader := listingdaily.SQLSourceReader{DB: dbx}
	dailyStore := listingdaily.SQLStore{DB: dbx}
	dailyProject := func(ctx context.Context, accountID string, targets []worker.DailyProjectionTarget, today time.Time) error {
		return projectDailyBatch(ctx, dailyReader, dailyStore, accountID, targets, today)
	}
	inventorySnapshot := func(ctx context.Context, accountID string, targets []worker.DailyProjectionTarget) error {
		dbTargets := make([]db.FBAInventorySnapshotTarget, 0, len(targets))
		for _, target := range targets {
			dbTargets = append(dbTargets, db.FBAInventorySnapshotTarget{Store: target.Store, Date: target.Date, StartedAt: target.StartedAt})
		}
		return db.CaptureFBAInventorySnapshots(ctx, dbx, accountID, dbTargets)
	}
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
		w.SetDailyProjector(dailyProject)
		if ep.Table == "ls_fba_inventory" {
			w.SetInventorySnapshotter(inventorySnapshot)
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
	reportLimiter := worker.NewLimiter(1, 1000)
	sched.SetCustomerReturnsRunner(customerReturnsRun(cfg, clients, db.NewReportStore(dbx), reportLimiter, dailyReader, dailyStore))
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

func formalReportMode(reportReturns, reportExport, probeInventoryPlanning bool, resumeAudit int64) (bool, error) {
	if resumeAudit < 0 {
		return false, fmt.Errorf("-resume-amazon-report-audit must be positive")
	}
	if probeInventoryPlanning && (reportReturns || reportExport || resumeAudit > 0) {
		return false, fmt.Errorf("-probe-fba-inventory-planning cannot be combined with report export or resume")
	}
	return reportReturns || reportExport || resumeAudit > 0, nil
}

func formatInventoryPlanningProbeFailure(result reportexport.ContractProbeResult, err error) string {
	return fmt.Sprintf("Inventory Planning 合同探针失败: task=%s document=%s rows=%d bytes=%d sha256=%s: %s",
		result.ReportTaskID,
		result.ReportDocumentID,
		result.Rows,
		result.DownloadedBytes,
		result.DownloadSHA256,
		api.SanitizeDiagnosticText(err.Error()),
	)
}

func customerReturnsRun(cfg *config.Config, clients *api.ClientRegistry, store reportexport.Store, limiter reportexport.Limiter, dailyReader listingdaily.SourceReader, dailyStore listingdaily.ReconciliationStore) func(context.Context, reportexport.Request) (reportexport.Result, error) {
	return func(ctx context.Context, request reportexport.Request) (reportexport.Result, error) {
		account := cfg.FindAccount(request.AccountID)
		if account == nil {
			return reportexport.Result{}, fmt.Errorf("Customer Returns 报表账号不存在: %s", request.AccountID)
		}
		request.AccountID = account.ID
		client := clients.Get(account.ID)
		if client == nil {
			return reportexport.Result{}, fmt.Errorf("Customer Returns 报表账号 Client 不存在: %s", account.ID)
		}
		runner := reportexport.Runner{Client: client, Store: store, Limiter: limiter}
		result, err := runner.Run(ctx, request)
		if err != nil {
			return result, err
		}
		if reportRequiresDailyProjection(normalizedReportType(request)) {
			if err := projectFormalReport(ctx, dailyReader, dailyStore, request, result, normalizedReportType(request)); err != nil {
				return result, err
			}
		}
		return result, nil
	}
}

func prepareDatabase(reportReturns bool, runMigrations, validateReportSchema func() error) error {
	if reportReturns {
		return validateReportSchema()
	}
	return runMigrations()
}

func customerReturnsSchemaRequirements() map[string][]string {
	return formalReportSchemaRequirements(reportexport.CustomerReturnsReportType)
}

func formalReportSchemaRequirements(reportType string) map[string][]string {
	fbaStorageFeeChargesSchemaColumns := []string{
		"asin", "fnsku", "product_name", "fulfillment_center", "country_code", "longest_side", "median_side", "shortest_side",
		"measurement_units", "weight", "weight_units", "item_volume", "volume_units", "product_size_tier", "average_quantity_on_hand",
		"average_quantity_pending_removal", "estimated_total_item_volume", "month_of_charge", "storage_rate", "currency",
		"estimated_monthly_storage_fee", "dangerous_goods_storage_type", "eligible_for_inventory_discount", "qualifies_for_inventory_discount",
		"total_incentive_fee_amount", "breakdown_incentive_fee_amount", "average_quantity_customer_orders", "sku",
		"storage_utilization_ratio", "storage_utilization_ratio_units", "base_rate", "utilization_surcharge_rate",
		"avg_qty_for_sus", "est_vol_for_sus", "est_base_msf", "est_sus",
	}
	fbaOverageFeeChargesSchemaColumns := []string{
		"charged_date", "country_code", "storage_type", "charge_rate", "storage_usage_volume", "storage_limit_volume", "overage_volume", "volume_unit", "charged_fee_amount", "currency_code",
	}
	fbaLongtermStorageFeeChargesSchemaColumns := []string{
		"snapshot-date", "sku", "fnsku", "asin", "product-name", "condition", "per-unit-volume", "currency", "volume-unit", "country", "qty-charged", "amount-charged", "surcharge-age-tier", "rate-surcharge",
	}
	fbaStrandedInventorySchemaColumns := []string{
		"primary-action", "date-stranded", "Date-to-take-auto-removal", "status-primary", "status-secondary", "error-message", "stranded-reason", "asin", "sku", "fnsku", "product-name", "condition", "fulfilled-by", "fulfillable-qty", "your-price", "unfulfillable-qty", "reserved-quantity", "inbound-shipped-qty", "program",
	}
	fbaEstimatedFeesSchemaColumns := []string{
		"sku", "fnsku", "asin", "product-name", "product-group", "brand", "fulfilled-by", "has-local-inventory", "your-price", "sales-price", "longest-side", "median-side", "shortest-side", "length-and-girth", "unit-of-dimension", "item-package-weight", "unit-of-weight", "product-size-weight-band", "currency", "estimated-fee-total", "estimated-referral-fee-per-unit", "estimated-variable-closing-fee", "expected-domestic-fulfilment-fee-per-unit", "expected-efn-fulfilment-fee-per-unit-uk", "expected-efn-fulfilment-fee-per-unit-de", "expected-efn-fulfilment-fee-per-unit-fr", "expected-efn-fulfilment-fee-per-unit-it", "expected-efn-fulfilment-fee-per-unit-es", "expected-efn-fulfilment-fee-per-unit-se", "amazon-store", "product-size-tier", "estimated-order-handling-fee-per-order", "estimated-pick-pack-fee-per-unit", "estimated-weight-handling-fee-per-unit", "expected-fulfillment-fee-per-unit", "estimated-future-fee", "estimated-future-order-handling-fee-per-order", "estimated-future-pick-pack-fee-per-unit", "estimated-future-weight-handling-fee-per-unit", "expected-future-fulfillment-fee-per-unit",
	}
	fbaInboundNoncomplianceSchemaColumns := []string{
		"issue-reported-date", "shipment-creation-date", "fba-shipment-id", "fba-carton-id", "fulfillment-center-id", "sku", "fnsku", "asin", "product-name", "problem-type", "problem-quantity", "expected-quantity", "received-quantity", "performance-measurement-unit", "coaching-level", "fee-type", "currency", "fee-total", "problem-level", "alert-status",
	}
	fbaRecommendedRemovalSchemaColumns := []string{
		"snapshot-date", "sku", "fnsku", "asin", "product-name", "condition", "sellable-quantity", "sellable-271-365-days", "sellable-365+-days", "sellable-removal-quantity", "unsellable-quantity", "unsellable-0-7-days", "unsellable-8-60-days", "unsellable-61-90-days", "sellable-121-180-days", "sellable-181-270-days",
	}
	fbaRemovalOrderSchemaColumns := []string{
		"request-date", "order-id", "order-source", "order-type", "service-speed", "order-status", "last-updated-date", "sku", "fnsku", "disposition", "requested-quantity", "cancelled-quantity", "disposed-quantity", "shipped-quantity", "in-process-quantity", "removal-fee", "currency",
	}
	fbaRemovalShipmentSchemaColumns := []string{
		"request-date", "order-id", "shipment-date", "sku", "fnsku", "disposition", "shipped-quantity", "carrier", "tracking-number", "removal-order-type",
	}
	allOrdersSchemaColumns := []string{
		"amazon-order-id", "merchant-order-id", "purchase-date", "last-updated-date", "order-status", "fulfillment-channel", "sales-channel", "order-channel", "ship-service-level", "product-name", "sku", "asin", "item-status", "quantity", "currency", "item-price", "item-tax", "shipping-price", "shipping-tax", "gift-wrap-price", "gift-wrap-tax", "item-promotion-discount", "ship-promotion-discount", "ship-city", "ship-state", "ship-postal-code", "ship-country", "promotion-ids", "cpf", "is-business-order", "purchase-order-number", "price-designation", "signature-confirmation-recommended", "order-item-id",
	}
	fulfilledShipmentsSchemaColumns := []string{
		"amazon-order-id", "merchant-order-id", "shipment-id", "shipment-item-id", "amazon-order-item-id", "merchant-order-item-id",
		"purchase-date", "payments-date", "shipment-date", "reporting-date", "buyer-email", "buyer-name", "buyer-phone-number",
		"sku", "product-name", "quantity-shipped", "currency", "item-price", "item-tax", "shipping-price", "shipping-tax",
		"gift-wrap-price", "gift-wrap-tax", "ship-service-level", "recipient-name", "ship-address-1", "ship-address-2", "ship-address-3",
		"ship-city", "ship-state", "ship-postal-code", "ship-country", "ship-phone-number", "bill-address-1", "bill-address-2",
		"bill-address-3", "bill-city", "bill-state", "bill-postal-code", "bill-country", "item-promotion-discount",
		"ship-promotion-discount", "carrier", "tracking-number", "estimated-arrival-date", "fulfillment-center-id", "fulfillment-channel", "sales-channel",
	}
	requirements := make(map[string][]string)
	if reportRequiresDailyProjection(reportType) {
		for table, columns := range listingdaily.CustomerReturnsSchemaRequirements() {
			requirements[table] = columns
		}
	}
	requirements["ls_report_export_tasks"] = []string{
		"id", "account_id", "seller_id", "store_id", "report_type", "region", "marketplace_ids",
		"date_from", "date_to", "report_task_id", "report_document_id", "status",
		"compression_algorithm", "download_url", "download_sha256", "downloaded_at", "rows_imported",
		"error_message", "active_scope_key", "updated_at",
	}
	switch reportType {
	case reportexport.CustomerReturnsReportType:
		requirements["ls_fba_fulfillment_customer_returns"] = []string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
			"return-date", "order-id", "sku", "asin", "fnsku", "product-name", "quantity",
			"fulfillment-center-id", "detailed-disposition", "reason", "status",
			"license-plate-number", "customer-comments",
		}
	case reportexport.CustomerShipmentSalesReportType:
		requirements["ls_fba_fulfillment_customer_shipment_sales"] = []string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
			"shipment-date", "sku", "fnsku", "asin", "fulfillment-center-id", "quantity", "amazon-order-id", "currency",
			"item-price-per-unit", "shipping-price", "gift-wrap-price", "ship-city", "ship-state", "ship-postal-code",
		}
	case reportexport.FBAInventoryReportType:
		requirements["ls_fba_myi_unsuppressed_inventory"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{
			"sku", "fnsku", "asin", "product-name", "condition", "your-price", "mfn-listing-exists", "mfn-fulfillable-quantity",
			"afn-listing-exists", "afn-warehouse-quantity", "afn-fulfillable-quantity", "afn-unsellable-quantity", "afn-reserved-quantity",
			"afn-total-quantity", "per-unit-volume", "afn-inbound-working-quantity", "afn-inbound-shipped-quantity",
			"afn-inbound-receiving-quantity", "afn-researching-quantity", "afn-reserved-future-supply", "afn-future-supply-buyable",
			"afn-fc-transfer-quantity", "afn-onhand-buyable-quantity", "store", "afn-fulfillable-quantity-local", "afn-fulfillable-quantity-remote",
		}...)
	case reportexport.FBAAllInventoryReportType:
		requirements["ls_fba_myi_all_inventory"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{
			"sku", "fnsku", "asin", "product-name", "condition", "your-price", "mfn-listing-exists", "mfn-fulfillable-quantity",
			"afn-listing-exists", "afn-warehouse-quantity", "afn-fulfillable-quantity", "afn-unsellable-quantity", "afn-reserved-quantity",
			"afn-total-quantity", "per-unit-volume", "afn-inbound-working-quantity", "afn-inbound-shipped-quantity",
			"afn-inbound-receiving-quantity", "afn-researching-quantity", "afn-reserved-future-supply", "afn-future-supply-buyable",
			"afn-fc-transfer-quantity", "afn-onhand-buyable-quantity", "store", "afn-fulfillable-quantity-local", "afn-fulfillable-quantity-remote",
		}...)
	case reportexport.ReservedInventoryReportType:
		requirements["ls_fba_reserved_inventory"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{"sku", "fnsku", "asin", "product-name", "reserved_qty", "reserved_customerorders", "reserved_fc-transfers", "reserved_fc-processing", "reserved_staging", "program"}...)
	case reportexport.AFNInventoryReportType:
		requirements["ls_afn_inventory"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{"seller-sku", "fulfillment-channel-sku", "asin", "condition-type", "Warehouse-Condition-code", "Quantity Available"}...)
	case reportexport.AFNInventoryByCountryReportType:
		requirements["ls_afn_inventory_by_country"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{"seller-sku", "fulfillment-channel-sku", "asin", "condition-type", "country", "quantity-for-local-fulfillment"}...)
	case reportexport.FBAStorageFeeChargesReportType:
		requirements["ls_fba_storage_fee_charges"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, fbaStorageFeeChargesSchemaColumns...)
	case reportexport.FBAOverageFeeChargesReportType:
		requirements["ls_fba_overage_fee_charges"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, fbaOverageFeeChargesSchemaColumns...)
	case reportexport.FBALongtermStorageFeeChargesReportType:
		requirements["ls_fba_longterm_storage_fee_charges"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, fbaLongtermStorageFeeChargesSchemaColumns...)
	case reportexport.CustomerShipmentReplacementsReportType:
		requirements["ls_fba_fulfillment_customer_shipment_replacements"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{"shipment-date", "sku", "asin", "fulfillment-center-id", "original-fulfillment-center-id", "quantity", "replacement-reason-code", "replacement-amazon-order-id", "original-amazon-order-id"}...)
	case reportexport.FBAReimbursementsReportType:
		requirements["ls_fba_reimbursements"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, []string{"approval-date", "reimbursement-id", "case-id", "amazon-order-id", "reason", "sku", "fnsku", "asin", "product-name", "condition", "currency-unit", "amount-per-unit", "amount-total", "quantity-reimbursed-cash", "quantity-reimbursed-inventory", "quantity-reimbursed-total", "original-reimbursement-id", "original-reimbursement-type"}...)
	case reportexport.FBAStrandedInventoryReportType:
		requirements["ls_fba_stranded_inventory"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, fbaStrandedInventorySchemaColumns...)
	case reportexport.FBAEstimatedFeesReportType:
		requirements["ls_fba_estimated_fees"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, fbaEstimatedFeesSchemaColumns...)
	case reportexport.FBAInventoryPlanningReportType:
		requirements["ls_fba_inventory_planning"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, reportexport.FBAInventoryPlanningColumns()...)
	case reportexport.FBAInboundNoncomplianceReportType:
		requirements["ls_fba_inbound_noncompliance"] = append([]string{
			"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256",
		}, fbaInboundNoncomplianceSchemaColumns...)
	case reportexport.FBARecommendedRemovalReportType:
		requirements["ls_fba_recommended_removals"] = append([]string{"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256"}, fbaRecommendedRemovalSchemaColumns...)
	case reportexport.FBARemovalOrderReportType:
		requirements["ls_fba_removal_order_details"] = append([]string{"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256"}, fbaRemovalOrderSchemaColumns...)
	case reportexport.FBARemovalShipmentReportType:
		requirements["ls_fba_removal_shipment_details"] = append([]string{"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256"}, fbaRemovalShipmentSchemaColumns...)
	case reportexport.AllOrdersReportType:
		requirements["ls_amazon_all_orders_by_order_date"] = append([]string{"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256"}, allOrdersSchemaColumns...)
	case reportexport.FulfilledShipmentsReportType:
		requirements["ls_amazon_fulfilled_shipments"] = append([]string{"account_id", "seller_id", "store_id", "report_task_id", "row_number", "row_sha256"}, fulfilledShipmentsSchemaColumns...)
	default:
		return nil
	}
	return requirements
}

func validateCustomerReturnsSchema(loadColumns func(string) ([]string, error)) error {
	return validateFormalReportSchema(reportexport.CustomerReturnsReportType, loadColumns)
}

func validateFormalReportSchema(reportType string, loadColumns func(string) ([]string, error)) error {
	requirements := formalReportSchemaRequirements(reportType)
	if requirements == nil {
		return fmt.Errorf("不支持的 Amazon 正式报告类型 %q", reportType)
	}
	tables := make([]string, 0, len(requirements))
	for table := range requirements {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	for _, table := range tables {
		columns, err := loadColumns(table)
		if err != nil {
			return fmt.Errorf("Amazon report schema %s: %w", table, err)
		}
		available := make(map[string]struct{}, len(columns))
		for _, column := range columns {
			available[column] = struct{}{}
		}
		for _, column := range requirements[table] {
			if _, ok := available[column]; !ok {
				return fmt.Errorf("Amazon report schema missing %s.%s", table, column)
			}
		}
	}
	return nil
}

func normalizedReportType(request reportexport.Request) string {
	if strings.TrimSpace(request.ReportType) == "" {
		return reportexport.CustomerReturnsReportType
	}
	return strings.TrimSpace(request.ReportType)
}

func reportRequiresDailyProjection(reportType string) bool {
	return reportType != reportexport.CustomerShipmentReplacementsReportType && reportType != reportexport.FBAReimbursementsReportType && reportType != reportexport.AFNInventoryByCountryReportType && reportType != reportexport.FBAStorageFeeChargesReportType && reportType != reportexport.FBAOverageFeeChargesReportType && reportType != reportexport.FBALongtermStorageFeeChargesReportType && reportType != reportexport.FBAStrandedInventoryReportType && reportType != reportexport.FBAEstimatedFeesReportType && reportType != reportexport.FBAInventoryPlanningReportType && reportType != reportexport.FBAInboundNoncomplianceReportType && reportType != reportexport.FBARecommendedRemovalReportType && reportType != reportexport.FBARemovalOrderReportType && reportType != reportexport.FBARemovalShipmentReportType && reportType != reportexport.AllOrdersReportType && reportType != reportexport.FulfilledShipmentsReportType
}

func projectDailyBatch(ctx context.Context, dailyReader listingdaily.SourceReader, dailyStore listingdaily.Store, accountID string, targets []worker.DailyProjectionTarget, today time.Time) error {
	if dailyReader == nil || dailyStore == nil {
		return fmt.Errorf("日维投影未配置")
	}
	rows := make([]listingdaily.Metric, 0)
	for _, target := range targets {
		_, built, err := listingdaily.BuildFromSQL(ctx, dailyReader, accountID, target.Store, target.Channel, target.Date, today, listingdaily.ReportAbsent)
		if err != nil {
			return fmt.Errorf("%s/%s/%s: %w", target.Store, target.Channel, target.Date.Format("2006-01-02"), err)
		}
		rows = append(rows, built...)
	}
	return dailyStore.Persist(ctx, rows)
}

func projectCustomerReturns(ctx context.Context, dailyReader listingdaily.SourceReader, dailyStore listingdaily.ReconciliationStore, request reportexport.Request, result reportexport.Result) error {
	return projectFormalReport(ctx, dailyReader, dailyStore, request, result, reportexport.CustomerReturnsReportType)
}

func projectCustomerShipmentSales(ctx context.Context, dailyReader listingdaily.SourceReader, dailyStore listingdaily.ReconciliationStore, request reportexport.Request, result reportexport.Result) error {
	return projectFormalReport(ctx, dailyReader, dailyStore, request, result, reportexport.CustomerShipmentSalesReportType)
}

func projectFormalReport(ctx context.Context, dailyReader listingdaily.SourceReader, dailyStore listingdaily.ReconciliationStore, request reportexport.Request, result reportexport.Result, reportType string) error {
	if dailyReader == nil || dailyStore == nil {
		return fmt.Errorf("正式报告日维投影未配置")
	}
	evidence := listingdaily.ReportEvidence{AuditID: result.AuditID, ReportTaskID: result.ReportTaskID, ReportType: reportType}
	if evidence.AuditID <= 0 || strings.TrimSpace(evidence.ReportTaskID) == "" {
		return fmt.Errorf("正式报告日维投影缺少本次 report audit/task")
	}
	from, to, err := formalReportBusinessDates(reportType, request, time.Now().UTC())
	if err != nil {
		return err
	}
	today := time.Now()
	rows := make([]listingdaily.Metric, 0)
	audits := make([]listingdaily.ReconciliationAudit, 0, int(to.Sub(from).Hours()/24)+1)
	for date := from; !date.After(to); date = date.AddDate(0, 0, 1) {
		projection, built, buildErr := listingdaily.BuildFromSQL(ctx, dailyReader, request.AccountID, request.StoreID, "sc_fba", date, today, listingdaily.ReportReconciled, evidence)
		reconciliation := listingdaily.Reconciliation{}
		if projection.Reconciliation != nil {
			reconciliation = *projection.Reconciliation
		}
		if buildErr != nil {
			failed := listingdaily.ReconciliationAudit{Evidence: evidence, BusinessDate: date, Status: listingdaily.ReconciliationFailed, Reconciliation: reconciliation, ErrorMessage: buildErr.Error()}
			if err := dailyStore.PersistFailedReconciliations(ctx, []listingdaily.ReconciliationAudit{failed}); err != nil {
				return fmt.Errorf("正式报告日维纠正 %s: %v; 保存失败对账: %w", date.Format("2006-01-02"), buildErr, err)
			}
			return fmt.Errorf("正式报告日维纠正 %s: %w", date.Format("2006-01-02"), buildErr)
		}
		status := listingdaily.ReconciliationMatched
		if len(reconciliation.MissingInDB) != 0 || len(reconciliation.MissingInReport) != 0 || len(reconciliation.FieldDiffs) != 0 {
			status = listingdaily.ReconciliationCorrected
		}
		rows = append(rows, built...)
		audits = append(audits, listingdaily.ReconciliationAudit{Evidence: evidence, BusinessDate: date, Status: status, Reconciliation: reconciliation})
	}
	if err := dailyStore.PersistReportBatch(ctx, rows, audits); err != nil {
		return fmt.Errorf("正式报告日维纠正发布: %w", err)
	}
	return nil
}

func reportBusinessDates(request reportexport.Request) (time.Time, time.Time, error) {
	from, err := time.Parse(time.RFC3339, request.DateFrom)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("Customer Returns date_from 非法: %w", err)
	}
	to, err := time.Parse(time.RFC3339, request.DateTo)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("Customer Returns date_to 非法: %w", err)
	}
	from = time.Date(from.Year(), from.Month(), from.Day(), 0, 0, 0, 0, time.UTC)
	to = time.Date(to.Year(), to.Month(), to.Day(), 0, 0, 0, 0, time.UTC)
	if from.After(to) {
		return time.Time{}, time.Time{}, fmt.Errorf("Customer Returns date_to 不能早于 date_from")
	}
	return from, to, nil
}

func formalReportBusinessDates(reportType string, request reportexport.Request, now time.Time) (time.Time, time.Time, error) {
	if reportType == reportexport.FBAInventoryReportType || reportType == reportexport.FBAAllInventoryReportType || reportType == reportexport.ReservedInventoryReportType || reportType == reportexport.AFNInventoryReportType {
		date := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
		return date, date, nil
	}
	return reportBusinessDates(request)
}

func splitNonEmpty(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
