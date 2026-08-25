package ratelimit_test

import (
	"context"
	"testing"
	"time"

	appRateLimit "github.com/aurora-vm/aurora/internal/app/ratelimit"
	domainRateLimit "github.com/aurora-vm/aurora/internal/domain/ratelimit"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

func TestRateLimiter_TokenBucketEnforcement(t *testing.T) {
	limiter := memory.NewMemoryRateLimiter()
	service := appRateLimit.NewService(limiter)

	rule := domainRateLimit.Rule{
		Name:      "test_rule",
		Limit:     3,
		Window:    500 * time.Millisecond,
		KeyPrefix: "test",
	}

	ctx := context.Background()
	key := "user-100"

	// First 3 requests should be allowed
	for i := 1; i <= 3; i++ {
		res, err := service.CheckLimit(ctx, key, rule)
		if err != nil {
			t.Fatalf("unexpected rate limit check error: %v", err)
		}
		if !res.Allowed {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}

	// 4th request must be blocked
	res, err := service.CheckLimit(ctx, key, rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Allowed {
		t.Fatalf("expected request 4 to be rejected due to rate limit")
	}

	// Wait for window to expire and verify reset
	time.Sleep(550 * time.Millisecond)
	resAfter, err := service.CheckLimit(ctx, key, rule)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resAfter.Allowed {
		t.Fatalf("expected request to be allowed after window reset")
	}
}
