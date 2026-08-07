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

	"gopkg.in/yaml.v3"
)

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
	ID         string `yaml:"id"` // 本系统内部 ID，写入每条数据的 account_id 列
	Name       string `yaml:"name"`
	QuotaGroup string `yaml:"quota_group"` // 限流分组；空则等于 ID
	AppKey     string `yaml:"app_key"`
	AppSecret  string `yaml:"app_secret"`
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
	WindowDays     int            `yaml:"window_days"` // 0=全量；>0=滚动 N 天
	ExtraParams    map[string]any `yaml:"extra_params"`

	// 多店铺迭代（宪法 §10）
	IsStoreSource  bool     `yaml:"is_store_source"`  // true=店铺来源接口，启动优先同步
	IterateByStore bool     `yaml:"iterate_by_store"` // true=对每个 sid 跑一次
	StoreParamName string   `yaml:"store_param_name"` // 迭代时注入的参数名，默认 sid
	StoreSids      []string `yaml:"store_sids"`       // 店铺白名单：空=同步该账号全部 sid；非空=只同步列出的 sid（仅 iterate_by_store 生效）
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
	// account ID 全局唯一
	seen := make(map[string]bool, len(c.Accounts))
	for i, a := range c.Accounts {
		if a.ID == "" {
			return fmt.Errorf("accounts[%d].id 不能为空", i)
		}
		if seen[a.ID] {
			return fmt.Errorf("account id 重复: %s", a.ID)
		}
		seen[a.ID] = true
		if a.AppKey == "" || a.AppSecret == "" {
			return fmt.Errorf("account %s 缺 app_key/app_secret", a.ID)
		}
	}
	// endpoint：account 必须存在；name 全局唯一；必填字段齐全
	endpointNames := make(map[string]bool, len(c.Endpoints))
	for i, e := range c.Endpoints {
		if e.Name == "" {
			return fmt.Errorf("endpoints[%d].name 不能为空", i)
		}
		if endpointNames[e.Name] {
			return fmt.Errorf("endpoint name 重复: %s", e.Name)
		}
		endpointNames[e.Name] = true
		if !seen[e.Account] {
			return fmt.Errorf("endpoint %s 的 account=%q 在 accounts 里找不到", e.Name, e.Account)
		}
		if e.Path == "" || e.Method == "" || e.Table == "" {
			return fmt.Errorf("endpoint %s 缺 path/method/table", e.Name)
		}
		if len(e.RecordIDFields) == 0 {
			return fmt.Errorf("endpoint %s 缺 record_id_fields", e.Name)
		}
		if e.Rate.Bucket <= 0 || e.Rate.IntervalMs <= 0 || e.Rate.Dimension == "" {
			return fmt.Errorf("endpoint %s 的 rate.bucket/interval_ms/dimension 必填且 >0", e.Name)
		}
		if e.Cron == "" {
			return fmt.Errorf("endpoint %s 缺 cron", e.Name)
		}
	}
	return nil
}

// FindAccount 按 ID 查找账号，找不到返回 nil。
func (c *Config) FindAccount(id string) *Account {
	for i := range c.Accounts {
		if c.Accounts[i].ID == id {
			return &c.Accounts[i]
		}
	}
	return nil
}
