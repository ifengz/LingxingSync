// internal/server/handlers_catalog.go — 接口清单（Catalog）HTTP handler。
//
// 目标（CLAUDE.md §1「加接口极简单」的 UI 版）：让不懂技术的人也能安全加接口——
// 从内置清单挑一个模板 → 选账号 → 点启用，全程不碰 SQL / JSON / 表名 / 唯一键。
//
// 两条路由：
//   - GET  /api/catalog        列出所有模板 + 账号，并标注每个模板在每个账号下是否已启用
//   - POST /api/catalog/enable {key, account} → 用模板生成 Endpoint 追加进配置
//
// 「启用」完全复用 apiCreateEndpoint 的校验链（账号存在 / 名称不重复 / 限流键不冲突 /
// 目标表已建）与 applyConfigWrite（校验+备份+原子写+重启提示），不新增任何运行时机制。
package server

import (
	"fmt"
	"net/http"

	"lingxing-sync/internal/config"
	"lingxing-sync/internal/db"
)

// registerCatalogRoutes 注册接口清单相关路由。由 server.go 的 Routes() 调一次。
func (s *Server) registerCatalogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/catalog", s.apiCatalogList)
	mux.HandleFunc("POST /api/catalog/enable", s.apiCatalogEnable)
}

// catalogAccountRef 是清单页下拉里的一个账号选项。
type catalogAccountRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// catalogTemplateOut 是清单里的一条模板对外结构。
// EnabledAccounts 列出「已经启用了本模板（同 quota_group+path 已被占用）」的账号 ID，
// 前端据此把这些账号从可选项里划掉/置灰，避免重复启用撞限流键。
type catalogTemplateOut struct {
	Key             string   `json:"key"`
	Display         string   `json:"display"`
	Summary         string   `json:"summary"`
	Path            string   `json:"path"`
	Method          string   `json:"method"`
	Table           string   `json:"table"`
	DefaultCron     string   `json:"default_cron"`
	ParamShape      string   `json:"param_shape"`      // 人话描述参数形态（滚动/单日期/全量），给用户看
	IterateByStore  bool     `json:"iterate_by_store"` // 是否按店铺逐个拉
	EnabledAccounts []string `json:"enabled_accounts"`
}

// catalogListOut 是 GET /api/catalog 的响应。
type catalogListOut struct {
	Templates []catalogTemplateOut `json:"templates"`
	Accounts  []catalogAccountRef  `json:"accounts"`
}

// catalogAccountEnabled 判断某账号是否已经启用模板。
// 精确 name 是 catalog 生成的标准形式；同 path 冲突兼容历史手工配置的自定义 name。
func catalogAccountEnabled(cfg *config.Config, entry config.CatalogEntry, accountID string) bool {
	candidate := entry.ToEndpoint(accountID)
	for _, ep := range cfg.Endpoints {
		if ep.Name == candidate.Name {
			return true
		}
	}
	_, conflict := cfg.ConflictingLimiterKey(candidate)
	return conflict
}

// paramShapeText 把模板的参数形态翻译成一句人话，给不懂技术的用户在清单里看。
func paramShapeText(e config.CatalogEntry) string {
	switch {
	case e.DateField != "":
		if e.DateOffsetDays <= 0 {
			return "每天拉当天数据"
		}
		if e.DateOffsetDays == 1 {
			return "每天拉昨天数据"
		}
		return fmt.Sprintf("每天拉 %d 天前数据", e.DateOffsetDays)
	case e.WindowDays > 0:
		return fmt.Sprintf("每次滚动拉近 %d 天", e.WindowDays)
	default:
		return "每次全量拉取"
	}
}

// apiCatalogList 列出内置清单模板 + 可用账号，并标注每个模板已在哪些账号启用。
func (s *Server) apiCatalogList(w http.ResponseWriter, r *http.Request) {
	cfg := s.store.Current()

	accounts := make([]catalogAccountRef, 0, len(cfg.Accounts))
	for _, a := range cfg.Accounts {
		accounts = append(accounts, catalogAccountRef{ID: a.ID, Name: a.Name})
	}

	templates := make([]catalogTemplateOut, 0)
	for _, e := range config.Catalog() {
		// 对每个账号判断「本模板是否已启用」：优先匹配 catalog 生成的标准 name，
		// 再用同 quota_group+path 兼容历史自定义接口名。
		enabled := make([]string, 0)
		for _, a := range cfg.Accounts {
			if catalogAccountEnabled(cfg, e, a.ID) {
				enabled = append(enabled, a.ID)
			}
		}
		templates = append(templates, catalogTemplateOut{
			Key:             e.Key,
			Display:         e.Display,
			Summary:         e.Summary,
			Path:            e.Path,
			Method:          e.Method,
			Table:           e.Table,
			DefaultCron:     e.DefaultCron,
			ParamShape:      paramShapeText(e),
			IterateByStore:  e.IterateByStore,
			EnabledAccounts: enabled,
		})
	}

	okJSON(w, catalogListOut{Templates: templates, Accounts: accounts})
}

// catalogEnableIn 是 POST /api/catalog/enable 的请求体。
type catalogEnableIn struct {
	Key     string `json:"key"`
	Account string `json:"account"`
}

// apiCatalogEnable 用「模板 key + 账号」生成一个 Endpoint 并追加进配置。
// 校验链与 apiCreateEndpoint 完全一致（账号存在 / 名称不重复 / 限流键不冲突 / 目标表已建），
// 保证清单启用与手工新增走同一道安全门；成功即结构性变更，need_restart:true。
func (s *Server) apiCatalogEnable(w http.ResponseWriter, r *http.Request) {
	var in catalogEnableIn
	if err := decodeJSON(r, &in); err != nil {
		errJSON(w, http.StatusBadRequest, "请求体格式错误: "+err.Error())
		return
	}
	if in.Key == "" || in.Account == "" {
		errJSON(w, http.StatusBadRequest, "key/account 不能为空")
		return
	}

	entry, err := config.FindCatalogEntry(in.Key)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err.Error())
		return
	}

	old := s.store.Current()
	snap := s.store.Snapshot()
	if snap.FindAccount(in.Account) == nil {
		errJSON(w, http.StatusBadRequest, "account 不存在: "+in.Account)
		return
	}

	ep := entry.ToEndpoint(in.Account)
	for _, e := range snap.Endpoints {
		if e.Name == ep.Name {
			errJSON(w, http.StatusBadRequest, "该账号已启用过此接口: "+entry.Display)
			return
		}
	}
	// 限流键冲突（同 quota_group+path 已被别的接口占用）：fail-loud 拦住，勿共享限流桶。
	if owner, dup := snap.ConflictingLimiterKey(ep); dup {
		errJSON(w, http.StatusBadRequest, fmt.Sprintf(
			"限流键 (quota_group=%s, path=%s) 已被接口 %s 占用；该账号可能已用别的接口拉这个 path",
			snap.QuotaGroupOf(in.Account), ep.Path, owner))
		return
	}
	// fail-loud：目标表必须已建好（宪法 §5）。清单模板的表都由 migrations 预建，
	// 正常不会走到这里；万一漏建，早拦比重启后 FATAL 好。
	if _, err := db.GetTableColumns(s.dbx, ep.Table); err != nil {
		errJSON(w, http.StatusBadRequest, "目标表 "+ep.Table+" 未建表或不可读，请先执行 migrations 建表: "+err.Error())
		return
	}
	snap.Endpoints = append(snap.Endpoints, ep)

	s.applyConfigWrite(w, old, snap, "已启用接口: "+entry.Display+"（账号 "+in.Account+"）")
}
