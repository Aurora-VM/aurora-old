package http

import (
	"context"
	"net/http"
	"strings"

	"github.com/aurora-vm/aurora/internal/app/apikeys"
	"github.com/aurora-vm/aurora/internal/domain/identity"
)

type contextKey string

const (
	SubjectContextKey contextKey = "aurora.subject"
)

// GetSubject extracts the authenticated identity.Subject from the request context.
func GetSubject(ctx context.Context) *identity.Subject {
	if s, ok := ctx.Value(SubjectContextKey).(*identity.Subject); ok {
		return s
	}
	return nil
}

// AuthenticateMiddleware extracts and validates Bearer JWT access tokens or X-API-Key headers.
func AuthenticateMiddleware(tokenManager identity.TokenManager, apiKeyService *apikeys.Service) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check API Key header
			apiKeyHeader := r.Header.Get("X-API-Key")
			if apiKeyHeader != "" {
				subject, err := apiKeyService.AuthenticateAPIKey(r.Context(), apiKeyHeader)
				if err != nil {
					if err == identity.ErrAPIKeyRevoked {
						RespondError(w, r, http.StatusUnauthorized, "AUTH_API_KEY_REVOKED", "API key has been revoked")
						return
					}
					if err == identity.ErrAPIKeyExpired {
						RespondError(w, r, http.StatusUnauthorized, "AUTH_API_KEY_EXPIRED", "API key has expired")
						return
					}
					RespondError(w, r, http.StatusUnauthorized, "AUTH_API_KEY_INVALID", "Invalid API key")
					return
				}
				ctx := context.WithValue(r.Context(), SubjectContextKey, subject)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// 2. Check Bearer Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Missing authorization header")
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
				RespondError(w, r, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "Authorization header must be Bearer token")
				return
			}

			subject, err := tokenManager.ValidateAccessToken(parts[1])
			if err != nil {
				if err == identity.ErrTokenExpired {
					RespondError(w, r, http.StatusUnauthorized, "AUTH_TOKEN_EXPIRED", "Access token has expired")
					return
				}
				RespondError(w, r, http.StatusUnauthorized, "AUTH_TOKEN_INVALID", "Invalid access token")
				return
			}

			ctx := context.WithValue(r.Context(), SubjectContextKey, subject)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequirePermission enforces that the authenticated subject possesses the specified permission code.
func RequirePermission(authorizer identity.Authorizer, permissionCode string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			subject := GetSubject(r.Context())
			if subject == nil {
				RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
				return
			}

			if err := authorizer.Authorize(r.Context(), subject, permissionCode, nil); err != nil {
				RespondError(w, r, http.StatusForbidden, "AUTH_INSUFFICIENT_PERMISSION", "You lack permission to perform this action")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
