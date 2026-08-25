package ratelimit

import (
	"context"
	"time"
)

// Result describes the rate limit evaluation outcome for an incoming request.
type Result struct {
	Allowed    bool          `json:"allowed"`
	Limit      int           `json:"limit"`
	Remaining  int           `json:"remaining"`
	ResetAfter time.Duration `json:"resetAfter"`
}

// Rule defines quota thresholds for an endpoint or action.
type Rule struct {
	Name       string        `json:"name"`
	Limit      int           `json:"limit"`      // Max requests in window
	Window     time.Duration `json:"window"`     // Time window (e.g. 1 minute)
	KeyPrefix  string        `json:"keyPrefix"`  // Namespace (e.g. "auth_ip", "create_inst")
}

// Limiter defines the distributed rate limiter port.
type Limiter interface {
	Allow(ctx context.Context, key string, rule Rule) (*Result, error)
	Reset(ctx context.Context, key string, keyPrefix string) error
}
