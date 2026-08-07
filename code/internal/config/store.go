// 本文件实现配置的运行期读写层：内存持有当前生效配置，提供并发安全的
// 读取快照，以及“校验 → 备份 → 原子写盘 → 切换内存”的保存流程。
//
// 定位：config.go 负责“从文件加载一次”；store.go 负责“运行时反复读/改/存”，
// 供 HTTP 管理接口（增删改 account/endpoint 等）使用。
package config

import (
	"fmt"
	"os"
	"reflect"
	"sync"

	"gopkg.in/yaml.v3"
)

// ConfigStore 是配置的运行期容器：内存持有一份当前生效的 Config，
// 所有对 cfg 的读写都经 mu 加锁，保证并发安全。
type ConfigStore struct {
	path string
	mu   sync.RWMutex
	cfg  *Config
}

// NewStore 用给定路径和初始配置构造一个 ConfigStore。
// cfg 会被深拷贝后存入，外部对传入指针的后续修改不会影响 store 内部状态。
func NewStore(path string, cfg *Config) *ConfigStore {
	return &ConfigStore{
		path: path,
		cfg:  deepCopy(cfg),
	}
}

// Current 返回当前配置的深拷贝，供只读场景使用（如渲染 UI、下发给 worker）。
func (s *ConfigStore) Current() *Config {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return deepCopy(s.cfg)
}

// Snapshot 是 Current 的别名：返回一份用于编辑的深拷贝，
// 调用方可在其上任意改动，改完后通过 Save 写回。
func (s *ConfigStore) Snapshot() *Config {
	return s.Current()
}

// Save 保存新配置：先校验，再备份旧文件，再原子写盘，最后切换内存中的当前配置。
// 任一步失败都不影响磁盘上原有的配置文件，也不影响内存中已生效的配置。
func (s *ConfigStore) Save(newCfg *Config) error {
	if err := newCfg.validate(); err != nil {
		return fmt.Errorf("校验新配置: %w", err)
	}

	// 若磁盘上已有配置文件，先备份一份 .bak（只保留最近一次，覆盖旧的）。
	if raw, err := os.ReadFile(s.path); err == nil {
		if err := os.WriteFile(s.path+".bak", raw, 0644); err != nil {
			return fmt.Errorf("备份旧配置: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("读旧配置用于备份: %w", err)
	}

	// 先写临时文件，再 rename，保证写盘过程是原子的（不会留下半截文件）。
	out, err := yaml.Marshal(newCfg)
	if err != nil {
		return fmt.Errorf("序列化新配置: %w", err)
	}
	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, out, 0644); err != nil {
		return fmt.Errorf("写临时配置文件: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		os.Remove(tmpPath) // 清理临时文件，避免残留
		return fmt.Errorf("替换配置文件: %w", err)
	}

	s.mu.Lock()
	s.cfg = deepCopy(newCfg)
	s.mu.Unlock()
	return nil
}

// Mask 返回当前配置的深拷贝，并对每个 Account.AppSecret 做掩码处理，
// 用于对外展示（如 UI、日志），绝不修改内存中真实存储的配置。
func (s *ConfigStore) Mask() *Config {
	c := s.Current()
	for i := range c.Accounts {
		c.Accounts[i].AppSecret = maskSecret(c.Accounts[i].AppSecret)
	}
	return c
}

// maskSecret 掩码规则：长度 <= 8 时整体替换为 "****"；
// 否则保留首 4 位与尾 4 位，中间替换为 "****"。
func maskSecret(secret string) string {
	if len(secret) <= 8 {
		return "****"
	}
	return secret[:4] + "****" + secret[len(secret)-4:]
}

// deepCopy 通过 yaml 序列化再反序列化实现深拷贝，是应对嵌套 slice/map
// 最简单可靠的方式。marshal 失败在理论上不会发生（内存结构体一定能序列化），
// 一旦发生则退化返回原指针，绝不 panic。
func deepCopy(c *Config) *Config {
	if c == nil {
		return nil
	}
	raw, err := yaml.Marshal(c)
	if err != nil {
		return c
	}
	var cp Config
	if err := yaml.Unmarshal(raw, &cp); err != nil {
		return c
	}
	return &cp
}

// ChangeKind 表示新旧配置之间差异的影响级别。
type ChangeKind int

const (
	ChangeNone    ChangeKind = iota // 无实质变化
	ChangeHot                       // 可热更新（不重启即可生效）
	ChangeRestart                   // 必须重启进程才能生效
)

// ClassifyChange 比较新旧配置，判断这次改动需要热更新还是重启。
// 判定顺序：先看是否触发 ChangeRestart，否则看是否触发 ChangeHot，否则 ChangeNone。
func ClassifyChange(oldCfg, newCfg *Config) ChangeKind {
	if oldCfg == nil || newCfg == nil {
		return ChangeRestart
	}

	if !reflect.DeepEqual(oldCfg.Database, newCfg.Database) {
		return ChangeRestart
	}
	if !reflect.DeepEqual(oldCfg.Server, newCfg.Server) {
		return ChangeRestart
	}

	oldAccounts := accountsByID(oldCfg.Accounts)
	newAccounts := accountsByID(newCfg.Accounts)
	if accountIDSetChanged(oldAccounts, newAccounts) {
		return ChangeRestart
	}
	for id, oldA := range oldAccounts {
		newA := newAccounts[id]
		if oldA.AppKey != newA.AppKey ||
			oldA.AppSecret != newA.AppSecret ||
			oldA.QuotaGroup != newA.QuotaGroup ||
			oldA.Name != newA.Name {
			return ChangeRestart
		}
	}
	oldEndpoints := endpointsByName(oldCfg.Endpoints)
	newEndpoints := endpointsByName(newCfg.Endpoints)
	if endpointNameSetChanged(oldEndpoints, newEndpoints) {
		return ChangeRestart
	}
	for name, oldE := range oldEndpoints {
		newE := newEndpoints[name]
		if oldE.Path != newE.Path ||
			oldE.Method != newE.Method ||
			oldE.Table != newE.Table ||
			oldE.Account != newE.Account ||
			!reflect.DeepEqual(oldE.RecordIDFields, newE.RecordIDFields) ||
			oldE.IsStoreSource != newE.IsStoreSource ||
			oldE.IterateByStore != newE.IterateByStore ||
			oldE.StoreParamName != newE.StoreParamName {
			return ChangeRestart
		}
	}
	for id, oldA := range oldAccounts {
		if !reflect.DeepEqual(oldA.ConnectionCheck, newAccounts[id].ConnectionCheck) {
			return ChangeHot
		}
	}

	for name, oldE := range oldEndpoints {
		newE := newEndpoints[name]
		if oldE.Enabled != newE.Enabled ||
			oldE.Cron != newE.Cron ||
			!reflect.DeepEqual(oldE.Rate, newE.Rate) ||
			oldE.WindowDays != newE.WindowDays ||
			!reflect.DeepEqual(oldE.ExtraParams, newE.ExtraParams) ||
			!reflect.DeepEqual(oldE.StoreSids, newE.StoreSids) {
			return ChangeHot
		}
	}

	return ChangeNone
}

// accountsByID 把账号列表按 ID 建索引，方便 ClassifyChange 做逐个比对。
func accountsByID(accounts []Account) map[string]Account {
	m := make(map[string]Account, len(accounts))
	for _, a := range accounts {
		m[a.ID] = a
	}
	return m
}

// accountIDSetChanged 判断两组账号的 ID 集合是否有增删（不比较字段内容）。
func accountIDSetChanged(oldM, newM map[string]Account) bool {
	if len(oldM) != len(newM) {
		return true
	}
	for id := range oldM {
		if _, ok := newM[id]; !ok {
			return true
		}
	}
	return false
}

// endpointsByName 把接口列表按 Name 建索引，方便 ClassifyChange 做逐个比对。
func endpointsByName(endpoints []Endpoint) map[string]Endpoint {
	m := make(map[string]Endpoint, len(endpoints))
	for _, e := range endpoints {
		m[e.Name] = e
	}
	return m
}

// endpointNameSetChanged 判断两组接口的 Name 集合是否有增删（不比较字段内容）。
func endpointNameSetChanged(oldM, newM map[string]Endpoint) bool {
	if len(oldM) != len(newM) {
		return true
	}
	for name := range oldM {
		if _, ok := newM[name]; !ok {
			return true
		}
	}
	return false
}
