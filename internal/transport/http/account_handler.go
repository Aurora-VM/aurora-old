package http

import (
	"encoding/json"
	"net/http"

	"github.com/aurora-vm/aurora/internal/app/account"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// AccountHandler manages user 2FA and profile security endpoints.
type AccountHandler struct {
	accountService *account.Service
}

// NewAccountHandler creates a new AccountHandler.
func NewAccountHandler(accountService *account.Service) *AccountHandler {
	return &AccountHandler{accountService: accountService}
}

// RegisterRoutes mounts account endpoints on the router.
func (h *AccountHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/account", func(acctRouter chi.Router) {
		acctRouter.Use(authMiddleware)
		acctRouter.Post("/2fa/setup", h.Setup2FA)
		acctRouter.Post("/2fa/enable", h.Enable2FA)
		acctRouter.Post("/2fa/disable", h.Disable2FA)
	})
}

// Setup2FA generates a new TOTP secret for the caller.
func (h *AccountHandler) Setup2FA(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
		return
	}

	result, err := h.accountService.Setup2FA(r.Context(), subject.UserID)
	if err != nil {
		if err == identity.ErrTOTPAlreadyEnabled {
			RespondError(w, r, http.StatusConflict, "AUTH_TOTP_ALREADY_ENABLED", "Two-factor authentication is already enabled")
			return
		}
		RespondError(w, r, http.StatusBadRequest, "SETUP_2FA_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, result)
}

type enable2FARequest struct {
	Secret string `json:"secret"`
	Code   string `json:"code"`
}

// Enable2FA validates code and activates encrypted 2FA.
func (h *AccountHandler) Enable2FA(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
		return
	}

	var req enable2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	err := h.accountService.Enable2FA(r.Context(), subject.UserID, req.Secret, req.Code)
	if err != nil {
		if err == identity.ErrTOTPInvalid {
			RespondError(w, r, http.StatusBadRequest, "AUTH_TOTP_INVALID", "Invalid 2FA verification code")
			return
		}
		RespondError(w, r, http.StatusBadRequest, "ENABLE_2FA_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "Two-factor authentication enabled successfully"})
}

type disable2FARequest struct {
	Password string `json:"password"`
	Code     string `json:"code"`
}

// Disable2FA validates password and code, then deactivates 2FA.
func (h *AccountHandler) Disable2FA(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
		return
	}

	var req disable2FARequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	err := h.accountService.Disable2FA(r.Context(), subject.UserID, req.Password, req.Code)
	if err != nil {
		if err == identity.ErrInvalidCredentials {
			RespondError(w, r, http.StatusUnauthorized, "AUTH_INVALID_CREDENTIALS", "Incorrect account password")
			return
		}
		if err == identity.ErrTOTPInvalid {
			RespondError(w, r, http.StatusBadRequest, "AUTH_TOTP_INVALID", "Invalid 2FA verification code")
			return
		}
		RespondError(w, r, http.StatusBadRequest, "DISABLE_2FA_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "Two-factor authentication disabled successfully"})
}
