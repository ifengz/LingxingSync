package worker

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"
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
