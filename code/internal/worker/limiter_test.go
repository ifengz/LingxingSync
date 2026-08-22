package worker

import (
	"context"
	"testing"
	"time"
)

func TestLimiterRegistrySharesBusinessQuotaByGroup(t *testing.T) {
	registry := NewLimiterRegistry()
	first := registry.Get("account-a", "/one", 1, 1)
	second := registry.Get("account-a", "/two", 1, 1)
	otherAccount := registry.Get("account-b", "/one", 1, 1)

	if first.accountLimiter == nil || first.accountLimiter != second.accountLimiter {
		t.Fatal("same quota group must share one account limiter")
	}
	if first.accountLimiter == otherAccount.accountLimiter {
		t.Fatal("different quota groups must not share an account limiter")
	}
}

func TestLimiterWaitUsesAccountAndEndpointBuckets(t *testing.T) {
	registry := NewLimiterRegistry()
	first := registry.Get("account-a", "/one", 1, 1)
	second := registry.Get("account-a", "/two", 1, 1)

	if err := first.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if err := second.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed < time.Duration(businessQuotaIntervalMs-25)*time.Millisecond {
		t.Fatalf("account quota was bypassed: waited %s", elapsed)
	}
}
