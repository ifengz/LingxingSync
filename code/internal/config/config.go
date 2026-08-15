// Package config 是配置加载与校验的唯一入口。
//
// 它定义了整个系统共用的数据结构（Config / Account / Endpoint 等），
// 所有模块（api / worker / db / server）都引用这里的类型——这是各接口互不影响、
// 又能并行开发的前提：类型即契约。
//
// 宪法对应：doc/03-config.md（字段说明）、doc/09-endpoint-contract.md（rate 字段含义）。
package config

import (
	"encoding/hex"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// accountIDPattern 约束账号 ID 的字符集（参考 GitHub username 规则，放行下划线因本项目
// 现有 ID 用它、且这是机器标识符不是公开 handle）：只允许字母/数字/下划线/连字符，
// 首尾必须是字母或数字，总长 1–32（对齐 account_id VARCHAR(32) 列宽）。
var accountIDPattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_-]{0,30}[A-Za-z0-9])?$`)

// NormID 是账号 ID 的归一化口径：去空白 + 转小写。全项目判定「两个账号 ID 是否同一个」
// 都以它为准（大小写不敏感唯一，照搬 GitHub：Sc_us 与 sc_us 视为撞名）。DB account_id 列
// 排序规则本就是 *_ci（大小写不敏感），此口径与存储层一致。
func NormID(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// ValidAccountID 报告 s 是否符合账号 ID 的 slug 字符集规范（见 accountIDPattern）。
// 导出供 server 层建账号入口做写盘前的前置校验，与 validate() 复用同一条正则，
// 保证「前置校验」与「落盘校验」判定完全一致。
func ValidAccountID(s string) bool {
	return accountIDPattern.MatchString(s)
}

// Config 是 config.yaml 的根结构。
type Config struct {
	Server        Server           `yaml:"server"`
	Database      Database         `yaml:"database"`
	Accounts      []Account        `yaml:"accounts"`
	Endpoints     []Endpoint       `yaml:"endpoints"`
	Retention     Retention        `yaml:"retention"`
	DatasetAPI    DatasetAPIConfig `yaml:"dataset_api"`
	ReportExports []ReportExport   `yaml:"report_exports"`
}

const (
	ReportExportCustomerReturns       = "fba_customer_returns"
	ReportExportCustomerShipmentSales = "fba_customer_shipment_sales"
)

// ReportExport is a fixed formal-report schedule. Disabled entries are kept
// as examples and do not require runtime fields until enabled.
type ReportExport struct {
	Type           string   `yaml:"type"`
	Enabled        bool     `yaml:"enabled"`
	Account        string   `yaml:"account"`
	SellerID       string   `yaml:"seller_id"`
	StoreID        string   `yaml:"store_id"`
	Region         string   `yaml:"region"`
	MarketplaceIDs []string `yaml:"marketplace_ids"`
	Cron           string   `yaml:"cron"`
	WindowDays     int      `yaml:"window_days"`
}

// DatasetAPIConfig stores only the fixed listing dataset publication contract.
// Project tokens are supplied as hashes; plaintext bearer tokens never belong
// in config.yaml.
type DatasetAPIConfig struct {
	CursorSecret    string         `yaml:"cursor_secret"`
	MaxDateSpanDays int            `yaml:"max_date_span_days"`
	MaxPageSize     int            `yaml:"max_page_size"`
	FieldAllowlist  []string       `yaml:"field_allowlist"`
	Tokens          []DatasetToken `yaml:"tokens"`
}

type DatasetToken struct {
	ID            string   `yaml:"id"`
	ProjectID     string   `yaml:"project_id"`
	TokenHash     string   `yaml:"token_hash"`
	DatasetScopes []string `yaml:"dataset_scopes"`
	StoreScopes   []string `yaml:"store_scopes"`
	Fields        []string `yaml:"fields"`
	ExpiresAt     string   `yaml:"expires_at"`
	Revoked       bool     `yaml:"revoked"`
}

// Server 是 HTTP 服务配置。
type Server struct {
	Port   int    `yaml:"port"`
	Secret string `yaml:"secret"`
}

// Database 是 MySQL 连接参数。
type Database struct {
	Host           string `yaml:"host"`
	Port           int    `yaml:"port"`
	User           string `yaml:"user"`
	Password       string `yaml:"password"`
	DB             string `yaml:"db"`
	MaxOpen        int    `yaml:"max_open"`
	MaxIdle        int    `yaml:"max_idle"`
	ConnTimeoutSec int    `yaml:"conn_timeout_sec"`
}

// Account 是一个领星账号（按 app_key 分组）。
type Account struct {
	ID              string          `yaml:"id"` // 本系统内部 ID，写入每条数据的 account_id 列
	Name            string          `yaml:"name"`
	QuotaGroup      string          `yaml:"quota_group"` // 限流分组；空则等于 ID
	AppKey          string          `yaml:"app_key"`
	AppSecret       string          `yaml:"app_secret"`
	ConnectionCheck ConnectionCheck `yaml:"connection_check"`
}

// ConnectionCheck 是账号级的 OpenAPI 连通性检查与 Token 主动续租计划。
// 它不属于业务 endpoint，因此没有 path/table/rate 等 endpoint 合同字段。
type ConnectionCheck struct {
	Cron    string `yaml:"cron"`
	Enabled bool   `yaml:"enabled"`
}

const DefaultConnectionCheckCron = "*/20 * * * *"

func DefaultConnectionCheck() ConnectionCheck {
	return ConnectionCheck{Cron: DefaultConnectionCheckCron, Enabled: true}
}

// QuotaGroup 返回生效的限流分组：未填则用 ID。
// 宪法 §6/09：运行时限流器 key = (quota_group, path)，不是 endpoint 名。
func (a Account) QuotaGroupOrID() string {
	if a.QuotaGroup != "" {
		return a.QuotaGroup
	}
	return a.ID
}

// WindowStartFieldOrDefault 返回窗口起始参数名，未填则 "start_date"。
// 见 Endpoint.WindowStartField 的注释：领星驼峰/蛇形两派并存，名字错就 400。
func (e Endpoint) WindowStartFieldOrDefault() string {
	if e.WindowStartField != "" {
		return e.WindowStartField
	}
	return "start_date"
}

// WindowEndFieldOrDefault 返回窗口结束参数名，未填则 "end_date"。
func (e Endpoint) WindowEndFieldOrDefault() string {
	if e.WindowEndField != "" {
		return e.WindowEndField
	}
	return "end_date"
}

// DateRangeCapable reports whether this endpoint has a verified range contract.
// Snapshot endpoints and single-date endpoints must not receive start/end parameters.
func (e Endpoint) DateRangeCapable() bool {
	return e.DateField == "" && (e.WindowDays > 0 || e.WindowStartField != "" || e.WindowEndField != "")
}

// Rate 是单个接口的限流档案，从领星文档 Rate Limit 区块原样抄写。
type Rate struct {
	Bucket          int    `yaml:"bucket"`            // 令牌桶容量；=1 时强制串行
	IntervalMs      int    `yaml:"interval_ms"`       // 单店铺调用最小间隔
	MultiIntervalMs int    `yaml:"multi_interval_ms"` // 多店铺调用最小间隔（无多店铺场景填 0）
	Dimension       string `yaml:"dimension"`         // 限流维度，通常 "account+path"
}

// Endpoint 是一个「账号+接口」的同步任务定义。
type Endpoint struct {
	Name            string         `yaml:"name"`             // 全局唯一任务标识
	Display         string         `yaml:"display"`          // UI 展示名
	Account         string         `yaml:"account"`          // 必须匹配某个 Account.ID
	Path            string         `yaml:"path"`             // 领星 API Path（原样抄）
	Method          string         `yaml:"method"`           // GET / POST
	Table           string         `yaml:"table"`            // 目标数据表名
	RecordIDFields  []string       `yaml:"record_id_fields"` // 唯一键字段数组（复合主键用多元素）
	ResponseShape   string         `yaml:"response_shape"`   // list（默认）或 object（data 单对象）
	Rate            Rate           `yaml:"rate"`
	Cron            string         `yaml:"cron"`
	Enabled         bool           `yaml:"enabled"`
	WindowDays      int            `yaml:"window_days"`       // 0=全量；>0=窗口天数；single_day_window 时逐日补偿最近 N 天
	SingleDayWindow bool           `yaml:"single_day_window"` // true=将配置窗口或手工范围拆成逐日的起止同日请求
	RowDateField    string         `yaml:"row_date_field"`    // 从实际发送的窗口起始参数注入 raw row，不发送给上游
	ExtraParams     map[string]any `yaml:"extra_params"`
	// RequestHeaders 是接口协议要求的固定非敏感请求头（如 X-API-VERSION: "2"）。
	// 公共认证仍由 Client 统一签名，禁止在这里放 Authorization/Cookie。
	RequestHeaders map[string]string `yaml:"headers"`

	// 窗口参数名（WindowDays>0 时注入的那两个参数叫什么）。
	// 领星各接口对日期参数的命名不统一，实测两派并存：
	//   - 蛇形 start_date/end_date：/basicOpen/platformOrder/vcOrder/pageList
	//   - 驼峰 startDate/endDate：  /basicOpen/vc/report/{sales,realtimeSales,traffic,inventory}/list
	// 名字对不上，领星一律回 code=400 "参数有误"。因此做成可配置，而不是在 worker 里
	// 按 path 写 if——那会违反 CLAUDE.md §1.3「加接口零代码改动」。
	// 空则默认 start_date/end_date（向后兼容既有配置，不必逐条回填）。
	WindowStartField string `yaml:"window_start_field"` // 窗口起始参数名，空=start_date
	WindowEndField   string `yaml:"window_end_field"`   // 窗口结束参数名，空=end_date

	// 单日期参数（报表类接口常见，如销量统计的 event_date）：与 WindowDays 的「范围」互补。
	// DateField 非空时，baseParams 注入 DateField = 今天往前 DateOffsetDays 天的单个 YYYY-MM-DD。
	// 通用机制，所有单日期接口共用，不给单个接口写死代码。DateField 空则整套机制不生效。
	DateField      string `yaml:"date_field"`       // 单日期参数名，如 "event_date"；空=不注入
	DateOffsetDays int    `yaml:"date_offset_days"` // 往前几天：0=今天，1=昨天（报表通常取昨天，T+1 才齐）

	// 多店铺迭代（宪法 §10）
	IsStoreSource  bool     `yaml:"is_store_source"`  // true=店铺来源接口，启动优先同步
	IterateByStore bool     `yaml:"iterate_by_store"` // true=对每个 sid 跑一次
	StoreParamName string   `yaml:"store_param_name"` // 迭代时注入的参数名，默认 sid
	StoreSids      []string `yaml:"store_sids"`       // 店铺白名单：空=同步该账号全部 sid；非空=只同步列出的 sid（仅 iterate_by_store 生效）
	// StoreType 限定 iterate_by_store 只迭代该类型店铺（"SC"/"VC"，对齐 ls_stores.store_type）。
	// 空=不过滤（迭代账号全部店铺，向后兼容）。SC 接口喂 VC 店铺 sid（或反之）会拉到错数据，
	// 故 SC 迭代接口应填 "SC"、VC 迭代接口填 "VC"。
	StoreType string `yaml:"store_type"`
	// 广告账号迭代：从 ls_ad_accounts 的有效账号读取 sid + profile_id。
	// 与 IterateByStore 互斥；AdAccountType 必须明确广告域，避免混用。
	IterateByAdAccount bool   `yaml:"iterate_by_ad_account"`
	AdAccountType      string `yaml:"ad_account_type"`
	// IterateByVCOrders reads recent local_po_number + vc_store_id candidates from
	// the same account's ls_vc_orders rows. The detail request itself is not paged.
	IterateByVCOrders bool `yaml:"iterate_by_vc_orders"`

	// ---- 行整形（落库前把领星返回的行"摆正"，两个机制都是通用的、配置驱动的）----
	//
	// 背景：通用 Upsert 的前提是「一行的每个字段都在顶层，列名 = 字段名」。领星有少数
	// 接口不满足这个前提，最典型的是产品表现 /bd/productPerformance/openApi/asinList：
	//   - 唯一键 asin 不在顶层，埋在 asins[0].asin 里（顶层 138 个字段全是指标）
	//   - 店铺号 sid 压根不返回——它是请求参数，领星认为调用方自己知道
	// 结果是通用 Upsert 认不出行身份，主键列写 NULL，整批 fail。
	// polabel2 的 sc-performance.ts 也遇到同一件事，它的解法是给这个接口单写代码
	// （asin 取不到就退到 asins[0].asin，sid 用 request.storeId 补）。本项目不能那么做——
	// 会违反 CLAUDE.md §1.3「加接口零代码改动」。故抽成下面两个配置项，任何「身份埋在
	// 嵌套里」或「身份只在请求参数里」的接口都能复用，不必改一行 Go 代码。

	// FieldPaths 把嵌套字段提升到顶层：键 = 目标列名，值 = 在返回行里的取值路径。
	// 路径语法：点号进对象，[n] 进数组，例如 "asins[0].asin"、"small_cate_rank.rank"。
	// 仅在目标列缺失或为空时才写入（领星哪天把字段提到顶层，以顶层实际值为准，不覆盖）。
	// 路径取不到值就跳过该列，不报错——因为「这行确实没有」和「配置写错了」在这里
	// 无法区分，真正的 fail-loud 交给主键列 NOT NULL 约束（写不进去必然报错）。
	FieldPaths map[string]string `yaml:"field_paths"`

	// InjectParams 把请求参数回填进每一行：列出参数名即可（如 ["sid"]）。
	// 用于领星"不回显请求参数"的接口——sid 是我们迭代时传进去的，行里没有它就
	// 没法区分这行属于哪个店铺。同样只在该列缺失或为空时写入：领星若回显了自己的
	// 值，以领星的为准（若两者不一致，说明接口行为变了，应当被主键冲突暴露出来）。
	InjectParams []string `yaml:"inject_params"`

	// ForceInjectParams 强制以请求参数覆盖响应同名字段，仅用于已确认的大整数 ID 精度问题。
	// 普通接口继续使用 InjectParams 的只填空语义。
	ForceInjectParams []string `yaml:"force_inject_params"`

	// 探测模式（临时）：true 时不要求目标表存在，worker 跳过建表断言与 Upsert，
	// 仅把领星返回的原始 JSON 存进 sync_task_logs.error_raw，用于摸清真实字段名后再正式建表。
	// 去掉 probe（或 false）即恢复正式同步。非生产运行态，仅在接入新接口时使用。
	Probe bool `yaml:"probe"`
}

// Retention 是日志留存策略。
type Retention struct {
	TaskLogsDays int    `yaml:"task_logs_days"`
	TasksDays    int    `yaml:"tasks_days"`
	CleanupCron  string `yaml:"cleanup_cron"`
}

// Load 从 path 读取并解析 YAML，再做语义校验。
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读配置 %s: %w", path, err)
	}
	c := Config{Retention: Retention{
		TaskLogsDays: 90,
		TasksDays:    365,
		CleanupCron:  "0 3 * * *",
	}}
	if err := yaml.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("解析 YAML: %w", err)
	}
	// 默认值
	if c.Server.Port == 0 {
		c.Server.Port = 7799 // 宪法：端口 7799 固定，不漂移
	}
	if c.Database.Port == 0 {
		c.Database.Port = 3306
	}
	if c.Database.MaxOpen == 0 {
		c.Database.MaxOpen = 20
	}
	if c.Database.MaxIdle == 0 {
		c.Database.MaxIdle = 5
	}
	if c.Database.ConnTimeoutSec == 0 {
		c.Database.ConnTimeoutSec = 10
	}
	for i := range c.Accounts {
		if c.Accounts[i].ConnectionCheck.Cron == "" {
			c.Accounts[i].ConnectionCheck = DefaultConnectionCheck()
		}
	}
	if c.Retention.TaskLogsDays <= 0 {
		return nil, fmt.Errorf("retention.task_logs_days 必须 > 0")
	}
	if c.Retention.TasksDays <= 0 {
		return nil, fmt.Errorf("retention.tasks_days 必须 > 0")
	}
	if c.Retention.CleanupCron == "" {
		c.Retention.CleanupCron = "0 3 * * *"
	}
	if err := validateDatasetAPI(c.DatasetAPI); err != nil {
		return nil, err
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func validateDatasetAPI(cfg DatasetAPIConfig) error {
	if len(cfg.Tokens) == 0 && len(cfg.FieldAllowlist) == 0 && cfg.CursorSecret == "" {
		return nil
	}
	if len(cfg.CursorSecret) < 16 {
		return fmt.Errorf("dataset_api.cursor_secret 至少 16 字节")
	}
	if cfg.MaxDateSpanDays < 0 || cfg.MaxPageSize < 0 {
		return fmt.Errorf("dataset_api limits 不能为负数")
	}
	available := make(map[string]struct{}, len(cfg.FieldAllowlist))
	for _, field := range cfg.FieldAllowlist {
		if field == "" || strings.ContainsAny(field, " .(),=;'") {
			return fmt.Errorf("dataset_api.field_allowlist 包含非法字段 %q", field)
		}
		if _, exists := available[field]; exists {
			return fmt.Errorf("dataset_api.field_allowlist 字段重复: %s", field)
		}
		available[field] = struct{}{}
	}
	seenTokens := make(map[string]struct{}, len(cfg.Tokens))
	for _, token := range cfg.Tokens {
		if token.ID == "" || len(token.TokenHash) != 64 {
			return fmt.Errorf("dataset_api token %q 缺 id/token_hash", token.ID)
		}
		if _, err := hex.DecodeString(token.TokenHash); err != nil {
			return fmt.Errorf("dataset_api token %q token_hash 非 SHA-256", token.ID)
		}
		if token.TokenHash != strings.ToLower(token.TokenHash) {
			return fmt.Errorf("dataset_api token %q token_hash 必须为小写 SHA-256", token.ID)
		}
		if _, exists := seenTokens[token.ID]; exists {
			return fmt.Errorf("dataset_api token id 重复: %s", token.ID)
		}
		seenTokens[token.ID] = struct{}{}
		projectID := token.ProjectID
		if projectID == "" {
			projectID = token.ID
		}
		if strings.TrimSpace(projectID) == "" {
			return fmt.Errorf("dataset_api token %q 缺 project_id", token.ID)
		}
		if len(token.Fields) == 0 {
			return fmt.Errorf("dataset_api token %q 未配置 fields", token.ID)
		}
		seenFields := make(map[string]struct{}, len(token.Fields))
		for _, field := range token.Fields {
			if _, ok := available[field]; !ok {
				return fmt.Errorf("dataset_api token %q field %q 不在 allowlist", token.ID, field)
			}
			if _, exists := seenFields[field]; exists {
				return fmt.Errorf("dataset_api token %q field 重复: %s", token.ID, field)
			}
			seenFields[field] = struct{}{}
		}
		if token.ExpiresAt != "" {
			if _, err := time.Parse(time.RFC3339, token.ExpiresAt); err != nil {
				return fmt.Errorf("dataset_api token %q expires_at 必须为 RFC3339", token.ID)
			}
		}
	}
	return nil
}

// ResponseShapeOrDefault 返回 endpoint 的响应形态，空值保持既有分页列表行为。
func (e Endpoint) ResponseShapeOrDefault() string {
	if strings.TrimSpace(e.ResponseShape) == "" {
		return "list"
	}
	return strings.ToLower(strings.TrimSpace(e.ResponseShape))
}

// validate 做启动断言式校验，fail-loud：缺字段直接 error，不静默兜底。
func (c *Config) validate() error {
	if c.Database.Host == "" || c.Database.User == "" || c.Database.DB == "" {
		return fmt.Errorf("database.host/user/db 不能为空")
	}
	if len(c.Accounts) == 0 {
		return fmt.Errorf("至少要配一个 account")
	}
	// account ID：字符集受 slug 约束 + 大小写不敏感全局唯一（NormID 归一后查重）。
	// seen 的 key 是归一化 ID，值是首次出现的原始 ID（用于报错时指明和谁撞了）。
	seen := make(map[string]string, len(c.Accounts))
	for i, a := range c.Accounts {
		if a.ID == "" {
			return fmt.Errorf("accounts[%d].id 不能为空", i)
		}
		if !accountIDPattern.MatchString(a.ID) {
			return fmt.Errorf("account id %q 非法：只允许字母/数字/下划线/连字符，首尾为字母或数字，长度 1–32", a.ID)
		}
		norm := NormID(a.ID)
		if first, dup := seen[norm]; dup {
			return fmt.Errorf("account id 撞名（大小写不敏感）: %q 与 %q 归一化后同为 %q", a.ID, first, norm)
		}
		seen[norm] = a.ID
		if a.AppKey == "" || a.AppSecret == "" {
			return fmt.Errorf("account %s 缺 app_key/app_secret", a.ID)
		}
		if a.ConnectionCheck.Enabled || a.ConnectionCheck.Cron != "" {
			if _, err := cron.ParseStandard(a.ConnectionCheck.Cron); err != nil {
				return fmt.Errorf("account %s 的 connection_check.cron=%q 非法: %w", a.ID, a.ConnectionCheck.Cron, err)
			}
		}
	}
	reportScopes := make(map[string]struct{}, len(c.ReportExports))
	for i, report := range c.ReportExports {
		if report.Type != ReportExportCustomerReturns && report.Type != ReportExportCustomerShipmentSales {
			return fmt.Errorf("report_exports[%d].type=%q 非法", i, report.Type)
		}
		scope := report.Type + "\x00" + NormID(report.Account) + "\x00" + report.StoreID
		if _, exists := reportScopes[scope]; exists {
			return fmt.Errorf("report_exports[%d] 的 type+account+store_id 重复", i)
		}
		reportScopes[scope] = struct{}{}
		if !report.Enabled {
			continue
		}
		if _, ok := seen[NormID(report.Account)]; !ok {
			return fmt.Errorf("report_exports[%d] 的 account=%q 在 accounts 里找不到", i, report.Account)
		}
		if !validReportIdentifier(report.SellerID, 64) || !validReportIdentifier(report.StoreID, 64) {
			return fmt.Errorf("report_exports[%d] 缺 seller_id/store_id", i)
		}
		if report.Region != "na" && report.Region != "eu" && report.Region != "fe" {
			return fmt.Errorf("report_exports[%d].region 必须为 na/eu/fe", i)
		}
		if len(report.MarketplaceIDs) == 0 {
			return fmt.Errorf("report_exports[%d] 缺 marketplace_ids", i)
		}
		marketplaces := make(map[string]struct{}, len(report.MarketplaceIDs))
		for _, marketplaceID := range report.MarketplaceIDs {
			if !validReportIdentifier(marketplaceID, 64) {
				return fmt.Errorf("report_exports[%d].marketplace_ids 包含空值", i)
			}
			if _, exists := marketplaces[marketplaceID]; exists {
				return fmt.Errorf("report_exports[%d].marketplace_ids 包含重复值 %q", i, marketplaceID)
			}
			marketplaces[marketplaceID] = struct{}{}
		}
		if report.WindowDays < 1 || report.WindowDays > 31 {
			return fmt.Errorf("report_exports[%d].window_days 必须为 1..31", i)
		}
		if _, err := cron.ParseStandard(report.Cron); err != nil {
			return fmt.Errorf("report_exports[%d].cron=%q 非法: %w", i, report.Cron, err)
		}
	}
	// endpoint：account 必须存在；name 全局唯一；必填字段齐全
	endpointNames := make(map[string]bool, len(c.Endpoints))
	// 默认禁止两个接口共享 (quota_group, path)。唯一例外是同账号、同请求方法、同限流档案、
	// 不同原始表，且固定 extra_params 恰好只有一个值不同的明确变体；它们必须共用上游配额，
	// 但各自仍是一条独立 Worker/原始表线路。
	limiterKeyOwners := make(map[string][]Endpoint, len(c.Endpoints))
	for i, e := range c.Endpoints {
		if e.Name == "" {
			return fmt.Errorf("endpoints[%d].name 不能为空", i)
		}
		if endpointNames[e.Name] {
			return fmt.Errorf("endpoint name 重复: %s", e.Name)
		}
		endpointNames[e.Name] = true
		if _, ok := seen[NormID(e.Account)]; !ok {
			return fmt.Errorf("endpoint %s 的 account=%q 在 accounts 里找不到", e.Name, e.Account)
		}
		if e.Path == "" || e.Method == "" || e.Table == "" {
			return fmt.Errorf("endpoint %s 缺 path/method/table", e.Name)
		}
		// 探测模式放松：record_id_fields 可空（表还没建，主键未定）。
		if !e.Probe && len(e.RecordIDFields) == 0 {
			return fmt.Errorf("endpoint %s 缺 record_id_fields（或改用 probe:true 探测字段）", e.Name)
		}
		if e.Rate.Bucket <= 0 || e.Rate.IntervalMs <= 0 || e.Rate.Dimension == "" {
			return fmt.Errorf("endpoint %s 的 rate.bucket/interval_ms/dimension 必填且 >0", e.Name)
		}
		responseShape := strings.ToLower(strings.TrimSpace(e.ResponseShape))
		if responseShape != "" && responseShape != "list" && responseShape != "object" {
			return fmt.Errorf("endpoint %s 的 response_shape=%q 非法：只能是 list / object", e.Name, e.ResponseShape)
		}
		if e.StoreType != "" && e.StoreType != "SC" && e.StoreType != "VC" {
			return fmt.Errorf("endpoint %s 的 store_type=%q 非法：只能是 SC / VC / 空", e.Name, e.StoreType)
		}
		if e.IterateByStore && e.IterateByAdAccount {
			return fmt.Errorf("endpoint %s 不能同时启用 iterate_by_store 与 iterate_by_ad_account", e.Name)
		}
		if e.SingleDayWindow && (e.WindowDays <= 0 || e.DateField != "") {
			return fmt.Errorf("endpoint %s 的 single_day_window 必须配置 window_days > 0 且不能同时配置 date_field", e.Name)
		}
		if e.RowDateField != "" && !e.SingleDayWindow {
			return fmt.Errorf("endpoint %s 的 row_date_field 必须与 single_day_window 一起配置", e.Name)
		}
		if e.RowDateField != "" {
			if _, exists := e.ExtraParams[e.RowDateField]; exists {
				return fmt.Errorf("endpoint %s 的 row_date_field=%q 不能出现在 extra_params", e.Name, e.RowDateField)
			}
			if e.RowDateField == e.WindowStartFieldOrDefault() || e.RowDateField == e.WindowEndFieldOrDefault() {
				return fmt.Errorf("endpoint %s 的 row_date_field=%q 不能与窗口起止参数同名", e.Name, e.RowDateField)
			}
		}
		if e.IterateByVCOrders && (e.IterateByStore || e.IterateByAdAccount) {
			return fmt.Errorf("endpoint %s 不能同时启用 iterate_by_vc_orders 与其他迭代模式", e.Name)
		}
		if e.IterateByVCOrders {
			if !strings.EqualFold(e.Method, "POST") {
				return fmt.Errorf("endpoint %s 的 iterate_by_vc_orders 必须使用 POST", e.Name)
			}
			if responseShape != "object" {
				return fmt.Errorf("endpoint %s 的 iterate_by_vc_orders 必须使用 response_shape=object", e.Name)
			}
			if e.WindowDays <= 0 {
				return fmt.Errorf("endpoint %s 的 iterate_by_vc_orders 必须配置 window_days > 0", e.Name)
			}
			if len(e.ExtraParams) > 0 || e.DateField != "" {
				return fmt.Errorf("endpoint %s 的 iterate_by_vc_orders 请求不得配置 extra_params/date_field", e.Name)
			}
			for _, required := range []string{"vc_store_id", "local_po_number"} {
				if !slices.Contains(e.ForceInjectParams, required) {
					return fmt.Errorf("endpoint %s 的 iterate_by_vc_orders 必须 force_inject_params=%q", e.Name, required)
				}
			}
		}
		if e.IterateByAdAccount && e.AdAccountType != "seller" {
			return fmt.Errorf("endpoint %s 的 ad_account_type=%q 非法：当前仅支持已验证的 seller", e.Name, e.AdAccountType)
		}
		for _, name := range e.ForceInjectParams {
			if strings.TrimSpace(name) == "" {
				return fmt.Errorf("endpoint %s 的 force_inject_params 不能包含空参数名", e.Name)
			}
		}
		for name, value := range e.RequestHeaders {
			normalized := strings.ToLower(strings.TrimSpace(name))
			if normalized == "" || strings.TrimSpace(value) == "" {
				return fmt.Errorf("endpoint %s 的 headers 不能包含空名称或空值", e.Name)
			}
			if normalized == "authorization" || normalized == "cookie" || normalized == "content-type" || normalized == "accept" {
				return fmt.Errorf("endpoint %s 的 headers 不允许覆盖认证或通用协议头: %s", e.Name, name)
			}
		}
		if e.Cron == "" {
			return fmt.Errorf("endpoint %s 缺 cron", e.Name)
		}
		if _, err := cron.ParseStandard(e.Cron); err != nil {
			return fmt.Errorf("endpoint %s 的 cron=%q 非法: %w", e.Name, e.Cron, err)
		}
		key := c.limiterKey(e)
		for _, owner := range limiterKeyOwners[key] {
			if separatedFixedParamVariants(owner, e) {
				continue
			}
			return fmt.Errorf("endpoint %s 与 %s 的限流键 (quota_group=%s, path=%s) 重复；仅允许同账号、同限流档案、不同原始表且恰好一个固定参数不同的独立变体",
				e.Name, owner.Name, c.QuotaGroupOf(e.Account), e.Path)
		}
		limiterKeyOwners[key] = append(limiterKeyOwners[key], e)
	}
	return nil
}

// validReportIdentifier mirrors reportexport.Runner's request contract so an
// enabled schedule cannot pass config validation and fail only when it runs.
func validReportIdentifier(value string, maxLength int) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed != "" && trimmed == value && len(value) <= maxLength && strings.IndexFunc(value, unicode.IsSpace) < 0
}

// QuotaGroupOf 返回某 endpoint 账号生效的限流分组：账号存在则取其 QuotaGroupOrID，
// 否则退回原始 account 字段（此时上游已因 account 不存在报错，仅作兜底不 panic）。
// 导出供 server 层拼装「限流键冲突」的用户可读报错。
func (c *Config) QuotaGroupOf(accountID string) string {
	if a := c.FindAccount(accountID); a != nil {
		return a.QuotaGroupOrID()
	}
	return accountID
}

// limiterKey 组装运行时限流器 key = (quota_group, path)，与 worker.LimiterRegistry
// 内部的 key 拼法保持一致（quotaGroup + "|" + path），保证「配置校验」与「运行时共享桶」
// 判定同一件事。
func (c *Config) limiterKey(e Endpoint) string {
	return c.QuotaGroupOf(e.Account) + "|" + e.Path
}

// separatedFixedParamVariants 报告两个共享上游 path 的配置是否为明确分离的固定参数变体。
// 恰好一个固定参数值不同，避免把完全重复请求或多处漂移的配置误当成合法变体。
func separatedFixedParamVariants(a, b Endpoint) bool {
	if NormID(a.Account) != NormID(b.Account) ||
		!strings.EqualFold(a.Method, b.Method) ||
		a.Table == b.Table ||
		a.Rate != b.Rate ||
		len(a.ExtraParams) == 0 ||
		len(a.ExtraParams) != len(b.ExtraParams) {
		return false
	}

	differences := 0
	for name, value := range a.ExtraParams {
		other, ok := b.ExtraParams[name]
		if !ok {
			return false
		}
		if fmt.Sprint(value) != fmt.Sprint(other) {
			differences++
		}
	}
	return differences == 1
}

// ConflictingLimiterKey 返回 cfg 中已存在的、与 e 共享同一 (quota_group, path) 限流键的
// 接口名；无冲突返回 ("", false)。给 server 层「创建/更新接口」做前置校验，让用户在写盘前
// 就拿到干净的报错（不带 validate 的 "校验新配置:" 包装前缀）。与 validate() 复用同一套
// limiterKey 逻辑，保证「前置校验」与「落盘校验」判定完全一致。
//
// 忽略与 e 同名的接口：更新场景下 e 自身已在 cfg.Endpoints 里，不能把自己判成冲突。
func (c *Config) ConflictingLimiterKey(e Endpoint) (string, bool) {
	key := c.limiterKey(e)
	for _, other := range c.Endpoints {
		if other.Name == e.Name {
			continue
		}
		if c.limiterKey(other) == key && !separatedFixedParamVariants(other, e) {
			return other.Name, true
		}
	}
	return "", false
}

// FindAccount 按 ID 查找账号，找不到返回 nil。大小写不敏感（照搬 GitHub：URL/API
// 传 SC_US 也能命中 sc_us）。因 validate 已保证不存在仅大小写不同的账号 ID，归一化
// 匹配至多命中一个，无歧义。
func (c *Config) FindAccount(id string) *Account {
	norm := NormID(id)
	for i := range c.Accounts {
		if NormID(c.Accounts[i].ID) == norm {
			return &c.Accounts[i]
		}
	}
	return nil
}
