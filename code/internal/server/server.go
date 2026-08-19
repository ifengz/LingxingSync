// Package server 是领星同步机的 HTTP 服务层。
//
// 它把 worker.Registry / db / api 用一个统一的 JSON over HTTP 接口暴露给
// 浏览器端 UI（Tailwind + Alpine.js，零构建）。宪法对应：
//   - doc/04-api.md：所有 `/api/*` 路由与响应结构
//   - doc/05-pages.md：5 个页面路由与布局
//
// 设计要点：
//   - 单二进制：模板与静态资源在入口层（cmd/main.go，位于仓库根 code/）
//     用 go:embed 把 web/ 打进二进制，再把 embed.FS 注入本包。
//     web/ 是唯一真值源（宪法 §4 目录结构），本包不自带资源副本。
//   - 路由用 Go 1.22 的 ServeMux 增强（支持 `GET /api/tasks/{id}/logs` 风格）
//   - 页面只渲染最小 HTML shell，具体数据由前端 Alpine.js 调 /api 拉取
package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/datasetapi"
	"lingxing-sync/internal/worker"
)

// Assets 是从入口层注入的静态资源（web/ 的 embed.FS）。
// templateRoot / staticRoot 是 embed.FS 内的子目录名（如 "templates"/"static"）。
type Assets struct {
	FS         embed.FS
	TemplateFS string // 默认 "templates"
	StaticFS   string // 默认 "static"
}

// Server 持有 HTTP 服务运行期所需的全部依赖。
type Server struct {
	cfg        *config.Config
	startupCfg *config.Config // 进程启动时的配置，用于判断落盘配置是否尚未被本进程加载。
	dbx        *sqlx.DB
	reg        *worker.Registry
	clients    *api.ClientRegistry
	httpSrv    *http.Server
	startTime  time.Time
	baseURL    string // 领星 openapi 根，默认 https://openapi.lingxing.com
	assets     Assets // 注入的 web/ 静态资源（embed.FS）

	// 配置读写 + 热加载/重启依赖（宪法 §7.5）
	store        *config.ConfigStore     // config.yaml 线程安全读写 + 变更分类
	sched        *worker.Scheduler       // 热加载时 Rebuild cron
	limiters     *worker.LimiterRegistry // rate 变化时 UpdateOrCreate
	configPath   string                  // config.yaml 路径（消息展示用）
	datasetAPI   *datasetapi.Handler     // listing-daily-v1 兼容入口
	datasetAPIs  map[string]*datasetapi.Handler
	dailyPreview dailyPreviewReader // 固定日维预览查询
	reportStatus reportStatusReader // 正式报表任务与对账状态

	// pages: 页面名 → 该页专属的已解析模板树。
	// 关键解耦：每页一棵独立模板树（layout + 该页 partial），这样各页的
	// {{define "content"}} 互不覆盖；改一页只重渲染该页。
	pages map[string]*template.Template
}

// New 构造一个未启动的 Server。baseURL 为空时使用领星线上默认值。
// assets 是从入口层注入的 web/ 资源（embed.FS）。
// store/sched/limiters/configPath 支撑配置读写与热加载/重启（宪法 §7.5）。
func New(cfg *config.Config, dbx *sqlx.DB, reg *worker.Registry, clients *api.ClientRegistry, baseURL string, assets Assets, store *config.ConfigStore, sched *worker.Scheduler, limiters *worker.LimiterRegistry, configPath string) *Server {
	if baseURL == "" {
		baseURL = "https://openapi.lingxing.com"
	}
	if assets.TemplateFS == "" {
		assets.TemplateFS = "templates"
	}
	if assets.StaticFS == "" {
		assets.StaticFS = "static"
	}
	startupCfg := cfg
	if store != nil {
		startupCfg = store.Current()
	}
	s := &Server{
		cfg:        cfg,
		startupCfg: startupCfg,
		dbx:        dbx,
		reg:        reg,
		clients:    clients,
		startTime:  time.Now(),
		baseURL:    baseURL,
		assets:     assets,
		store:      store,
		sched:      sched,
		limiters:   limiters,
		configPath: configPath,
		pages:      map[string]*template.Template{},
	}
	if dbx != nil {
		s.dailyPreview = sqlDailyPreviewReader{db: dbx}
		s.reportStatus = sqlReportStatusReader{db: dbx}
	}
	s.datasetAPIs = make(map[string]*datasetapi.Handler)
	for _, definition := range datasetapi.Definitions() {
		handler, err := s.newDatasetHandler(cfg, definition)
		if err != nil {
			log.Printf("[server] dataset %s unavailable: %v", definition.ID, err)
			continue
		}
		s.datasetAPIs[definition.ID] = handler
	}
	s.datasetAPI = s.datasetAPIs[datasetapi.DatasetID]
	if err := s.parseTemplates(); err != nil {
		// 模板编译错属编程期错误，启动直接 fail-loud（宪法：不静默兜底）
		log.Fatalf("[server] 模板编译失败: %v", err)
	}
	return s
}

func (s *Server) datasetConfigNeedsRestart() bool {
	if s.startupCfg == nil || s.store == nil {
		return false
	}
	return config.ClassifyChange(s.startupCfg, s.store.Current()) == config.ChangeRestart
}

func (s *Server) newDatasetHandler(cfg *config.Config, definition datasetapi.Definition) (*datasetapi.Handler, error) {
	fields := configuredDatasetFields(cfg.DatasetAPI, definition)
	tokens := make([]datasetapi.Token, 0, len(cfg.DatasetAPI.Tokens))
	for _, token := range cfg.DatasetAPI.Tokens {
		if !containsDatasetScope(token.DatasetScopes, definition.ID) {
			continue
		}
		expiresAt, err := parseDatasetTokenExpiry(token.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("dataset token %s expires_at invalid: %w", token.ID, err)
		}
		tokens = append(tokens, datasetapi.Token{ID: token.ID, ProjectID: token.ProjectID, Hash: token.TokenHash, DatasetScopes: token.DatasetScopes, StoreScopes: token.StoreScopes, Fields: fields, ExpiresAt: expiresAt, Revoked: token.Revoked})
	}
	handler, err := datasetapi.New(datasetapi.Config{Definition: definition, Tokens: tokens, FieldAllowlist: fields, CatalogFields: catalogDatasetFields(definition), MaxDateSpanDays: cfg.DatasetAPI.MaxDateSpanDays, MaxPageSize: cfg.DatasetAPI.MaxPageSize, CursorSecret: []byte(cfg.DatasetAPI.CursorSecret)}, nil)
	if err != nil || s.dbx == nil {
		return handler, err
	}
	switch definition.ID {
	case datasetapi.DatasetID:
		handler.SetReader(datasetapi.NewSQLReader(s.dbx))
	case "return-reason-detail-v1":
		handler.SetReader(datasetapi.NewReturnReasonDetailReader(s.dbx))
	case "fba-inventory-snapshot-v1":
		handler.SetReader(datasetapi.NewFBAInventorySnapshotReader(s.dbx))
	case "order-shipping-address-detail-v1":
		handler.SetReader(datasetapi.NewOrderShippingAddressDetailReader(s.dbx))
	case "address-order-item-detail-v1":
		handler.SetReader(datasetapi.NewAddressOrderItemDetailReader(s.dbx))
	}
	return handler, nil
}

func configuredDatasetFields(cfg config.DatasetAPIConfig, definition datasetapi.Definition) []string {
	if definition.ID == datasetapi.DatasetID {
		return append([]string(nil), cfg.FieldAllowlist...)
	}
	if fields := cfg.FieldAllowlists[definition.ID]; len(fields) > 0 {
		return append([]string(nil), fields...)
	}
	return append([]string(nil), definition.Fields...)
}

func catalogDatasetFields(definition datasetapi.Definition) []string {
	if definition.ID == datasetapi.DatasetID {
		return append([]string(nil), availableDatasetFields...)
	}
	return append([]string(nil), definition.Fields...)
}

func containsDatasetScope(scopes []string, datasetID string) bool {
	for _, scope := range scopes {
		if scope == datasetID {
			return true
		}
	}
	return false
}

func parseDatasetTokenExpiry(raw string) (time.Time, error) {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (s *Server) SetDatasetReader(reader datasetapi.Reader) {
	if s.datasetAPI != nil {
		s.datasetAPI.SetReader(reader)
	}
}

// sharedFuncs 是所有模板树共享的 FuncMap。
func sharedFuncs() template.FuncMap {
	return template.FuncMap{
		// 安全地把后端注入的 JSON 串带进前端 <script>，避免双重转义。
		"rawJS": func(s string) template.JS { return template.JS(s) },
		// json 把任意值序列化为 JSON 字符串，便于模板里注入到 <script>。
		// 出错时返回 "null"，避免把 Go 的 %+v 格式当 JS。
		"json": func(v any) (template.JS, error) {
			b, err := json.Marshal(v)
			if err != nil {
				return template.JS("null"), err
			}
			return template.JS(b), nil
		},
		// dict 在模板里临时构造一个 map，方便把多个变量传给 sub-template（如侧栏项）。
		"dict": func(values ...any) map[string]any {
			m := make(map[string]any, len(values)/2)
			for i := 0; i+1 < len(values); i += 2 {
				k, _ := values[i].(string)
				m[k] = values[i+1]
			}
			return m
		},
		// listItems 返回侧栏导航项；宪法 §3 固定顺序。
		"listItems": func() []navItem {
			return []navItem{
				{Key: "settings_api", Href: "/settings/api", Label: "API配置"},
				{Key: "sync_manage", Href: "/sync", Label: "同步配置"},
				{Key: "logs", Href: "/logs", Label: "同步日志"},
				{Key: "datasources", Href: "/datasources", Label: "数据源"},
				{Key: "dataset_fields", Href: "/dataset-fields", Label: "数据表字段"},
			}
		},
	}
}

// navItem 是侧栏导航项。
type navItem struct {
	Key   string
	Href  string
	Label string
}

// parseTemplates 用「clone 基树法」为每个页面构建独立模板树。
//
// 为什么用 clone 法：layout 是裸模板（含 {{block "content" .}}），先解析成基树；
// 每页只写 {{define "content"}}...{{end}}，对基树 Clone 后 Parse 该页内容，
// 用页面的 content 定义覆盖基树的 block。这样：
//   - layout 只解析一次
//   - 各页 content 在各自 clone 出的树里，互不覆盖（改一页不影响其他页）
//   - 避免「多文件 ParseFS 时 define 位置错乱」的解析陷阱
func (s *Server) parseTemplates() error {
	root := s.assets.TemplateFS
	entries, err := fs.ReadDir(s.assets.FS, root)
	if err != nil {
		return fmt.Errorf("读 %s 目录: %w", root, err)
	}
	// 1. 解析 layout 成基树（裸模板，无 define 包裹）
	base := template.New("layout").Funcs(sharedFuncs())
	if _, err := base.ParseFS(s.assets.FS, root+"/layout.html"); err != nil {
		return fmt.Errorf("解析 layout: %w", err)
	}
	// 2. 对每个页面文件，clone 基树后 parse 该页的 content 定义
	for _, e := range entries {
		name := e.Name()
		if name == "layout.html" || !strings.HasSuffix(name, ".html") {
			continue
		}
		pageKey := strings.TrimSuffix(name, ".html")
		pageBytes, err := s.assets.FS.ReadFile(root + "/" + name)
		if err != nil {
			return fmt.Errorf("读页面 %s: %w", name, err)
		}
		clone, err := base.Clone()
		if err != nil {
			return fmt.Errorf("clone 基树给 %s: %w", name, err)
		}
		if _, err := clone.Parse(string(pageBytes)); err != nil {
			return fmt.Errorf("解析页面 %s: %w", name, err)
		}
		s.pages[pageKey] = clone
	}
	if len(s.pages) == 0 {
		return fmt.Errorf("%s/ 下没有页面文件", root)
	}
	return nil
}

// Routes 构建并返回完整的 ServeMux（每次调用返回新实例）。
//
// 宪法对应：doc/04-api.md（API 路由）+ doc/05-pages.md（页面路由）。
func (s *Server) Routes() *http.ServeMux {
	mux := http.NewServeMux()

	// ---- 静态资源（/static/*）----
	staticSub, err := fs.Sub(s.assets.FS, s.assets.StaticFS)
	if err != nil {
		// embed 失败属编译期问题，运行期不应到达
		log.Fatalf("[server] static sub fs: %v", err)
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))

	// ---- 页面路由（返回 HTML shell）----
	mux.HandleFunc("GET /{$}", s.pageIndex)       // 重定向到 /sync（概览页已删）
	mux.HandleFunc("GET /sync", s.pageSyncManage) // 同步管理
	mux.HandleFunc("GET /logs", s.pageLogs)       // 同步日志
	mux.HandleFunc("GET /datasources", s.pageDataSources)
	mux.HandleFunc("GET /dataset-fields", s.pageDatasetFields)
	mux.HandleFunc("GET /settings/api", s.pageSettingsAPI)

	// ---- API 路由：状态/端点/任务 ----
	mux.HandleFunc("GET /api/endpoints", s.apiEndpoints)
	mux.HandleFunc("GET /api/egress-ip", s.apiEgressIP)
	mux.HandleFunc("GET /api/tasks", s.apiTasks)
	mux.HandleFunc("GET /api/tasks/{id}/logs", s.apiTaskLogs)

	// ---- API 路由：触发同步 ----
	mux.HandleFunc("POST /api/sync/{name}", s.apiSyncTrigger)
	mux.HandleFunc("POST /api/sync/{name}/cancel", s.apiSyncCancel)

	// ---- API 路由：设置 ----
	mux.HandleFunc("GET /api/settings", s.apiSettings)
	mux.HandleFunc("POST /api/settings/reload", s.apiSettingsReload)
	mux.HandleFunc("POST /api/settings/test-db", s.apiSettingsTestDB)

	// ---- API 路由：对账 ----
	mux.HandleFunc("POST /api/reconcile", s.apiReconcile)

	// ---- API 路由：代码注册的数据表 ----
	s.registerDailyReportRoutes(mux)
	for datasetID, handler := range s.datasetAPIs {
		mux.Handle("/api/v1/datasets/"+datasetID+"/", handler)
		mux.Handle(datasetapi.FieldsPathFor(datasetID), handler)
	}

	// ---- API 路由：配置读写（账号/接口 CRUD + 重启 + 字段查询）----
	// 全部在 handlers_config.go 内定义并自注册，统一维护（宪法 §7.5）。
	s.registerConfigRoutes(mux)

	// ---- API 路由：接口清单（从模板挑一个 → 选账号 → 启用）----
	// 全部在 handlers_catalog.go 内定义并自注册。
	s.registerCatalogRoutes(mux)

	return mux
}

// Start 在 cfg.Server.Port 上 ListenAndServe。阻塞调用，失败返回。
//
// 注意：宪法要求端口固定 7799；若 cfg 未填则在 config.Load 里已被兜底为 7799，
// 因此这里直接信任 cfg。
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.Port)
	s.httpSrv = &http.Server{
		Addr:              addr,
		Handler:           s.withMiddleware(s.Routes()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	log.Printf("[server] HTTP 监听 %s", addr)
	return s.httpSrv.ListenAndServe()
}

// withMiddleware 包一层全局中间件：日志 + panic recover + 可选 secret 校验。
//
// secret（cfg.Server.Secret）非空时，要求所有 /api/* 请求头带
// `X-Sync-Secret: <secret>`；页面与静态资源不受限，便于浏览器直接访问。
// 这是单机内网部署的最小保护，不替代正经鉴权。
func (s *Server) withMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rr := &recorder{ResponseWriter: w, status: 200}
		defer func() {
			if rec := recover(); rec != nil {
				log.Printf("[server] panic: %v", rec)
				http.Error(rr, "internal server error", http.StatusInternalServerError)
			}
			log.Printf("[server] %s %s %d %dms",
				r.Method, r.URL.RequestURI(), rr.status, time.Since(start).Milliseconds())
		}()

		if isDatasetGuidePath(r.URL.Path) && !s.authorizeDatasetGuide(w, r) {
			return
		}
		if s.cfg.Server.Secret != "" && strings.HasPrefix(r.URL.Path, "/api/") && !datasetapi.IsBearerPath(r.URL.Path) {
			if r.Header.Get("X-Sync-Secret") != s.cfg.Server.Secret {
				errJSON(w, http.StatusUnauthorized, "missing or wrong X-Sync-Secret")
				return
			}
		}
		h.ServeHTTP(rr, r)
	})
}

func (s *Server) authorizeDatasetGuide(w http.ResponseWriter, r *http.Request) bool {
	if s.cfg.Server.Secret == "" {
		errJSON(w, http.StatusForbidden, "dataset guide requires X-Sync-Secret configuration")
		return false
	}
	if r.Header.Get("X-Sync-Secret") != s.cfg.Server.Secret {
		errJSON(w, http.StatusUnauthorized, "missing or wrong X-Sync-Secret")
		return false
	}
	return true
}

func isDatasetGuidePath(path string) bool {
	const prefix = "/api/datasources/datasets/projects/"
	if !strings.HasPrefix(path, prefix) || !strings.HasSuffix(path, "/guide") {
		return false
	}
	tokenID := strings.TrimSuffix(strings.TrimPrefix(path, prefix), "/guide")
	return tokenID != "" && !strings.Contains(tokenID, "/")
}

// recorder 仅用于记录响应状态码，供日志中间件读取。
type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
