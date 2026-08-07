// Package worker 的 retry.go 实现「可恢复失败」判定与抓取重试。
//
// 背景（宪法 §5 fail-loud）：抓取出错时不静默兜底、不写脏数据。但网络抖动、
// 领星侧 429 限流、5xx 临时不可用属于**可恢复**故障——立即放弃会让整条同步
// 任务无谓失败。这里区分两类：
//   - 可重试：传输层错误（*url.Error，无 HTTP 状态）、HTTP 429、HTTP 5xx。
//   - 不可重试：4xx 客户端错误（参数/权限问题，重试也白搭）、业务契约错误
//     （HTTP 200 但领星返回 code!=0，属数据/契约问题，必须 fail-loud 暴露）、
//     ctx 已取消/超时（调用方主动中止，不该续命）。
//
// 判定函数 retryableFetchFailure 是纯函数，便于单测（见 retry_test.go）。
// 真正的重试循环在 fetchAllPages 里，退避期间仍走同一个 (quota_group,path) 桶，
// 避免重试风暴踩踏限流。
package worker

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"time"

	"lingxing-sync/internal/api"
)

// 抓取重试参数。故意用常量而非配置：重试是「抗抖动」而非「调优旋钮」，
// 加配置项反而违背 §3「加接口极简单」。此取舍与最高价值同语言参考仓
// QQiot/lingxing 一致——它把 retryCount/waitTime 写死进 client，不做 per-endpoint
// 覆盖，只对 429 / 限流 / token 过期码重试（见 otherlingxinggithub.md §1/§3）。
// 如需按接口调，再提到 config 层。
const (
	// maxFetchRetries 是单页抓取失败后的最大重试次数（不含首次）。
	maxFetchRetries = 3
	// fetchRetryBaseDelayMs 是指数退避的基数：第 n 次重试等待 base*2^n ms
	// （attempt 从 0 计：0→base, 1→2*base, 2→4*base）。
	fetchRetryBaseDelayMs = 500
)

// retryableFetchFailure 判断一次 client.Fetch 的失败是否值得重试。
//
// 参数：
//   - ctx：当前抓取上下文。已取消/超时（ctx.Err()!=nil）一律不重试。
//   - httpStatus：领星返回的 HTTP 状态码；传输层错误时通常为 0。
//   - err：Fetch 返回的错误（nil 表示成功，直接返回 false）。
//
// 返回 true 仅当：传输层错误（*url.Error 且无 HTTP 状态）、HTTP 429、HTTP 5xx。
func retryableFetchFailure(ctx context.Context, httpStatus int, err error) bool {
	if err == nil {
		return false
	}
	// 调用方已主动取消/超时：不续命，交由上层置 cancelled/error。
	if ctx.Err() != nil {
		return false
	}
	// HTTP 层可恢复：被限流或服务端临时故障。
	if httpStatus == http.StatusTooManyRequests || httpStatus >= 500 {
		return true
	}
	// 传输层错误：没拿到 HTTP 状态（连接重置/超时/DNS 等），表现为 *url.Error。
	if httpStatus == 0 {
		var urlErr *url.Error
		if errors.As(err, &urlErr) {
			return true
		}
	}
	// 其余（4xx 客户端错误、业务契约错误）不重试，fail-loud。
	return false
}

// backoffDelay 返回第 attempt 次重试前的退避时长（attempt 从 0 计）：base * 2^attempt。
// 不加随机抖动——单进程内同 (quota_group,path) 已由 limiter 串行/令牌桶天然错开，
// 退避只为给上游喘息，无需再抖动去同步化。
func backoffDelay(attempt int) time.Duration {
	d := time.Duration(fetchRetryBaseDelayMs) * time.Millisecond
	for i := 0; i < attempt; i++ {
		d *= 2
	}
	return d
}

// fetchPageWithRetry 抓取一页，对「可恢复失败」做指数退避重试。
//
// 宪法「各接口独立」：整个重试完全内包在本 worker 这一次翻页里，退避 sleep 只阻塞
// 自己这个 goroutine，绝不牵动别的接口。
//
// 关键约束（用户明确要求 + otherlingxinggithub.md §3）：每次尝试（含每次重试）都先过
// 同一个 (quota_group,path) limiter.Wait——退避叠加在限流之上，重试绝不绕开限流去踩
// 踏上游配额。ctx 取消/超时在退避与限流等待处都会即时返回，不续命。
//
// 返回最后一次尝试的 (result, httpStatus, apiCode, durationMs, err)。durationMs 是含
// 所有重试与退避的墙钟耗时（落 sync_task_logs 时体现真实代价）。
func (w *EndpointWorker) fetchPageWithRetry(ctx context.Context, limiter *Limiter, method, path string, params map[string]any) (*api.FetchResult, int, int, int, error) {
	start := time.Now()
	for attempt := 0; ; attempt++ {
		// 每次尝试前都要过限流器（重试也不例外）。
		if werr := limiter.Wait(ctx); werr != nil {
			return nil, 0, 0, int(time.Since(start).Milliseconds()), werr
		}

		result, httpStatus, apiCode, err := w.Client.Fetch(ctx, method, path, params)
		if err == nil {
			return result, httpStatus, apiCode, int(time.Since(start).Milliseconds()), nil
		}

		// 不可重试，或已到最大重试次数：把最后一次错误抛回调用方 fail-loud。
		if attempt >= maxFetchRetries || !retryableFetchFailure(ctx, httpStatus, err) {
			return nil, httpStatus, apiCode, int(time.Since(start).Milliseconds()), err
		}

		// 可恢复失败且仍有重试额度：退避后再来。退避期间监听 ctx，取消则立刻返回。
		delay := backoffDelay(attempt)
		log.Printf("[worker:%s] 抓取可恢复失败，第 %d/%d 次重试前退避 %s: %v (http=%d)",
			w.Endpoint.Name, attempt+1, maxFetchRetries, delay, err, httpStatus)
		select {
		case <-ctx.Done():
			return nil, httpStatus, apiCode, int(time.Since(start).Milliseconds()), ctx.Err()
		case <-time.After(delay):
		}
	}
}
