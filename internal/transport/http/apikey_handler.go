package http

import (
	"encoding/json"
	"net/http"

	"github.com/aurora-vm/aurora/internal/app/apikeys"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// APIKeyHandler manages scoped API key creation, listing, and revocation endpoints.
type APIKeyHandler struct {
	apiKeyService *apikeys.Service
	authorizer    identity.Authorizer
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(apiKeyService *apikeys.Service, authorizer identity.Authorizer) *APIKeyHandler {
	return &APIKeyHandler{apiKeyService: apiKeyService, authorizer: authorizer}
}

// RegisterRoutes mounts API key endpoints on the router.
func (h *APIKeyHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/api-keys", func(keyRouter chi.Router) {
		keyRouter.Use(authMiddleware)
		keyRouter.Post("/", h.Create)
		keyRouter.Get("/", h.List)
		keyRouter.Delete("/{id}", h.Revoke)
	})
}

// Create generates a new scoped API key and returns plaintext once.
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
		return
	}

	var req apikeys.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST_PAYLOAD", "Malformed JSON payload")
		return
	}

	res, err := h.apiKeyService.CreateAPIKey(r.Context(), subject.UserID, req)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "CREATE_API_KEY_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, res)
}

// List returns all API keys for the calling user.
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
		return
	}

	keys, err := h.apiKeyService.ListAPIKeys(r.Context(), subject.UserID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "LIST_API_KEYS_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, keys)
}

// Revoke revokes an API key by ID.
func (h *APIKeyHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	subject := GetSubject(r.Context())
	if subject == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "Authentication required")
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_KEY_ID", "API key ID is required")
		return
	}

	err := h.apiKeyService.RevokeAPIKey(r.Context(), subject.UserID, keyID)
	if err != nil {
		if err == identity.ErrAPIKeyNotFound {
			RespondError(w, r, http.StatusNotFound, "AUTH_API_KEY_NOT_FOUND", "API key not found")
			return
		}
		if err == identity.ErrResourceForbidden {
			RespondError(w, r, http.StatusForbidden, "AUTH_RESOURCE_FORBIDDEN", "You cannot revoke another user's API key")
			return
		}
		RespondError(w, r, http.StatusBadRequest, "REVOKE_API_KEY_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "API key revoked successfully"})
}
