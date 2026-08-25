package memory

import (
	"context"
	"fmt"
	"sync"
	"time"

	domainRateLimit "github.com/aurora-vm/aurora/internal/domain/ratelimit"
)

type memoryBucket struct {
	tokens       int
	maxTokens    int
	lastRefillAt time.Time
	window       time.Duration
}

// MemoryRateLimiter implements domainRateLimit.Limiter using an in-memory token bucket store.
type MemoryRateLimiter struct {
	mu      sync.Mutex
	buckets map[string]*memoryBucket
}

// NewMemoryRateLimiter creates an in-memory rate limiter.
func NewMemoryRateLimiter() *MemoryRateLimiter {
	return &MemoryRateLimiter{
		buckets: make(map[string]*memoryBucket),
	}
}

func (l *MemoryRateLimiter) Allow(ctx context.Context, key string, rule domainRateLimit.Rule) (*domainRateLimit.Result, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucketKey := fmt.Sprintf("%s:%s", rule.KeyPrefix, key)
	now := time.Now().UTC()

	b, exists := l.buckets[bucketKey]
	if !exists {
		b = &memoryBucket{
			tokens:       rule.Limit,
			maxTokens:    rule.Limit,
			lastRefillAt: now,
			window:       rule.Window,
		}
		l.buckets[bucketKey] = b
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(b.lastRefillAt)
	if elapsed >= b.window {
		b.tokens = b.maxTokens
		b.lastRefillAt = now
	}

	if b.tokens > 0 {
		b.tokens--
		return &domainRateLimit.Result{
			Allowed:    true,
			Limit:      rule.Limit,
			Remaining:  b.tokens,
			ResetAfter: b.window - elapsed,
		}, nil
	}

	return &domainRateLimit.Result{
		Allowed:    false,
		Limit:      rule.Limit,
		Remaining:  0,
		ResetAfter: b.window - elapsed,
	}, nil
}

func (l *MemoryRateLimiter) Reset(ctx context.Context, key string, keyPrefix string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	bucketKey := fmt.Sprintf("%s:%s", keyPrefix, key)
	delete(l.buckets, bucketKey)
	return nil
}
