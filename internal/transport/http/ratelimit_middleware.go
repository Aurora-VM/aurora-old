package http

import (
	"fmt"
	"net"
	"net/http"
	"strings"

	appRateLimit "github.com/aurora-vm/aurora/internal/app/ratelimit"
	domainRateLimit "github.com/aurora-vm/aurora/internal/domain/ratelimit"
)

// RateLimitMiddleware applies a rate-limiting rule to incoming HTTP requests.
func RateLimitMiddleware(service *appRateLimit.Service, rule domainRateLimit.Rule) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if service == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Identify client by authenticated user ID or remote IP
			key := extractClientKey(r)
			res, err := service.CheckLimit(r.Context(), key, rule)
			if err != nil {
				// On internal rate limit check error, fail-open to avoid total outage
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", res.Limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", res.Remaining))
			w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", int(res.ResetAfter.Seconds())))

			if !res.Allowed {
				retryAfterSec := int(res.ResetAfter.Seconds())
				if retryAfterSec <= 0 {
					retryAfterSec = 1
				}
				w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfterSec))
				RespondError(w, r, http.StatusTooManyRequests, "RATE_LIMIT_EXCEEDED", fmt.Sprintf("Rate limit exceeded for %s. Please retry in %d seconds.", rule.Name, retryAfterSec))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func extractClientKey(r *http.Request) string {
	sub := GetSubject(r.Context())
	if sub != nil && sub.UserID != "" {
		return fmt.Sprintf("user:%s", sub.UserID)
	}

	// Extract IP
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return fmt.Sprintf("ip:%s", strings.TrimSpace(parts[0]))
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil && host != "" {
		return fmt.Sprintf("ip:%s", host)
	}

	return fmt.Sprintf("ip:%s", r.RemoteAddr)
}
