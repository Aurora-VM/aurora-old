package http

import (
	"net/http"
)

// SecurityHeadersMiddleware adds production security headers and body limit protection.
func SecurityHeadersMiddleware(maxBodyBytes int64) func(http.Handler) http.Handler {
	if maxBodyBytes <= 0 {
		maxBodyBytes = 10 * 1024 * 1024 // 10MB default
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Enforce Request Body Size Limit
			r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

			// 2. Comprehensive Production Security Headers
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("X-XSS-Protection", "1; mode=block")
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
			w.Header().Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
			w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https:; font-src 'self' data:; connect-src 'self' ws: wss:; frame-ancestors 'none';")

			next.ServeHTTP(w, r)
		})
	}
}

// WebSocketOriginMiddleware validates that incoming WebSocket requests originate from trusted origins.
func WebSocketOriginMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && len(allowedOrigins) > 0 {
				allowed := false
				for _, ao := range allowedOrigins {
					if ao == "*" || ao == origin {
						allowed = true
						break
					}
				}
				if !allowed {
					http.Error(w, "Cross-Origin WebSocket connection rejected", http.StatusForbidden)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
