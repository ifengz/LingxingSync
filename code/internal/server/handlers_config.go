// internal/server/handlers_config.go — 配置读写 HTTP handler：账号/接口 CRUD、
// 数据源列结构查询、重启、测试连接，以及 POST /api/settings/reload 的真实实现。
//
// 宪法对应：doc/core/04-api.md「配置读写接口」章节 + 宪法 §7.5（配置热加载/重启）。
//
// 统一写盘流程（宪法 §7.5）：解析请求体 → 在 snap（Snapshot 深拷贝）上改 → Save
// （校验 + 备份 .bak + 原子写盘）→ ClassifyChange 判定热加载/重启 → 命中热加载则
// 立即下发 worker.Registry / worker.Scheduler / worker.LimiterRegistry → 响应统一
// 带 need_restart 标志。任一步失败都不影响磁盘上原有配置（Save 内部保证）。
//
// 本文件负责自注册除 reload 外的全部配置读写路由（registerConfigRoutes，server.go
// 只调一次）；POST /api/settings/reload 的路由已在 server.go 的 Routes() 里注册好，
// 本文件只提供 apiSettingsReload 方法本身，避免 ServeMux 重复注册 panic。
package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/robfig/cron/v3"

	"lingxing-sync/internal/api"
	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

// registerConfigRoutes 注册配置读写相关的路由（不含 reload，见文件头注释）。
func (s *Server) registerConfigRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/config", s.apiGetConfig)
	mux.HandleFunc("POST /api/accounts", s.apiCreateAccount)
	mux.HandleFunc("PUT /api/accounts/{id}", s.apiUpdateAccount)
	mux.HandleFunc("PUT /api/accounts/{id}/connection-check", s.apiUpdateConnectionCheck)
	mux.HandleFunc("DELETE /api/accounts/{id}", s.apiDeleteAccount)
	mux.HandleFunc("GET /api/accounts/{id}/stores", s.apiAccountStores)
	mux.HandleFunc("PUT /api/accounts/{id}/stores/{sid}/vc-profile", s.apiSaveVCStoreProfile)
	mux.HandleFunc("POST /api/accounts/{id}/stores/sync", s.apiAccountStoreSync)
	mux.HandleFunc("POST /api/accounts/{id}/stores/selection", s.apiSaveStoreSelection)
	mux.HandleFunc("POST /api/endpoints", s.apiCreateEndpoint)
	mux.HandleFunc("PUT /api/endpoints/{name}", s.apiUpdateEndpoint)
	mux.HandleFunc("DELETE /api/endpoints/{name}", s.apiDeleteEndpoint)
	mux.HandleFunc("GET /api/datasources/{table}/columns", s.apiDatasourceColumns)
	mux.HandleFunc("POST /api/settings/restart", s.apiRestart)
	mux.HandleFunc("POST /api/settings/test-connection", s.apiTestConnection)
}

// ---------------------------------------------------------------------------
// DTO：config.Account / config.Endpoint / config.Rate 只带 yaml 标签，不能直接
// 塞进 decodeJSON（json 标签缺失会导致字段全部丢失）。这里显式定义 json 版结构
// 并做双向转换，输入输出的字段名与 doc/core/04-api.md 保持一致（snake_case）。
// ---------------------------------------------------------------------------

// accountDTO 是账号的 HTTP 输入输出结构。
type accountDTO struct {
	ID              string             `json:"id"`
	Name            string             `json:"name"`
	QuotaGroup      string             `json:"quota_group"`
	AppKey          string             `json:"app_key"`
	AppSecret       string             `json:"app_secret"`
	ConnectionCheck connectionCheckDTO `json:"connection_check"`
}

type connectionCheckDTO struct {
	Cron    string `json:"cron"`
	Enabled bool   `json:"enabled"`
}

func accountToDTO(a config.Account) accountDTO {
	return accountDTO{
		ID:         a.ID,
		Name:       a.Name,
		QuotaGroup: a.QuotaGroup,
		AppKey:     a.AppKey,
		AppSecret:  a.AppSecret,
		ConnectionCheck: connectionCheckDTO{
			Cron: a.ConnectionCheck.Cron, Enabled: a.ConnectionCheck.Enabled,
		},
	}
}

func accountsToDTO(as []config.Account) []accountDTO {
	out := make([]accountDTO, 0, len(as))
	for _, a := range as {
		out = append(out, accountToDTO(a))
	}
	return out
}

func dtoToAccount(d accountDTO) config.Account {
	check := config.DefaultConnectionCheck()
	if d.ConnectionCheck.Cron != "" {
		check = config.ConnectionCheck{Cron: d.ConnectionCheck.Cron, Enabled: d.ConnectionCheck.Enabled}
	}
	return config.Account{
		ID:              d.ID,
		Name:            d.Name,
		QuotaGroup:      d.QuotaGroup,
		AppKey:          d.AppKey,
		AppSecret:       d.AppSecret,
		ConnectionCheck: check,
	}
}

// rateDTO 对应 config.Rate。
type rateDTO struct {
	Bucket          int    `json:"bucket"`
	IntervalMs      int    `json:"interval_ms"`
	MultiIntervalMs int    `json:"multi_interval_ms"`
	Dimension       string `json:"dimension"`
}

func rateToDTO(r config.Rate) rateDTO {
	return rateDTO{
		Bucket:          r.Bucket,
		IntervalMs:      r.IntervalMs,
		MultiIntervalMs: r.MultiIntervalMs,
		Dimension:       r.Dimension,
	}
}

func dtoToRate(d rateDTO) config.Rate {
	return config.Rate{
		Bucket:          d.Bucket,
		IntervalMs:      d.IntervalMs,
		MultiIntervalMs: d.MultiIntervalMs,
		Dimension:       d.Dimension,
	}
}

// endpointDTO 是接口的 HTTP 输入输出结构。
type endpointDTO struct {
	Name             string         `json:"name"`
	Display          string         `json:"display"`
	Account          string         `json:"account"`
	Path             string         `json:"path"`
	Method           string         `json:"method"`
	Table            string         `json:"table"`
	RecordIDFields   []string       `json:"record_id_fields"`
	Rate             rateDTO        `json:"rate"`
	Cron             string         `json:"cron"`
	Enabled          bool           `json:"enabled"`
	WindowDays       int            `json:"window_days"`
	WindowStartField string         `json:"window_start_field"`
	WindowEndField   string         `json:"window_end_field"`
	DateField        string         `json:"date_field"`
	DateOffsetDays   int            `json:"date_offset_days"`
	DateRangeCapable bool           `json:"date_range_capable"`
	ExtraParams      map[string]any `json:"extra_params"`
	IsStoreSource    bool           `json:"is_store_source"`
	IterateByStore   bool           `json:"iterate_by_store"`
	StoreParamName   string         `json:"store_param_name"`
	StoreSids        []string       `json:"store_sids"`
	StoreType        string         `json:"store_type"`

	// 以下两个是**只读运行态**，由 apiGetConfig 从 worker registry 填充，
	// dtoToEndpoint 故意不映射它们——运行态永远写不回 config.yaml。
	//
	// 为什么放在这个双向 DTO 里而不是另开接口：前端 saveRow 用
	// Object.assign({}, e) 整行回传，而 decodeJSON 开了 DisallowUnknownFields。
	// 若这两个键只出现在响应里、不在 DTO 里，坏接口那一行一保存就 400。
	// omitempty 保证健康行根本不带这两个键，回传体与改动前完全一致。
	FatalError string   `json:"fatal_error,omitempty"` // 非空=该接口不可同步（最常见：表没建）
	Warnings   []string `json:"warnings,omitempty"`    // 不阻断同步的表结构告警（缺声明列）
}

func endpointToDTO(e config.Endpoint) endpointDTO {
	return endpointDTO{
		Name:             e.Name,
		Display:          e.Display,
		Account:          e.Account,
		Path:             e.Path,
		Method:           e.Method,
		Table:            e.Table,
		RecordIDFields:   e.RecordIDFields,
		Rate:             rateToDTO(e.Rate),
		Cron:             e.Cron,
		Enabled:          e.Enabled,
		WindowDays:       e.WindowDays,
		WindowStartField: e.WindowStartField,
		WindowEndField:   e.WindowEndField,
		DateField:        e.DateField,
		DateOffsetDays:   e.DateOffsetDays,
		DateRangeCapable: e.DateRangeCapable(),
		ExtraParams:      e.ExtraParams,
		IsStoreSource:    e.IsStoreSource,
		IterateByStore:   e.IterateByStore,
		StoreParamName:   e.StoreParamName,
		StoreSids:        e.StoreSids,
		StoreType:        e.StoreType,
	}
}

func endpointsToDTO(es []config.Endpoint) []endpointDTO {
	out := make([]endpointDTO, 0, len(es))
	for _, e := range es {
		out = append(out, endpointToDTO(e))
	}
	return out
}

func dtoToEndpoint(d endpointDTO) config.Endpoint {
	return config.Endpoint{
		Name:             d.Name,
		Display:          d.Display,
		Account:          d.Account,
		Path:             d.Path,
		Method:           d.Method,
		Table:            d.Table,
		RecordIDFields:   d.RecordIDFields,
		Rate:             dtoToRate(d.Rate),
		Cron:             d.Cron,
		Enabled:          d.Enabled,
		WindowDays:       d.WindowDays,
		WindowStartField: d.WindowStartField,
		WindowEndField:   d.WindowEndField,
		DateField:        d.DateField,
		DateOffsetDays:   d.DateOffsetDays,
		ExtraParams:      d.ExtraParams,
		IsStoreSource:    d.IsStoreSource,
		IterateByStore:   d.IterateByStore,
		StoreParamName:   d.StoreParamName,
		StoreSids:        d.StoreSids,
		StoreType:        d.StoreType,
	}
}

// configOut 是 GET /api/config 的响应结构。
type configOut struct {
	Accounts  []accountDTO  `json:"accounts"`
	Endpoints []endpointDTO `json:"endpoints"`
}

type accountStoresOut struct {
	AccountID    string            `json:"account_id"`
	Total        int               `json:"total"`
	LastSyncedAt *time.Time        `json:"last_synced_at"`
	Configured   bool              `json:"configured"` // 该账号是否已保存过店铺同步选择（false=从未配置，前端默认全勾）
	Items        []db.StoreSummary `json:"items"`
}

// ---------------------------------------------------------------------------
// 共享写盘流程（宪法 §7.5）：账号/接口 CRUD 全部收敛到这一个函数，保证
// 校验 → 备份 → 原子写盘 → 热加载判定 → 下发 worker 的逻辑只有一处实现。
// ---------------------------------------------------------------------------

// refreshLimitersFromConfig 按 cfg.Endpoints 逐个 UpdateOrCreate 对应限流器。
// 热加载（write flow 的 ChangeHot 分支）与 /api/settings/reload 都要做同一件事，
// 抽成一个方法避免两处漂移。
func (s *Server) refreshLimitersFromConfig(cfg *config.Config) {
	for _, ep := range cfg.Endpoints {
		qg := ep.Account
		if acc := cfg.FindAccount(ep.Account); acc != nil {
			qg = acc.QuotaGroupOrID()
		}
		s.limiters.UpdateOrCreate(qg, ep.Path, ep.Rate.Bucket, ep.Rate.IntervalMs)
	}
}

// applyConfigWrite 保存 snap（调用方已在其上完成本次改动），成功后按
// ClassifyChange(old, snap) 决定是否立即热加载，最终统一响应 {message, need_restart}。
// 失败时已写好 400 响应，调用方直接 return。
// extra 里的键会并入响应 JSON（如建账号回传自动配定的 account_id），供前端读结构化字段
// 而非解析 message 文案。
func (s *Server) applyConfigWrite(w http.ResponseWriter, old, snap *config.Config, message string, extra ...map[string]any) {
	if err := s.store.Save(snap); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	kind := config.ClassifyChange(old, snap)
	if kind == config.ChangeHot {
		s.reg.ApplyHotReload(snap)
		if err := s.sched.Rebuild(snap); err != nil {
			log.Printf("[server] 热加载 Rebuild 调度失败: %v", err)
		}
		s.refreshLimitersFromConfig(snap)
	}
	// 单管理员工具：接受良性的指针切换竞态（无锁快速刷新，供其它 handler 读到新配置）。
	s.cfg = snap
	resp := map[string]any{"message": message, "need_restart": kind == config.ChangeRestart}
	for _, m := range extra {
		for k, v := range m {
			resp[k] = v
		}
	}
	okJSON(w, resp)
}

// ---------------------------------------------------------------------------
// API: GET /api/config
// ---------------------------------------------------------------------------

// apiGetConfig 返回完整配置供 UI 编辑；app_secret 经 Mask 脱敏，绝不明文回传。
//
// 除配置本身，还附带每个接口的只读运行态（fatal_error / warnings），让同步配置页
// 能把「表没建」的接口直接标红并写明原因——否则用户只能去翻启动日志。
// registry 里查不到该 name（刚热加载新增、还没重启）时两者留空，属正确表现：
// 那个接口确实还没有 worker，谈不上健康与否。
func (s *Server) apiGetConfig(w http.ResponseWriter, r *http.Request) {
	masked := s.store.Mask()
	eps := endpointsToDTO(masked.Endpoints)
	for i := range eps {
		w0 := s.reg.Get(eps[i].Name)
		if w0 == nil {
			continue
		}
		st := w0.Status()
		eps[i].FatalError = st.FatalError
		eps[i].Warnings = st.Warnings
	}
	okJSON(w, configOut{
		Accounts:  accountsToDTO(masked.Accounts),
		Endpoints: eps,
	})
}

// ---------------------------------------------------------------------------
// API: POST /api/accounts
// ---------------------------------------------------------------------------

// nextAvailableAccountID 在填入的 base 与现有账号大小写不敏感撞名时，自动往后找第一个
// 可用的 base_2 / base_3 …（延用 sc_us_1/sc_us_2 的命名直觉），保证系统自动区分、不靠人眼。
// base 本身可用则原样返回。account_id 列宽 32，超长的后缀不会产生（base 已过 slug 校验 ≤32，
// 实际账号数远达不到需要截断的规模）。
func nextAvailableAccountID(base string, accounts []config.Account) string {
	taken := make(map[string]bool, len(accounts))
	for _, a := range accounts {
		taken[config.NormID(a.ID)] = true
	}
	if !taken[config.NormID(base)] {
		return base
	}
	for n := 2; ; n++ {
		cand := fmt.Sprintf("%s_%d", base, n)
		if !taken[config.NormID(cand)] {
			return cand
		}
	}
}

// apiCreateAccount 新增账号。name/app_key/app_secret 必填；app_secret 不能是脱敏占位串。
// id 须符合 slug 字符集；与现有账号大小写不敏感撞名时自动改配可用 ID（base_2/base_3…），
// 响应回显最终 id。新增账号属结构性变更，恒为 need_restart:true。
func (s *Server) apiCreateAccount(w http.ResponseWriter, r *http.Request) {
	var in accountDTO
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if in.ID == "" || in.Name == "" || in.AppKey == "" || in.AppSecret == "" {
		errJSON(w, http.StatusBadRequest, "id/name/app_key/app_secret 不能为空")
		return
	}
	if strings.Contains(in.AppSecret, "****") {
		errJSON(w, http.StatusBadRequest, "app_secret 不能是脱敏后的占位串")
		return
	}
	if !config.ValidAccountID(in.ID) {
		errJSON(w, http.StatusBadRequest, "账号 id 非法：只允许字母/数字/下划线/连字符，首尾为字母或数字，长度 1–32")
		return
	}

	old := s.store.Current()
	snap := s.store.Snapshot()
	// 撞名不再报错：以填入 id 为 base 自动配一个可用 id（大小写不敏感唯一），人不用操心。
	requestedID := in.ID
	finalID := nextAvailableAccountID(requestedID, snap.Accounts)
	in.ID = finalID
	snap.Accounts = append(snap.Accounts, dtoToAccount(in))

	msg := "账号已新增: " + finalID
	if finalID != requestedID {
		msg = fmt.Sprintf("账号已新增: %s（%q 已被占用，自动配为 %s）", finalID, requestedID, finalID)
	}
	s.applyConfigWrite(w, old, snap, msg, map[string]any{"account_id": finalID})
}

// ---------------------------------------------------------------------------
// API: PUT /api/accounts/{id}
// ---------------------------------------------------------------------------

// apiUpdateAccount 更新账号。app_secret 若为空或含 "****"（脱敏占位），保留原
// 真实密钥，避免脱敏值覆盖真值（宪法 doc/04-api.md）。
func (s *Server) apiUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in accountDTO
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}

	old := s.store.Current()
	snap := s.store.Snapshot()
	acc := snap.FindAccount(id)
	if acc == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}

	if in.Name != "" {
		acc.Name = in.Name
	}
	if in.QuotaGroup != "" {
		acc.QuotaGroup = in.QuotaGroup
	}
	if in.AppKey != "" {
		acc.AppKey = in.AppKey
	}
	if in.AppSecret != "" && !strings.Contains(in.AppSecret, "****") {
		acc.AppSecret = in.AppSecret
	}

	s.applyConfigWrite(w, old, snap, "账号已更新: "+id)
}

// apiUpdateConnectionCheck 保存账号级的连接检查/Token 续租计划。
func (s *Server) apiUpdateConnectionCheck(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var in connectionCheckDTO
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if _, err := cron.ParseStandard(in.Cron); err != nil {
		errJSON(w, http.StatusBadRequest, "Cron 表达式无效: "+err.Error())
		return
	}

	old := s.store.Current()
	snap := s.store.Snapshot()
	acc := snap.FindAccount(id)
	if acc == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}
	acc.ConnectionCheck = config.ConnectionCheck{Cron: in.Cron, Enabled: in.Enabled}
	s.applyConfigWrite(w, old, snap, "连接续租计划已更新: "+id)
}

// ---------------------------------------------------------------------------
// API: DELETE /api/accounts/{id}
// ---------------------------------------------------------------------------

// apiDeleteAccount 删除账号。若仍有 endpoint 引用该账号，409 拦截并列出引用
// 接口名，不级联删除（宪法 doc/04-api.md）。
func (s *Server) apiDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	old := s.store.Current()
	snap := s.store.Snapshot()
	if snap.FindAccount(id) == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}

	var refs []string
	for _, ep := range snap.Endpoints {
		if config.NormID(ep.Account) == config.NormID(id) {
			refs = append(refs, ep.Name)
		}
	}
	if len(refs) > 0 {
		errJSON(w, http.StatusConflict, fmt.Sprintf(
			"账号 %s 仍被 %d 个接口引用，请先删除接口: %v", id, len(refs), refs))
		return
	}

	kept := make([]config.Account, 0, len(snap.Accounts))
	for _, a := range snap.Accounts {
		if config.NormID(a.ID) != config.NormID(id) {
			kept = append(kept, a)
		}
	}
	snap.Accounts = kept

	s.applyConfigWrite(w, old, snap, "账号已删除: "+id)
}

// apiAccountStores 返回已配置账号的本地店铺摘要。只允许读取仍存在于配置中的账号，
// 避免已删除账号的历史行成为可见数据。
func (s *Server) apiAccountStores(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.cfg == nil || s.cfg.FindAccount(id) == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}

	items, lastSyncedAt, err := db.ListStoresForAccount(s.dbx, id)
	if err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	// 用 store_sync_selection 注解每个店铺的 enabled（复选框回填初值）：
	//   该账号从未保存过选择（configured=false）→ 全部默认勾选（与「空=全放行」一致）；
	//   已保存 → 映射里 enabled=1 的为勾选，未出现的新店铺视作未勾选。
	sel, configured, serr := db.LoadStoreSelection(s.dbx, id)
	if serr != nil {
		errJSON(w, http.StatusServiceUnavailable, serr.Error())
		return
	}
	for i := range items {
		if !configured {
			items[i].Enabled = true
			continue
		}
		items[i].Enabled = sel[items[i].SID]
	}
	okJSON(w, accountStoresOut{
		AccountID:    id,
		Total:        len(items),
		LastSyncedAt: lastSyncedAt,
		Configured:   configured,
		Items:        items,
	})
}

func findVCStoreForProfile(items []db.StoreSummary, sid string) (db.StoreSummary, int, error) {
	for _, item := range items {
		if item.SID != sid {
			continue
		}
		if item.StoreType != "VC" {
			return db.StoreSummary{}, http.StatusBadRequest, fmt.Errorf("店铺 %s 不是 VC 店铺", sid)
		}
		return item, http.StatusOK, nil
	}
	return db.StoreSummary{}, http.StatusNotFound, fmt.Errorf("未找到店铺: %s", sid)
}

// apiSaveVCStoreProfile 只允许给当前账号实际存在的 VC 店铺保存人工 Profile ID。
// 空 profile_id 删除映射；SC 店铺和未知 sid 不写库。
func (s *Server) apiSaveVCStoreProfile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	sid := r.PathValue("sid")
	if s.cfg == nil || s.cfg.FindAccount(id) == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}
	var in struct {
		ProfileID *string `json:"profile_id"`
	}
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if in.ProfileID == nil {
		errJSON(w, http.StatusBadRequest, "profile_id 不能为空；清除映射请传空字符串")
		return
	}

	items, _, err := db.ListStoresForAccount(s.dbx, id)
	if err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	if _, status, err := findVCStoreForProfile(items, sid); err != nil {
		errJSON(w, status, err.Error())
		return
	}
	profileID := strings.TrimSpace(*in.ProfileID)
	if len(profileID) > 32 {
		errJSON(w, http.StatusBadRequest, "profile_id 长度不能超过 32")
		return
	}
	if err := db.SaveVCStoreProfile(s.dbx, id, sid, profileID); err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	message := "VC 广告 Profile ID 已保存"
	if profileID == "" {
		message = "VC 广告 Profile ID 已清除"
	}
	okJSON(w, map[string]any{"account_id": id, "sid": sid, "profile_id": profileID, "message": message})
}

// apiAccountStoreSync 触发该账号全部店铺来源接口（如 SC + VC 各一个）。前端不传 endpoint 名，
// 避免把账号页的"店铺目录"错误绑定到其他业务接口。多个 store source 全部触发，各自独立排队。
func (s *Server) apiAccountStoreSync(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	cfg := s.cfg
	if s.store != nil {
		cfg = s.store.Current()
	}
	if cfg == nil || cfg.FindAccount(id) == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}

	var names []string
	for _, ep := range cfg.Endpoints {
		if config.NormID(ep.Account) != config.NormID(id) || !ep.IsStoreSource {
			continue
		}
		names = append(names, ep.Name)
	}
	if len(names) == 0 {
		errJSON(w, http.StatusConflict, "该账号未配置店铺目录接口: "+id)
		return
	}
	queued := make([]string, 0, len(names))
	skipped := make([]string, 0)
	for _, name := range names {
		w0 := s.reg.Get(name)
		if w0 == nil {
			skipped = append(skipped, name+"(未就绪)")
			continue
		}
		if w0.Status().Status == "disabled" {
			skipped = append(skipped, name+"(已禁用)")
			continue
		}
		if fe := w0.Status().FatalError; fe != "" {
			// 启动断言未过（多半是目标表没建）：跳过它，其余店铺目录接口照常刷新。
			skipped = append(skipped, name+"(目标表未就绪)")
			continue
		}
		if !w0.TriggerManual(nil) {
			skipped = append(skipped, name+"(运行中)")
			continue
		}
		queued = append(queued, name)
	}
	msg := "店铺目录刷新已加入队列: " + strings.Join(queued, ", ")
	if len(skipped) > 0 {
		msg += "；跳过: " + strings.Join(skipped, ", ")
	}
	okJSON(w, map[string]any{"message": msg, "endpoints": queued, "queued": len(queued) > 0})
}

// apiSaveAccountStores 覆盖式保存某账号的「店铺参与同步」选择。
// 请求体 {sids:[...]} 是勾选参与同步的店铺；后端以本地 ls_stores 的当前店铺全集为准，
// 对每个店铺写一行（在 sids 里→enabled=1，否则→0），前端传来的未知 sid 自动忽略。
// 只写 store_sync_selection（系统表），不碰 ls_* 数据表，不触发同步。
func (s *Server) apiSaveStoreSelection(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if s.cfg == nil || s.cfg.FindAccount(id) == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}
	var in struct {
		SIDs []string `json:"sids"`
	}
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	// 全集以本地 ls_stores 为准，杜绝前端伪造 sid 越权写入。店铺选择是账号级、跨接口共用，
	// 不按 store_type 过滤（传空）：选择表存全部店铺的开关，由各接口的 store_type 在迭代时再筛。
	allSIDs, err := db.QuerySIDsForAccount(s.dbx, id, "")
	if err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	// 勾选集与全集取交集（忽略不存在于本地的 sid）。
	valid := make(map[string]struct{}, len(allSIDs))
	for _, sid := range allSIDs {
		valid[sid] = struct{}{}
	}
	enabled := make([]string, 0, len(in.SIDs))
	for _, sid := range in.SIDs {
		if _, ok := valid[sid]; ok {
			enabled = append(enabled, sid)
		}
	}
	if err := db.SaveStoreSelection(s.dbx, id, allSIDs, enabled); err != nil {
		errJSON(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	okJSON(w, map[string]any{
		"account_id": id,
		"total":      len(allSIDs),
		"enabled":    len(enabled),
		"message":    fmt.Sprintf("已保存：%d 个店铺参与后续同步", len(enabled)),
	})
}

// ---------------------------------------------------------------------------
// API: POST /api/endpoints
// ---------------------------------------------------------------------------

// apiCreateEndpoint 新增接口。name 全局唯一、account 必须存在、
// path/method/table/record_id_fields 必填、table 必须已在 DB 建表。
// 新增接口属结构性变更，恒为 need_restart:true。
func (s *Server) apiCreateEndpoint(w http.ResponseWriter, r *http.Request) {
	var in endpointDTO
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if in.Name == "" || in.Path == "" || in.Method == "" || in.Table == "" {
		errJSON(w, http.StatusBadRequest, "name/path/method/table 不能为空")
		return
	}
	if len(in.RecordIDFields) == 0 {
		errJSON(w, http.StatusBadRequest, "record_id_fields 不能为空")
		return
	}

	old := s.store.Current()
	snap := s.store.Snapshot()
	for _, e := range snap.Endpoints {
		if e.Name == in.Name {
			errJSON(w, http.StatusBadRequest, "接口 name 已存在: "+in.Name)
			return
		}
	}
	if snap.FindAccount(in.Account) == nil {
		errJSON(w, http.StatusBadRequest, "account 不存在: "+in.Account)
		return
	}
	// 限流键 (quota_group, path) 不得与现有接口重复：否则两个接口共享同一个 rate.Limiter
	// 桶，一个翻页占满配额会拖慢另一个——正是「各接口独立、互不牵连」要杜绝的（CLAUDE.md §1.1）。
	// 在此 fail-loud 拦住，比等 Save→validate 报错更早、消息更直白（不带「校验新配置」前缀）。
	if owner, dup := snap.ConflictingLimiterKey(dtoToEndpoint(in)); dup {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf(
			"限流键 (quota_group=%s, path=%s) 已被接口 %s 占用；换 path 或换 quota_group，勿共享同一限流桶",
			snap.QuotaGroupOf(in.Account), in.Path, owner))
		return
	}
	// fail-loud：目标表必须已建好（宪法 §5）。Worker 启动时 GetTableColumns 会读不到列
	// 而走 main.go FATAL 退出；这里在创建阶段就拦住，避免重启后才发现表不存在。
	// 文档契约（04-api.md）：table 必须已建表，否则 400。
	if _, err := db.GetTableColumns(s.dbx, in.Table); err != nil {
		errJSON(w, http.StatusBadRequest, "目标表 "+in.Table+" 未建表或不可读，请先执行 migrations 建表: "+err.Error())
		return
	}
	snap.Endpoints = append(snap.Endpoints, dtoToEndpoint(in))

	s.applyConfigWrite(w, old, snap, "接口已新增: "+in.Name)
}

// ---------------------------------------------------------------------------
// API: PUT /api/endpoints/{name}
// ---------------------------------------------------------------------------

// apiUpdateEndpoint 更新接口。只替换可编辑字段（display/path/method/table/
// record_id_fields/rate/cron/enabled/window_days/extra_params/store_sids/
// iterate_by_store/store_param_name）；account 不可通过本接口改（改账号归属
// 属结构性变更，需走删除重建）。path/method/table 变化会被 ClassifyChange
// 判定为 ChangeRestart；cron/rate/enabled/store_sids 等变化判定为 ChangeHot。
func (s *Server) apiUpdateEndpoint(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var in endpointDTO
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}

	old := s.store.Current()
	snap := s.store.Snapshot()
	idx := -1
	for i, e := range snap.Endpoints {
		if e.Name == name {
			idx = i
			break
		}
	}
	if idx == -1 {
		errJSON(w, http.StatusNotFound, "未找到接口: "+name)
		return
	}

	ep := &snap.Endpoints[idx]
	ep.Display = in.Display
	ep.Path = in.Path
	ep.Method = in.Method
	ep.Table = in.Table
	ep.RecordIDFields = in.RecordIDFields
	ep.Rate = dtoToRate(in.Rate)
	ep.Cron = in.Cron
	ep.Enabled = in.Enabled
	ep.WindowDays = in.WindowDays
	ep.DateOffsetDays = in.DateOffsetDays
	ep.ExtraParams = in.ExtraParams
	ep.StoreSids = in.StoreSids
	ep.IterateByStore = in.IterateByStore
	ep.StoreParamName = in.StoreParamName

	s.applyConfigWrite(w, old, snap, "接口已更新: "+name)
}

// ---------------------------------------------------------------------------
// API: DELETE /api/endpoints/{name}
// ---------------------------------------------------------------------------

// apiDeleteEndpoint 删除接口配置（停止其调度），不删除已同步落库的数据。
func (s *Server) apiDeleteEndpoint(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	old := s.store.Current()
	snap := s.store.Snapshot()
	found := false
	kept := make([]config.Endpoint, 0, len(snap.Endpoints))
	for _, e := range snap.Endpoints {
		if e.Name == name {
			found = true
			continue
		}
		kept = append(kept, e)
	}
	if !found {
		errJSON(w, http.StatusNotFound, "未找到接口: "+name)
		return
	}
	snap.Endpoints = kept

	s.applyConfigWrite(w, old, snap, "接口已删除: "+name)
}

// ---------------------------------------------------------------------------
// API: GET /api/datasources/{table}/columns
// ---------------------------------------------------------------------------

// columnInfoRow 是 INFORMATION_SCHEMA.COLUMNS 单行查询结果（sqlx 用 db 标签映射列名）。
type columnInfoRow struct {
	ColumnName string `db:"COLUMN_NAME"`
	ColumnType string `db:"COLUMN_TYPE"`
	ColumnKey  string `db:"COLUMN_KEY"`
}

// datasourceColumnOut 是对外输出的单列结构。
type datasourceColumnOut struct {
	Name      string `json:"name"`
	Type      string `json:"type"`
	IsPrimary bool   `json:"is_primary"`
}

// apiDatasourceColumns 查询目标表的真实列结构。
// 出于安全考虑，只允许查询当前配置里被某个 endpoint 引用的表名，防止任意表探测。
func (s *Server) apiDatasourceColumns(w http.ResponseWriter, r *http.Request) {
	table := r.PathValue("table")

	allowed := false
	for _, ep := range s.cfg.Endpoints {
		if ep.Table == table {
			allowed = true
			break
		}
	}
	if !allowed {
		errJSON(w, http.StatusNotFound, "未知数据表: "+table)
		return
	}

	const q = `
SELECT COLUMN_NAME, COLUMN_TYPE, COLUMN_KEY
FROM INFORMATION_SCHEMA.COLUMNS
WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ?
ORDER BY ORDINAL_POSITION
`
	var rows []columnInfoRow
	if err := s.dbx.Select(&rows, q, table); err != nil {
		errJSON(w, http.StatusInternalServerError, "查询表结构失败: "+err.Error())
		return
	}

	cols := make([]datasourceColumnOut, 0, len(rows))
	for _, c := range rows {
		cols = append(cols, datasourceColumnOut{
			Name:      c.ColumnName,
			Type:      c.ColumnType,
			IsPrimary: c.ColumnKey == "PRI",
		})
	}
	okJSON(w, map[string]any{"table": table, "columns": cols})
}

// ---------------------------------------------------------------------------
// API: POST /api/settings/restart
// ---------------------------------------------------------------------------

// apiRestart 优雅重启进程：syscall.Exec 原地替换进程镜像，PID 不变（宝塔
// Supervisor 无感）。响应必须先写完并让客户端收到，才能替换进程，因此在
// goroutine 里 sleep 一小段再 Exec，给 ResponseWriter 留出 flush 时间。
func (s *Server) apiRestart(w http.ResponseWriter, r *http.Request) {
	okJSON(w, map[string]any{"message": "正在重启…"})

	go func() {
		time.Sleep(300 * time.Millisecond)
		exe, err := os.Executable()
		if err != nil {
			log.Printf("[server] 重启失败，取不到可执行文件路径: %v", err)
			return
		}
		if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
			log.Printf("[server] syscall.Exec 重启失败: %v", err)
		}
	}()
}

// ---------------------------------------------------------------------------
// API: POST /api/settings/test-connection?account={id}
// ---------------------------------------------------------------------------

// apiTestConnection 用指定账号真实取一次 token，验证 app_key/app_secret 是否可用。
func (s *Server) apiTestConnection(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("account")
	acc := s.store.Current().FindAccount(id)
	if acc == nil {
		errJSON(w, http.StatusNotFound, "未找到账号: "+id)
		return
	}

	client := s.clients.Get(id)
	if client == nil {
		client = api.NewClient(acc, s.baseURL)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := client.TokenHolder().ForceRefresh(ctx); err != nil {
		errJSON(w, http.StatusBadGateway, err.Error())
		return
	}

	okJSON(w, map[string]any{
		"token_valid":    true,
		"expires_in_sec": client.TokenHolder().ExpiresInSec(),
	})
}

// ---------------------------------------------------------------------------
// API: POST /api/settings/reload
//
// 路由已在 server.go 的 Routes() 里注册（mux.HandleFunc("POST /api/settings/reload",
// s.apiSettingsReload)），本文件只提供方法本身，不再重复 HandleFunc，避免 ServeMux
// 对同一 pattern 注册两次而在启动时 panic。
// ---------------------------------------------------------------------------

// apiSettingsReload 从磁盘重新读取 config.yaml，与当前内存配置比对：
//   - ChangeRestart（结构性变更）：不应用，仅提示需要重启（避免运行中 worker/
//     调度器与磁盘配置的结构产生不一致）。
//   - 否则（ChangeHot 或 ChangeNone）：Save 落盘、下发 worker.Registry /
//     worker.Scheduler / worker.LimiterRegistry，立即热加载生效。
func (s *Server) apiSettingsReload(w http.ResponseWriter, r *http.Request) {
	newCfg, err := config.Load(s.configPath)
	if err != nil {
		errJSON(w, http.StatusBadRequest, "重新读取配置失败: "+err.Error())
		return
	}

	old := s.store.Current()
	kind := config.ClassifyChange(old, newCfg)
	if kind == config.ChangeRestart {
		okJSON(w, map[string]any{"message": "结构性变更，需重启生效", "need_restart": true})
		return
	}

	if err := s.store.Save(newCfg); err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	s.reg.ApplyHotReload(newCfg)
	if err := s.sched.Rebuild(newCfg); err != nil {
		log.Printf("[server] reload Rebuild 调度失败: %v", err)
	}
	s.refreshLimitersFromConfig(newCfg)
	s.cfg = newCfg

	okJSON(w, map[string]any{"message": "配置已热加载", "need_restart": false})
}
