package http

import (
	"encoding/json"
	"net/http"

	"github.com/aurora-vm/aurora/internal/app/auth"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// AuthHandler handles user registration, authentication, 2FA validation, and session rotation.
type AuthHandler struct {
	authService *auth.Service
}

// NewAuthHandler creates an authentication HTTP handler.
func NewAuthHandler(authService *auth.Service) *AuthHandler {
	return &AuthHandler{authService: authService}
}

// RegisterRoutes mounts authentication endpoints on the router.
func (h *AuthHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/auth", func(authRouter chi.Router) {
		authRouter.Post("/register", h.Register)
		authRouter.Post("/login", h.Login)
		authRouter.Post("/2fa/verify", h.VerifyTOTP)
		authRouter.Post("/refresh", h.Refresh)
		authRouter.Post("/logout", h.Logout)

		// Protected endpoints
		authRouter.Group(func(protected chi.Router) {
			protected.Use(authMiddleware)
			protected.Get("/me", h.GetMe)
		})
	})
}

// Register creates a new user account.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	var req auth.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	req.IPAddress = r.RemoteAddr
	req.UserAgent = r.UserAgent()

	user, err := h.authService.Register(r.Context(), req)
	if err != nil {
		if err == identity.ErrUsernameExists {
			RespondError(w, r, http.StatusConflict, "AUTH_USERNAME_EXISTS", "Username is already taken")
			return
		}
		if err == identity.ErrEmailExists {
			RespondError(w, r, http.StatusConflict, "AUTH_EMAIL_EXISTS", "Email address is already registered")
			return
		}
		RespondError(w, r, http.StatusBadRequest, "REGISTRATION_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, map[string]interface{}{
		"id":        user.ID,
		"username":  user.Username,
		"email":     user.Email,
		"createdAt": user.CreatedAt,
	})
}

// Login authenticates credentials and returns JWT tokens or 2FA challenge.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req auth.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	req.IPAddress = r.RemoteAddr
	req.UserAgent = r.UserAgent()

	res, err := h.authService.Login(r.Context(), req)
	if err != nil {
		if err == identity.ErrInvalidCredentials {
			RespondError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Invalid username, email or password")
			return
		}
		if err == identity.ErrAccountDisabled {
			RespondError(w, r, http.StatusForbidden, "AUTH_ACCOUNT_DISABLED", "Your account has been suspended")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "LOGIN_FAILED", "Authentication failed")
		return
	}

	RespondJSON(w, r, http.StatusOK, res)
}

type verifyTOTPRequest struct {
	ChallengeToken string `json:"challengeToken"`
	Code           string `json:"code"`
}

// VerifyTOTP completes the login flow when 2FA is required.
func (h *AuthHandler) VerifyTOTP(w http.ResponseWriter, r *http.Request) {
	var req verifyTOTPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	res, err := h.authService.VerifyTOTP(r.Context(), req.ChallengeToken, req.Code, r.RemoteAddr, r.UserAgent())
	if err != nil {
		if err == identity.ErrTOTPInvalid {
			RespondError(w, r, http.StatusUnauthorized, "AUTH_TOTP_INVALID", "Invalid or expired 2FA code")
			return
		}
		if err == identity.ErrTokenInvalid {
			RespondError(w, r, http.StatusUnauthorized, "AUTH_CHALLENGE_EXPIRED", "2FA challenge session has expired")
			return
		}
		RespondError(w, r, http.StatusBadRequest, "VERIFY_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, res)
}

type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// Refresh rotates the refresh token and issues a new access token.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	pair, err := h.authService.RefreshSession(r.Context(), req.RefreshToken, r.RemoteAddr, r.UserAgent())
	if err != nil {
		if err == identity.ErrRefreshTokenReused {
			RespondError(w, r, http.StatusUnauthorized, "AUTH_REFRESH_TOKEN_REUSED", "Refresh token reuse detected. All sessions revoked for security.")
			return
		}
		RespondError(w, r, http.StatusUnauthorized, "AUTH_REFRESH_TOKEN_INVALID", "Invalid or expired refresh token")
		return
	}

	RespondJSON(w, r, http.StatusOK, pair)
}

type logoutRequest struct {
	RefreshToken string `json:"refreshToken"`
}

// Logout revokes the session.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	if req.RefreshToken != "" {
		_ = h.authService.Logout(r.Context(), req.RefreshToken)
	}

	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "Logged out successfully"})
}

// GetMe returns current authenticated subject claims.
func (h *AuthHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Not authenticated")
		return
	}
	RespondJSON(w, r, http.StatusOK, subject)
}
