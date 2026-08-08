// Package config 是配置加载与校验的唯一入口。
//
// 它定义了整个系统共用的数据结构（Config / Account / Endpoint 等），
// 所有模块（api / worker / db / server）都引用这里的类型——这是各接口互不影响、
// 又能并行开发的前提：类型即契约。
//
// 宪法对应：doc/03-config.md（字段说明）、doc/09-endpoint-contract.md（rate 字段含义）。
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

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
	Server    Server     `yaml:"server"`
	Database  Database   `yaml:"database"`
	Accounts  []Account  `yaml:"accounts"`
	Endpoints []Endpoint `yaml:"endpoints"`
	Retention Retention  `yaml:"retention"`
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

// Rate 是单个接口的限流档案，从领星文档 Rate Limit 区块原样抄写。
type Rate struct {
	Bucket          int    `yaml:"bucket"`            // 令牌桶容量；=1 时强制串行
	IntervalMs      int    `yaml:"interval_ms"`       // 单店铺调用最小间隔
	MultiIntervalMs int    `yaml:"multi_interval_ms"` // 多店铺调用最小间隔（无多店铺场景填 0）
	Dimension       string `yaml:"dimension"`         // 限流维度，通常 "account+path"
}

// Endpoint 是一个「账号+接口」的同步任务定义。
type Endpoint struct {
	Name           string         `yaml:"name"`             // 全局唯一任务标识
	Display        string         `yaml:"display"`          // UI 展示名
	Account        string         `yaml:"account"`          // 必须匹配某个 Account.ID
	Path           string         `yaml:"path"`             // 领星 API Path（原样抄）
	Method         string         `yaml:"method"`           // GET / POST
	Table          string         `yaml:"table"`            // 目标数据表名
	RecordIDFields []string       `yaml:"record_id_fields"` // 唯一键字段数组（复合主键用多元素）
	Rate           Rate           `yaml:"rate"`
	Cron           string         `yaml:"cron"`
	Enabled        bool           `yaml:"enabled"`
	WindowDays     int            `yaml:"window_days"` // 0=全量；>0=滚动 N 天（注入 start_date/end_date 范围）
	ExtraParams    map[string]any `yaml:"extra_params"`

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
	var c Config
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
	if c.Retention.TaskLogsDays == 0 {
		c.Retention.TaskLogsDays = 90
	}
	if c.Retention.TasksDays == 0 {
		c.Retention.TasksDays = 365
	}
	if c.Retention.CleanupCron == "" {
		c.Retention.CleanupCron = "0 3 * * *"
	}
	for i := range c.Accounts {
		if c.Accounts[i].ConnectionCheck.Cron == "" {
			c.Accounts[i].ConnectionCheck = DefaultConnectionCheck()
		}
	}
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
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
	}
	// endpoint：account 必须存在；name 全局唯一；必填字段齐全
	endpointNames := make(map[string]bool, len(c.Endpoints))
	// 限流键 (quota_group, path) 全局唯一：同分组同 path 的两个接口会共享同一个
	// rate.Limiter 桶，一个翻页把配额占满会拖慢另一个——正是「各接口独立」要杜绝的
	// 牵连。命中即 fail-loud，让加接口的人换 path 或换分组，而不是静默共享（CLAUDE.md §1.1）。
	limiterKeyOwner := make(map[string]string, len(c.Endpoints))
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
		if e.StoreType != "" && e.StoreType != "SC" && e.StoreType != "VC" {
			return fmt.Errorf("endpoint %s 的 store_type=%q 非法：只能是 SC / VC / 空", e.Name, e.StoreType)
		}
		if e.Cron == "" {
			return fmt.Errorf("endpoint %s 缺 cron", e.Name)
		}
		key := c.limiterKey(e)
		if owner, dup := limiterKeyOwner[key]; dup {
			return fmt.Errorf("endpoint %s 与 %s 的限流键 (quota_group=%s, path=%s) 重复；换 path 或换 quota_group，勿共享同一限流桶",
				e.Name, owner, c.QuotaGroupOf(e.Account), e.Path)
		}
		limiterKeyOwner[key] = e.Name
	}
	return nil
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
		if c.limiterKey(other) == key {
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
