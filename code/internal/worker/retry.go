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
	"net/http"
	"net/url"
)

// 抓取重试参数。故意用常量而非配置：重试是「抗抖动」而非「调优旋钮」，
// 加配置项反而违背 §3「加接口极简单」。如需按接口调，再提到 config 层。
const (
	// maxFetchRetries 是单页抓取失败后的最大重试次数（不含首次）。
	maxFetchRetries = 3
	// fetchRetryBaseDelayMs 是指数退避的基数：第 n 次重试等待 base*2^(n-1) ms。
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
