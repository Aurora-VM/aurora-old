package ratelimit

import (
	"context"
	"time"

	domainRateLimit "github.com/aurora-vm/aurora/internal/domain/ratelimit"
)

// Predefined production rate limiting rules
var (
	RuleAuthAttempts   = domainRateLimit.Rule{Name: "auth_attempts", Limit: 10, Window: 1 * time.Minute, KeyPrefix: "auth"}
	RuleTokenRefresh   = domainRateLimit.Rule{Name: "token_refresh", Limit: 30, Window: 1 * time.Minute, KeyPrefix: "refresh"}
	RuleInstanceCreate = domainRateLimit.Rule{Name: "instance_create", Limit: 15, Window: 1 * time.Minute, KeyPrefix: "inst_create"}
	RuleInstanceDelete = domainRateLimit.Rule{Name: "instance_delete", Limit: 20, Window: 1 * time.Minute, KeyPrefix: "inst_del"}
	RuleConsoleSession = domainRateLimit.Rule{Name: "console_session", Limit: 10, Window: 1 * time.Minute, KeyPrefix: "console"}
	RuleWebhookCreate  = domainRateLimit.Rule{Name: "webhook_create", Limit: 10, Window: 1 * time.Minute, KeyPrefix: "wh_create"}
	RuleAdminMutations = domainRateLimit.Rule{Name: "admin_mutations", Limit: 60, Window: 1 * time.Minute, KeyPrefix: "admin"}
)

// Service provides a high-level API abuse protection and rate-limiting interface.
type Service struct {
	limiter domainRateLimit.Limiter
}

// NewService constructs a Rate Limiting Application Service.
func NewService(limiter domainRateLimit.Limiter) *Service {
	return &Service{limiter: limiter}
}

// CheckLimit evaluates whether an action identified by key is allowed by the given rule.
func (s *Service) CheckLimit(ctx context.Context, key string, rule domainRateLimit.Rule) (*domainRateLimit.Result, error) {
	if s.limiter == nil {
		return &domainRateLimit.Result{Allowed: true, Limit: rule.Limit, Remaining: rule.Limit}, nil
	}
	return s.limiter.Allow(ctx, key, rule)
}

// Reset clears the rate limit counter for a specific key.
func (s *Service) Reset(ctx context.Context, key string, keyPrefix string) error {
	if s.limiter == nil {
		return nil
	}
	return s.limiter.Reset(ctx, key, keyPrefix)
}
