// Package worker 实现「账号+接口」粒度的同步执行单元。
//
// 本文件 limiter.go 实现 §6 的限流器：
//   - 每个接口的限流器按 key=(quota_group, path) 复用，同 key 共用一个桶。
//   - bucket=1 时强制串行：每次 Wait 保证距上次放行 >= interval_ms。
//   - bucket>1 时允许并发 bucket 个调用，按 token bucket 模型补令牌。
//
// 实现策略（宪法明确允许的简化）：
//   - bucket=1：sleep 间隔 = max(0, interval - elapsed)。
//   - bucket>1：token bucket，capacity=bucket，每 interval_ms 补 1 个令牌。
//   - 不引 golang.org/x/time/rate，纯标准库 sync/time 实现。
package worker

import (
	"context"
	"sync"
	"time"
)

const businessQuotaIntervalMs = 600

// Limiter 是一个接口限流器；由 LimiterRegistry 创建的实例还会先经过同账号总桶。
//
// 字段全部用 mu 保护：因为同一个 (quota_group, path) 的桶会被多个 worker（同分组
// 不同账号、或同账号同 path）共享，必须线程安全。
type Limiter struct {
	mu sync.Mutex

	capacity   int           // 令牌桶容量（= bucket）
	intervalMs int           // 补令牌间隔（毫秒）
	interval   time.Duration // intervalMs 的时间表示，避免每次转换

	// token bucket 状态：当前可用令牌（浮点，便于按时间线性补充），
	// 以及上次补充时刻。bucket=1 走 lastFire 分支，不走这里。
	tokens   float64
	lastFill time.Time

	// bucket=1 专用：上次放行时刻，用于强制串行间隔。
	lastFire time.Time

	// accountLimiter is set only for endpoint limiters created by a registry.
	// Directly constructed limiters remain standalone for focused callers/tests.
	accountLimiter *Limiter
}

// NewLimiter 构造一个限流器。
// bucket<=0 视为 1（防御），intervalMs<=0 视为 1ms（防御，不阻塞死）。
func NewLimiter(bucket, intervalMs int) *Limiter {
	if bucket <= 0 {
		bucket = 1
	}
	if intervalMs <= 0 {
		intervalMs = 1
	}
	return &Limiter{
		capacity:   bucket,
		intervalMs: intervalMs,
		interval:   time.Duration(intervalMs) * time.Millisecond,
		tokens:     float64(bucket), // 初始满桶，启动即可用
		lastFill:   time.Now(),
	}
}

// Wait 阻塞直到拿到一个令牌或 ctx 被取消。
// 返回 nil 表示已拿到令牌可以放行；返回 ctx.Err() 表示取消。
//
// 两种模式：
//   - bucket=1（强制串行）：算距上次放行的间隔，不足则 sleep 差额。
//   - bucket>1（token bucket）：补充令牌（按经过时间线性补），不足则 sleep 到下一个令牌补出。
func (l *Limiter) Wait(ctx context.Context) error {
	if l.accountLimiter != nil {
		if err := l.accountLimiter.Wait(ctx); err != nil {
			return err
		}
	}
	return l.waitLocal(ctx)
}

func (l *Limiter) waitLocal(ctx context.Context) error {
	// 先快速路径：尝试一次抢令牌
	if l.tryAcquire() {
		return nil
	}
	// 抢不到，进入「算等待 → select sleep/ctx.Done → 再试」循环
	for {
		wait := l.nextWait()
		if wait <= 0 {
			if l.tryAcquire() {
				return nil
			}
			continue
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
			// 醒来再抢
			if l.tryAcquire() {
				return nil
			}
		}
	}
}

// tryAcquire 尝试非阻塞拿一个令牌，成功返回 true。线程安全。
func (l *Limiter) tryAcquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// bucket=1：强制串行分支
	if l.capacity == 1 {
		if l.lastFire.IsZero() || now.Sub(l.lastFire) >= l.interval {
			l.lastFire = now
			return true
		}
		return false
	}

	// bucket>1：先按时间补令牌
	l.refillLocked(now)
	if l.tokens >= 1.0 {
		l.tokens -= 1.0
		return true
	}
	return false
}

// nextWait 返回下一个可能拿到令牌需要的等待时长。
// 用于让 Wait 在 sleep 与 ctx.Done 之间做选择。线程安全。
func (l *Limiter) nextWait() time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()

	if l.capacity == 1 {
		if l.lastFire.IsZero() {
			return 0
		}
		elapsed := now.Sub(l.lastFire)
		if elapsed >= l.interval {
			return 0
		}
		return l.interval - elapsed
	}

	l.refillLocked(now)
	if l.tokens >= 1.0 {
		return 0
	}
	// 还差 (1 - tokens) 个令牌，每 interval 补 1 个
	missing := 1.0 - l.tokens
	// 至少补一个令牌的时间（向上取整避免 busy loop）
	return time.Duration(missing * float64(l.interval)).Round(time.Millisecond)
}

// Update 热更新限流参数（配置热加载用）。在 mu 保护下原地覆盖 capacity/interval
// 数值字段，不新建 Limiter（同 key 的调用方持有的是同一个指针，必须原地改才能生效）。
// bucket<=0 / intervalMs<=0 的防御规则与 NewLimiter 一致。
//
// 本实现是计数器+时间戳模型（非 channel 令牌桶），直接覆盖数值字段即可：
// tokens/lastFill/lastFire 不用特殊处理——refillLocked 每次都会把 tokens 封顶到
// （新）capacity，旧值即便暂时超出新容量也会在下一次 tryAcquire/nextWait 时自动纠正。
//
// 只在锁内做纯字段赋值，不调用 l 的其它加锁方法，避免死锁。
func (l *Limiter) Update(bucket, intervalMs int) {
	if bucket <= 0 {
		bucket = 1
	}
	if intervalMs <= 0 {
		intervalMs = 1
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	l.capacity = bucket
	l.intervalMs = intervalMs
	l.interval = time.Duration(intervalMs) * time.Millisecond
}

// refillLocked 按经过时间线性补充令牌，封顶 capacity。调用方持锁。
func (l *Limiter) refillLocked(now time.Time) {
	if l.interval <= 0 {
		return
	}
	elapsed := now.Sub(l.lastFill)
	if elapsed <= 0 {
		return
	}
	add := float64(elapsed) / float64(l.interval)
	l.tokens += add
	if l.tokens > float64(l.capacity) {
		l.tokens = float64(l.capacity)
	}
	l.lastFill = now
}

// LimiterRegistry 按 key 复用 Limiter：同 quota_group 共用业务总桶，同
// (quota_group, path) 再共用接口桶。
type LimiterRegistry struct {
	mu             sync.Mutex
	m              map[string]*Limiter
	accountBuckets map[string]*Limiter
}

// NewLimiterRegistry 构造空注册表。
func NewLimiterRegistry() *LimiterRegistry {
	return &LimiterRegistry{m: make(map[string]*Limiter), accountBuckets: make(map[string]*Limiter)}
}

// Get 返回 key 对应的 Limiter；不存在则用 (bucket, intervalMs) 新建。
// 同 key 多次调用返回同一个指针（宪法 §6：同 key 共享桶）。
// 后续调用传入的 bucket/intervalMs 仅在首次创建时生效，已存在则忽略。
func (r *LimiterRegistry) Get(quotaGroup, path string, bucket, intervalMs int) *Limiter {
	key := quotaGroup + "|" + path
	r.mu.Lock()
	defer r.mu.Unlock()
	if l, ok := r.m[key]; ok {
		if l.accountLimiter == nil {
			l.accountLimiter = r.accountBucketLocked(quotaGroup)
		}
		return l
	}
	l := NewLimiter(bucket, intervalMs)
	l.accountLimiter = r.accountBucketLocked(quotaGroup)
	r.m[key] = l
	return l
}

// UpdateOrCreate 热更新 key=(quotaGroup, path) 对应的 Limiter 参数；不存在则新建
// （配置热加载用）。key 计算方式必须和 Get 保持一致，否则热加载后 worker 通过
// Get 拿到的桶和这里更新的不是同一个。
//
// 注意：Update 调用发生在 r.mu 已释放之后——先在 r.mu 保护下拿到/塞入指针，
// 再在锁外调用 l.Update（它自己的 l.mu），避免嵌套锁。
func (r *LimiterRegistry) UpdateOrCreate(quotaGroup, path string, bucket, intervalMs int) *Limiter {
	key := quotaGroup + "|" + path

	r.mu.Lock()
	l, ok := r.m[key]
	if !ok {
		l = NewLimiter(bucket, intervalMs)
		l.accountLimiter = r.accountBucketLocked(quotaGroup)
		r.m[key] = l
	} else if l.accountLimiter == nil {
		l.accountLimiter = r.accountBucketLocked(quotaGroup)
	}
	r.mu.Unlock()

	if ok {
		l.Update(bucket, intervalMs)
	}
	return l
}

func (r *LimiterRegistry) accountBucketLocked(quotaGroup string) *Limiter {
	if l, ok := r.accountBuckets[quotaGroup]; ok {
		return l
	}
	l := NewLimiter(1, businessQuotaIntervalMs)
	r.accountBuckets[quotaGroup] = l
	return l
}
