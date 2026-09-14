package reportexport

import (
	"context"
	"net/http"
	"testing"
	"time"

	"lingxing-sync/internal/api"
)

type passLimiter struct{}

func (passLimiter) Wait(context.Context) error { return nil }

type recordingSellerLimiter struct {
	waits     []string
	cooldowns []struct {
		seller string
		wait   time.Duration
	}
}

func (l *recordingSellerLimiter) WaitSeller(_ context.Context, seller string) error {
	l.waits = append(l.waits, seller)
	return nil
}

func (l *recordingSellerLimiter) CooldownSeller(seller string, wait time.Duration) {
	l.cooldowns = append(l.cooldowns, struct {
		seller string
		wait   time.Duration
	}{seller, wait})
}

func TestSellerRateLimiterBlocksOnlyCoolingSeller(t *testing.T) {
	limiter := NewSellerRateLimiter(passLimiter{})
	limiter.CooldownSeller("seller-a", 35*time.Millisecond)

	start := time.Now()
	if err := limiter.WaitSeller(context.Background(), "seller-a"); err != nil {
		t.Fatalf("seller-a wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
		t.Fatalf("seller-a returned after %s, want cooldown wait", elapsed)
	}

	start = time.Now()
	if err := limiter.WaitSeller(context.Background(), "seller-b"); err != nil {
		t.Fatalf("seller-b wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed > 15*time.Millisecond {
		t.Fatalf("seller-b was blocked by seller-a cooldown for %s", elapsed)
	}
}

func TestSellerRateLimiterExtendsExistingCooldown(t *testing.T) {
	limiter := NewSellerRateLimiter(passLimiter{})
	limiter.CooldownSeller("seller-a", 20*time.Millisecond)
	limiter.CooldownSeller("seller-a", 45*time.Millisecond)

	start := time.Now()
	if err := limiter.WaitSeller(context.Background(), "seller-a"); err != nil {
		t.Fatalf("seller-a wait: %v", err)
	}
	if elapsed := time.Since(start); elapsed < 35*time.Millisecond {
		t.Fatalf("cooldown was not extended, waited %s", elapsed)
	}
}

func TestRunnerSharesQueryCooldownWithSellerLimiter(t *testing.T) {
	calls := 0
	client := signedClientFunc(func(_ context.Context, _, _ string, _ map[string]any) ([]byte, int, int, error) {
		calls++
		if calls == 1 {
			return nil, http.StatusOK, http.StatusTooManyRequests, api.NewFetchError(http.StatusOK, http.StatusTooManyRequests,
				`api error code=429 error_details=["请在2.5秒后重试"]`, 0, true)
		}
		return []byte(`{"code":0,"data":{"progress_status":"IN_PROGRESS"}}`), http.StatusOK, 0, nil
	})
	limiter := &recordingSellerLimiter{}
	runner := Runner{Client: client, SellerLimiter: limiter, sleep: func(context.Context, time.Duration) error { return nil }}
	if _, err := runner.call(context.Background(), queryPath, map[string]any{"seller_id": "seller-a"}); err != nil {
		t.Fatalf("query call = %v", err)
	}
	if len(limiter.waits) != 2 || limiter.waits[0] != "seller-a" || limiter.waits[1] != "seller-a" {
		t.Fatalf("seller waits = %#v", limiter.waits)
	}
	if len(limiter.cooldowns) != 1 || limiter.cooldowns[0].seller != "seller-a" || limiter.cooldowns[0].wait != 2500*time.Millisecond {
		t.Fatalf("seller cooldowns = %#v", limiter.cooldowns)
	}
}
