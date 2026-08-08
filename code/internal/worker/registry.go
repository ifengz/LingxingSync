// Package worker 的 registry.go 实现全局 worker 注册表。
//
// 职责：维护「任务名 → EndpointWorker」的全局索引，供：
//   - server 层 /api/sync/<name> 手动触发时按 name 找到 worker（§8）；
//   - server 层禁用闸门按 name 取 worker 读 Status()（手动触发挡禁用接口）；
//   - scheduler 按 endpoint.Name 取 worker 发触发信号（§3）。
//
// 宪法对应：§2（每「账号+接口」一个 worker）。
package worker

import (
	"sync"

	"lingxing-sync/internal/config"
)

// Registry 是 worker 的全局注册表。线程安全。
type Registry struct {
	mu      sync.RWMutex
	workers map[string]*EndpointWorker
}

// NewRegistry 构造空注册表。
func NewRegistry() *Registry {
	return &Registry{workers: make(map[string]*EndpointWorker)}
}

// Register 注册一个 worker，key=worker.Endpoint.Name。
// 同名重复注册：后写覆盖（启动期应避免，由 config.validate 保证 name 唯一）。
func (r *Registry) Register(w *EndpointWorker) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.workers[w.Endpoint.Name] = w
}

// Get 按 name 取 worker，不存在返回 nil。
func (r *Registry) Get(name string) *EndpointWorker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.workers[name]
}

// All 返回所有 worker（拷贝切片，调用方可安全遍历）。
func (r *Registry) All() []*EndpointWorker {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*EndpointWorker, 0, len(r.workers))
	for _, w := range r.workers {
		out = append(out, w)
	}
	return out
}

// ApplyHotReload 用新配置热更新已存在的 worker（配置热加载入口之一）。
// 只处理「按 name 能在注册表里找到对应 worker」的 endpoint：新增的 endpoint 没有
// worker 可更新（结构性变更，需要完整重启），这里直接忽略。
func (r *Registry) ApplyHotReload(cfg *config.Config) {
	for _, ep := range cfg.Endpoints {
		if w := r.Get(ep.Name); w != nil {
			w.UpdateEndpoint(ep)
		}
	}
}
