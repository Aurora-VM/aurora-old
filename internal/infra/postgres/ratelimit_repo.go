package postgres

import (
	"context"
	"fmt"
	"time"

	domainRateLimit "github.com/aurora-vm/aurora/internal/domain/ratelimit"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RateLimiter implements domainRateLimit.Limiter using PostgreSQL distributed token buckets.
type RateLimiter struct {
	pool *pgxpool.Pool
}

// NewRateLimiter creates a new PostgreSQL distributed rate limiter.
func NewRateLimiter(pool *pgxpool.Pool) *RateLimiter {
	return &RateLimiter{pool: pool}
}

func (l *RateLimiter) Allow(ctx context.Context, key string, rule domainRateLimit.Rule) (*domainRateLimit.Result, error) {
	bucketKey := fmt.Sprintf("%s:%s", rule.KeyPrefix, key)
	now := time.Now().UTC()
	expiresAt := now.Add(rule.Window * 2)

	query := `
	INSERT INTO rate_limit_buckets (bucket_key, tokens, max_tokens, last_refill_at, expires_at)
	VALUES ($1, $2 - 1, $2, $3, $4)
	ON CONFLICT (bucket_key) DO UPDATE
	SET tokens = CASE
	        WHEN rate_limit_buckets.last_refill_at + ($5 * INTERVAL '1 second') <= $3
	        THEN EXCLUDED.max_tokens - 1
	        WHEN rate_limit_buckets.tokens > 0
	        THEN rate_limit_buckets.tokens - 1
	        ELSE 0
	    END,
	    last_refill_at = CASE
	        WHEN rate_limit_buckets.last_refill_at + ($5 * INTERVAL '1 second') <= $3
	        THEN $3
	        ELSE rate_limit_buckets.last_refill_at
	    END,
	    expires_at = $4
	RETURNING tokens, last_refill_at;
	`

	var currentTokens int
	var lastRefill time.Time
	err := l.pool.QueryRow(ctx, query, bucketKey, rule.Limit, now, expiresAt, rule.Window.Seconds()).Scan(&currentTokens, &lastRefill)
	if err != nil {
		return nil, err
	}

	elapsed := now.Sub(lastRefill)
	resetAfter := rule.Window - elapsed
	if resetAfter < 0 {
		resetAfter = 0
	}

	if currentTokens >= 0 {
		return &domainRateLimit.Result{
			Allowed:    true,
			Limit:      rule.Limit,
			Remaining:  currentTokens,
			ResetAfter: resetAfter,
		}, nil
	}

	return &domainRateLimit.Result{
		Allowed:    false,
		Limit:      rule.Limit,
		Remaining:  0,
		ResetAfter: resetAfter,
	}, nil
}

func (l *RateLimiter) Reset(ctx context.Context, key string, keyPrefix string) error {
	bucketKey := fmt.Sprintf("%s:%s", keyPrefix, key)
	query := `DELETE FROM rate_limit_buckets WHERE bucket_key = $1;`
	_, err := l.pool.Exec(ctx, query, bucketKey)
	return err
}
