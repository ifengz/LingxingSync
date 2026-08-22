package worker

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"lingxing-sync/internal/api"
)

func TestRetryableFetchFailureOnlyRetriesRecoverableFailures(t *testing.T) {
	transport := &url.Error{Op: "Get", URL: "https://example.test", Err: errors.New("connection reset")}

	tests := []struct {
		name       string
		httpStatus int
		err        error
		want       bool
	}{
		{name: "network error", err: transport, want: true},
		{name: "too many requests", httpStatus: 429, err: errors.New("rate limited"), want: true},
		{name: "server error", httpStatus: 503, err: errors.New("unavailable"), want: true},
		{name: "bad request", httpStatus: 400, err: errors.New("invalid parameters"), want: false},
		{name: "business contract error", httpStatus: 200, err: errors.New("api code=500"), want: false},
		{name: "cancelled context", err: transport, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.name == "cancelled context" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			if got := retryableFetchFailure(ctx, tt.httpStatus, tt.err); got != tt.want {
				t.Fatalf("retryableFetchFailure(status=%d, err=%v) = %v, want %v", tt.httpStatus, tt.err, got, tt.want)
			}
		})
	}
}

func TestRetryDelayClassifiesLingxingBusinessFailures(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		status    int
		code      int
		err       error
		attempt   int
		wantRetry bool
		wantDelay time.Duration
	}{
		{name: "3001008 uses rate limit schedule", code: 3001008, err: errors.New("rate limited"), attempt: 0, wantRetry: true, wantDelay: 5 * time.Second},
		{name: "429 uses rate limit schedule", status: 429, err: errors.New("rate limited"), attempt: 1, wantRetry: true, wantDelay: 15 * time.Second},
		{name: "generic 103 is temporary", code: 103, err: errors.New("temporary upstream"), attempt: 0, wantRetry: true, wantDelay: 30 * time.Second},
		{name: "performance frequent 103 is rate limited", path: "/bd/productPerformance/openApi/asinList", code: 103, err: api.NewFetchError(200, 103, "请勿频繁请求", 0, false), attempt: 0, wantRetry: true, wantDelay: 5 * time.Second},
		{name: "VC PO business 500 is bounded temporary", path: "/basicOpen/platformOrder/vcOrderPo/detail", code: 500, err: errors.New("upstream unstable"), attempt: 0, wantRetry: true, wantDelay: 10 * time.Second},
		{name: "VC PO business 500 stops after two retries", path: "/basicOpen/platformOrder/vcOrderPo/detail", code: 500, err: errors.New("upstream unstable"), attempt: 2, wantRetry: false},
		{name: "request connection business 500 is bounded temporary", path: "/erp/sc/data/order/list", code: 500, err: api.NewFetchError(200, 500, "api error code=500 msg=\"请求连接异常,请稍后再试\" path=/erp/sc/data/order/list", 0, true), attempt: 0, wantRetry: true, wantDelay: 10 * time.Second},
		{name: "permission business 500 is permanent", path: "/basicOpen/vc/report/sales/list", code: 500, err: api.NewFetchError(200, 500, "api error code=500 msg=\"获取权限店铺失败\" path=/basicOpen/vc/report/sales/list", 0, true), attempt: 0, wantRetry: false},
		{name: "unauthorized is permanent", code: 2001004, err: errors.New("unauthorized"), attempt: 0, wantRetry: false},
		{name: "ip whitelist is permanent", code: 3001002, err: errors.New("ip not permit"), attempt: 0, wantRetry: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			retry, delay := retryDecision(context.Background(), tt.path, tt.status, tt.code, tt.err, tt.attempt)
			if retry != tt.wantRetry || delay != tt.wantDelay {
				t.Fatalf("retryDecision = %v, %s; want %v, %s", retry, delay, tt.wantRetry, tt.wantDelay)
			}
		})
	}
}

func TestRetryAfterOverridesRateLimitSchedule(t *testing.T) {
	err := api.NewFetchError(429, 0, "rate", 17*time.Second, false)
	retry, delay := retryDecision(context.Background(), "", 429, 0, err, 0)
	if !retry || delay != 17*time.Second {
		t.Fatalf("retryDecision = %v, %s, want true, 17s", retry, delay)
	}
}

func TestTimeoutUsesRemoteHoldBeforeRetry(t *testing.T) {
	err := api.NewFetchError(0, 0, "timeout", 0, true)
	retry, delay := retryDecision(context.Background(), "", 0, 0, err, 0)
	if !retry || delay != remoteTokenHoldDelay {
		t.Fatalf("retryDecision = %v, %s, want true, %s", retry, delay, remoteTokenHoldDelay)
	}
}

// backoffDelay 必须是指数退避：base * 2^attempt（attempt 从 0 计）。
// 常量固定，不随接口配置变（与 QQiot/lingxing 的 client 级写死一致）。
func TestBackoffDelayIsExponential(t *testing.T) {
	base := time.Duration(fetchRetryBaseDelayMs) * time.Millisecond
	want := []time.Duration{base, 2 * base, 4 * base, 8 * base}
	for attempt, w := range want {
		if got := backoffDelay(attempt); got != w {
			t.Fatalf("backoffDelay(%d) = %v, want %v", attempt, got, w)
		}
	}
}
