// internal/server/handlers.go — 所有 HTTP handler 的实现。
//
// 宪法对应：
//   - doc/04-api.md：每个 /api/* handler 的输入输出结构
//   - doc/05-pages.md：页面 handler（仅渲染 HTML shell）
//
// 响应统一约定：
//   成功 → {"ok": true, "data": <...>}
//   失败 → {"ok": false, "error": "<msg>"}
// 时间字段一律 RFC3339（UTC）；nil 指针序列化为 null。

package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

// ---------------------------------------------------------------------------
// 响应 helper
// ---------------------------------------------------------------------------

// writeJSON 把 v 以 JSON 写入 w，并设置 status 与 Content-Type。
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		// 编码失败通常只发生在连接断开，记日志即可
		log.Printf("[server] json encode: %v", err)
	}
}

// envelope 是统一的响应外壳。
type envelope struct {
	OK    bool   `json:"ok"`
	Data  any    `json:"data,omitempty"`
	Error string `json:"error,omitempty"`
}

// okJSON 返回成功结构 {ok:true, data:...}。
func okJSON(w http.ResponseWriter, data any) {
	writeJSON(w, http.StatusOK, envelope{OK: true, Data: data})
}

// errJSON 返回失败结构 {ok:false, error:"..."}，并指定 HTTP 状态码。
func errJSON(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, envelope{OK: false, Error: msg})
}

// ---------------------------------------------------------------------------
// 页面 handler
// ---------------------------------------------------------------------------

// pageData 是所有页面模板共享的最小渲染上下文。
//
// 前端用 Alpine.js 调 /api 拿真实数据，模板只需要：
//   - Active：当前激活的侧栏项 key（决定高亮）
//   - 配置计数：用于页面降级展示（如端点数为 0 时给提示）
type pageData struct {
	Active         string
	EndpointCount  int
	AccountCount   int
	SecretRequired bool
	// EndpointNames 提供给某些页面（如 logs）做下拉选项的初始值
	EndpointNames  []string
	AccountIDs     []string
	AccountOptions []pageAccountOption
}

type pageAccountOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func (s *Server) newPageData(active string) pageData {
	names := make([]string, 0, len(s.cfg.Endpoints))
	for _, e := range s.cfg.Endpoints {
		names = append(names, e.Name)
	}
	ids := make([]string, 0, len(s.cfg.Accounts))
	accountOptions := make([]pageAccountOption, 0, len(s.cfg.Accounts))
	for _, a := range s.cfg.Accounts {
		ids = append(ids, a.ID)
		name := strings.TrimSpace(a.Name)
		if name == "" {
			name = a.ID
		}
		accountOptions = append(accountOptions, pageAccountOption{ID: a.ID, Name: name})
	}
	return pageData{
		Active:         active,
		EndpointCount:  len(s.cfg.Endpoints),
		AccountCount:   len(s.cfg.Accounts),
		SecretRequired: s.cfg.Server.Secret != "",
		EndpointNames:  names,
		AccountIDs:     ids,
		AccountOptions: accountOptions,
	}
}

// renderPage 渲染 layout + 指定 page 的 content block。
//
// clone 基树法（见 server.parseTemplates）：每页 clone 出一棵独立树，
// 树内 "layout" 是骨架，该页 {{define "content"}} 覆盖 layout 的 block。
// 因此执行 "layout" 即输出完整页面（content 由 clone 进来的定义填充）。
func (s *Server) renderPage(w http.ResponseWriter, page string, data pageData) {
	tree, ok := s.pages[page]
	if !ok {
		errJSON(w, http.StatusInternalServerError, "未找到模板: "+page)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// 执行 "layout" 模板（content block 被该页覆盖）
	if err := tree.ExecuteTemplate(w, "layout.html", data); err != nil {
		log.Printf("[server] render %s: %v", page, err)
	}
}

// pageIndex 把首页 / 永久重定向到 /sync。
//
// 原「概览」页（sync_center）已删除：它只读进程内存状态（/api/status），
// 进程重启即全部回落「空闲」，与 /logs（查 sync_tasks，有完整历史）信息重叠且更弱。
// 数据新鲜度改由 /datasources 的「最后写入」列承担（db.TableLastSync）。
func (s *Server) pageIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/sync", http.StatusFound)
}

func (s *Server) pageSyncManage(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "sync_manage", s.newPageData("sync_manage"))
}

func (s *Server) pageLogs(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "logs", s.newPageData("logs"))
}

func (s *Server) pageDataSources(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "datasources", s.newPageData("datasources"))
}

func (s *Server) pageSettingsAPI(w http.ResponseWriter, r *http.Request) {
	s.renderPage(w, "settings_api", s.newPageData("settings_api"))
}

// ---------------------------------------------------------------------------
// API: GET /api/endpoints
// ---------------------------------------------------------------------------

type endpointOut struct {
	Name      string   `json:"name"`
	Display   string   `json:"display"`
	AccountID string   `json:"account_id"`
	Table     string   `json:"table"`
	Enabled   bool     `json:"enabled"`
	LastSync  *rfc3339 `json:"last_sync"` // 该表 MAX(synced_at)，来自 DB（重启不失忆）；无数据/查询失败为 null
}

func (s *Server) apiEndpoints(w http.ResponseWriter, r *http.Request) {
	items := make([]endpointOut, 0, len(s.cfg.Endpoints))
	for _, e := range s.cfg.Endpoints {
		// 数据新鲜度：读该表最后一次写入时间。这是「数据源」页展示用的只读信息，
		// 某张表尚未建好或缺 synced_at 列时不能让整表接口失败——降级为 null，前端显示「从未」。
		// N 个 endpoint = N 次 MAX(synced_at)，本页无轮询、仅加载/手动刷新触发，成本可接受。
		var lastSync *rfc3339
		if e.Table != "" {
			if ts, err := db.TableLastSync(s.dbx, e.Table); err == nil {
				lastSync = toRFC3339(ts)
			}
		}
		items = append(items, endpointOut{
			Name: e.Name, Display: e.Display,
			AccountID: e.Account, Table: e.Table, Enabled: e.Enabled,
			LastSync: lastSync,
		})
	}
	okJSON(w, items)
}

// ---------------------------------------------------------------------------
// API: GET /api/tasks
// ---------------------------------------------------------------------------

type taskItemOut struct {
	ID              int64    `json:"id"`
	Endpoint        string   `json:"endpoint"`
	AccountID       string   `json:"account_id"`
	Status          string   `json:"status"`
	TriggerType     string   `json:"trigger_type"`
	StartedAt       *rfc3339 `json:"started_at"`
	FinishedAt      *rfc3339 `json:"finished_at"`
	RecordsUpserted int      `json:"records_upserted"`
	PagesFetched    int      `json:"pages_fetched"`
	ErrorMessage    *string  `json:"error_message"`
	CreatedAt       rfc3339  `json:"created_at"`
	DurationSec     int64    `json:"duration_sec"`
}

type tasksOut struct {
	Items    []taskItemOut `json:"items"`
	Total    int           `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

func (s *Server) apiTasks(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	endpoint := q.Get("endpoint")
	account := q.Get("account")
	status := q.Get("status")
	dateFrom := q.Get("date_from")
	dateTo := q.Get("date_to")
	page := atoiOr(q.Get("page"), 1)
	pageSize := atoiOr(q.Get("page_size"), 20)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 200 {
		pageSize = 20
	}

	tasks, total, err := db.ListTasks(s.dbx, endpoint, account, status, dateFrom, dateTo, page, pageSize)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "查询任务失败: "+err.Error())
		return
	}
	items := make([]taskItemOut, 0, len(tasks))
	for _, t := range tasks {
		var dur int64
		if t.StartedAt != nil && t.FinishedAt != nil {
			dur = int64(t.FinishedAt.Sub(*t.StartedAt).Seconds())
			if dur < 0 {
				dur = 0
			}
		}
		// db.Task.ErrorMessage 是 sql.NullString；转成 *string（Valid=false → nil）
		var errMsg *string
		if t.ErrorMessage.Valid {
			s := t.ErrorMessage.String
			errMsg = &s
		}
		items = append(items, taskItemOut{
			ID:              t.ID,
			Endpoint:        t.Endpoint,
			AccountID:       t.AccountID,
			Status:          t.Status,
			TriggerType:     t.TriggerType,
			StartedAt:       toRFC3339(t.StartedAt),
			FinishedAt:      toRFC3339(t.FinishedAt),
			RecordsUpserted: t.RecordsUpserted,
			PagesFetched:    t.PagesFetched,
			ErrorMessage:    errMsg,
			CreatedAt:       *toRFC3339Must(t.CreatedAt),
			DurationSec:     dur,
		})
	}
	okJSON(w, tasksOut{Items: items, Total: total, Page: page, PageSize: pageSize})
}

// ---------------------------------------------------------------------------
// API: GET /api/tasks/{id}/logs
// ---------------------------------------------------------------------------

type taskLogOut struct {
	ID           int64   `json:"id"`
	TaskID       int64   `json:"task_id"`
	Page         *int    `json:"page"`
	HTTPStatus   *int    `json:"http_status"`
	APICode      *int    `json:"api_code"`
	RecordsCount int     `json:"records_count"`
	ErrorRaw     *string `json:"error_raw"`
	DurationMs   int     `json:"duration_ms"`
	CreatedAt    rfc3339 `json:"created_at"`
}

func (s *Server) apiTaskLogs(w http.ResponseWriter, r *http.Request) {
	id, ok := pathInt64(r, "id")
	if !ok {
		errJSON(w, http.StatusBadRequest, "无效的 task id")
		return
	}
	logs, err := db.ListTaskLogs(s.dbx, id)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, "查询日志失败: "+err.Error())
		return
	}
	out := make([]taskLogOut, 0, len(logs))
	for _, l := range logs {
		page := l.Page // db.TaskLog.Page 是 int；契约要求 *int 输出
		var hs, ac *int
		if l.HTTPStatus.Valid {
			v := int(l.HTTPStatus.Int64)
			hs = &v
		}
		if l.APICode.Valid {
			v := int(l.APICode.Int64)
			ac = &v
		}
		var errRaw *string
		if l.ErrorRaw.Valid {
			s := l.ErrorRaw.String
			errRaw = &s
		}
		out = append(out, taskLogOut{
			ID:           l.ID,
			TaskID:       l.TaskID,
			Page:         &page,
			HTTPStatus:   hs,
			APICode:      ac,
			RecordsCount: l.RecordsCount,
			ErrorRaw:     errRaw,
			DurationMs:   l.DurationMs,
			CreatedAt:    *toRFC3339Must(l.CreatedAt),
		})
	}
	okJSON(w, out)
}

// ---------------------------------------------------------------------------
// API: POST /api/sync/{name}
// ---------------------------------------------------------------------------

type syncTriggerIn struct {
	Force     bool     `json:"force"`
	StoreSids []string `json:"store_sids"` // 可选：本次只同步这些店铺；空=按配置白名单
	DateFrom  string   `json:"date_from"`
	DateTo    string   `json:"date_to"`
}

func validateSyncDateRange(dateFrom, dateTo string) error {
	if dateFrom == "" || dateTo == "" {
		return fmt.Errorf("日期范围必须同时填写开始和结束日期")
	}
	from, err := time.Parse("2006-01-02", dateFrom)
	if err != nil {
		return fmt.Errorf("开始日期必须是 YYYY-MM-DD")
	}
	to, err := time.Parse("2006-01-02", dateTo)
	if err != nil {
		return fmt.Errorf("结束日期必须是 YYYY-MM-DD")
	}
	if from.After(to) {
		return fmt.Errorf("结束日期不能早于开始日期")
	}
	return nil
}

// apiSyncTrigger 立即触发某 endpoint 的同步。
// task id 是 worker 异步产生，这里返回 ok+message 即可；前端在 /logs 页通过 /api/tasks 观察结果。
// body 可选携带 store_sids[]：仅对 iterate_by_store 的接口生效，按次覆盖店铺范围，
// 不写回 config.yaml（10-frontend-rework-flow.md §3.2）。
func (s *Server) apiSyncTrigger(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	w0 := s.reg.Get(name)
	if w0 == nil {
		errJSON(w, http.StatusNotFound, "未找到 endpoint: "+name)
		return
	}
	if w0.Status().Status == "disabled" {
		errJSON(w, http.StatusConflict, "接口已禁用，请先在同步配置的定时调度中启用: "+name)
		return
	}
	// 启动断言未过（最常见：目标表没建）→ 直接把原因告诉用户，不入队跑一个必败任务。
	if fe := w0.Status().FatalError; fe != "" {
		errJSON(w, http.StatusConflict, "接口不可同步（"+fe+"），请先建表并重启进程: "+name)
		return
	}
	var in syncTriggerIn
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	in.DateFrom = strings.TrimSpace(in.DateFrom)
	in.DateTo = strings.TrimSpace(in.DateTo)
	if in.DateFrom != "" || in.DateTo != "" {
		if err := validateSyncDateRange(in.DateFrom, in.DateTo); err != nil {
			errJSON(w, http.StatusBadRequest, err.Error())
			return
		}
		if w0.Endpoint.DateField != "" && in.DateFrom == in.DateTo {
			// Single-date endpoints use the same day for their verified DateField.
		} else if !w0.Endpoint.DateRangeCapable() {
			errJSON(w, http.StatusBadRequest, "该接口不支持日期范围：快照接口同步当前全量，单日接口按自身日期配置执行")
			return
		}
	}

	if !w0.TriggerManualWithRange(in.StoreSids, in.DateFrom, in.DateTo) {
		errJSON(w, http.StatusConflict, "同步任务已在运行或队列中，请在同步日志查看结果: "+name)
		return
	}
	okJSON(w, map[string]any{"message": "任务已加入队列，请在同步日志查看结果: " + name, "endpoint": name, "queued": true})
}

// ---------------------------------------------------------------------------
// API: POST /api/sync/{name}/cancel
// ---------------------------------------------------------------------------

type syncCancelIn struct {
	TaskID int64 `json:"task_id"`
}

func (s *Server) apiSyncCancel(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	w0 := s.reg.Get(name)
	if w0 == nil {
		errJSON(w, http.StatusNotFound, "未找到 endpoint: "+name)
		return
	}
	var in syncCancelIn
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if in.TaskID == 0 {
		errJSON(w, http.StatusBadRequest, "缺少 task_id")
		return
	}
	w0.Cancel(in.TaskID)
	okJSON(w, map[string]any{"message": "已请求取消", "task_id": in.TaskID})
}

// ---------------------------------------------------------------------------
// API: GET /api/settings
// ---------------------------------------------------------------------------

type accountStatusOut struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	TokenKnown        bool   `json:"token_known"`
	TokenValid        bool   `json:"token_valid"`
	TokenExpiresInSec int64  `json:"token_expires_in_sec"`
}

type settingsOut struct {
	Version     string             `json:"version"`
	UptimeSec   int64              `json:"uptime_sec"`
	DBConnected bool               `json:"db_connected"`
	BaseURL     string             `json:"base_url"`
	Accounts    []accountStatusOut `json:"accounts"`
}

func (s *Server) apiSettings(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	dbOK := true
	if err := s.dbx.PingContext(ctx); err != nil {
		dbOK = false
		log.Printf("[server] db ping: %v", err)
	}

	accounts := make([]accountStatusOut, 0, len(s.cfg.Accounts))
	for _, a := range s.cfg.Accounts {
		valid := false
		var exp int64
		if c := s.clients.Get(a.ID); c != nil {
			if h := c.TokenHolder(); h != nil {
				known := h.IsKnown()
				valid = h.IsValid()
				exp = h.ExpiresInSec()
				accounts = append(accounts, accountStatusOut{
					ID: a.ID, Name: a.Name, TokenKnown: known, TokenValid: valid, TokenExpiresInSec: exp,
				})
				continue
			}
		}
		accounts = append(accounts, accountStatusOut{
			ID: a.ID, Name: a.Name, TokenKnown: false, TokenValid: valid, TokenExpiresInSec: exp,
		})
	}

	okJSON(w, settingsOut{
		Version:     "0.1.0",
		UptimeSec:   int64(time.Since(s.startTime).Seconds()),
		DBConnected: dbOK,
		BaseURL:     s.baseURL,
		Accounts:    accounts,
	})
}

// 注：POST /api/settings/reload 的真实实现在 handlers_config.go（apiSettingsReload），
// 与账号/接口 CRUD、重启、字段查询等配置读写接口同属一处，便于统一维护。

// ---------------------------------------------------------------------------
// API: POST /api/settings/test-db
// ---------------------------------------------------------------------------

func (s *Server) apiSettingsTestDB(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	t0 := time.Now()
	if err := s.dbx.PingContext(ctx); err != nil {
		errJSON(w, http.StatusServiceUnavailable, "数据库不可达: "+err.Error())
		return
	}
	latency := time.Since(t0).Milliseconds()
	okJSON(w, map[string]any{"latency_ms": latency, "ok": true})
}

// ---------------------------------------------------------------------------
// 小工具
// ---------------------------------------------------------------------------

// rfc3339 是一个会以 RFC3339 序列化的 time.Time 包装类型。
// nil 指针 → JSON null（通过 *rfc3339 表达）；非空则 marshal 为 "2006-01-02T15:04:05Z"。
type rfc3339 time.Time

func (t rfc3339) MarshalJSON() ([]byte, error) {
	return []byte(`"` + time.Time(t).UTC().Format(time.RFC3339) + `"`), nil
}

// toRFC3339 把 *time.Time 转成 *rfc3339；nil 进 nil 出。
func toRFC3339(t *time.Time) *rfc3339 {
	if t == nil {
		return nil
	}
	v := rfc3339((*t).UTC())
	return &v
}

// toRFC3339Must 用于非空时间字段；输入为 time.Time 零值时返回当前时间的串。
func toRFC3339Must(t time.Time) *rfc3339 {
	v := rfc3339(t.UTC())
	return &v
}

// atoiOr 解析整型，失败回退到 def。
func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// pathInt64 从 mux 路径变量读 int64。
func pathInt64(r *http.Request, key string) (int64, bool) {
	v := r.PathValue(key)
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

// decodeJSON 把请求体解析到 v，允许空 body（返回 nil）。
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		if strings.Contains(err.Error(), "EOF") {
			return nil
		}
		return err
	}
	return nil
}

// 为防止 sql.ErrNoRows 在某些 helper 中被忽略而引入；当前 handler 没直接用，
// 但 db.Connected 检查可能用到。保留以备后用。
var _ = sql.ErrNoRows

// 保证引入 config 包用于未来扩展（避免删 import 循环警告）。
var _ = config.Account{}
