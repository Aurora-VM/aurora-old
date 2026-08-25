package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	appKeyRotation "github.com/aurora-vm/aurora/internal/app/keyrotation"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainKeyRotation "github.com/aurora-vm/aurora/internal/domain/keyrotation"
	"github.com/go-chi/chi/v5"
)

// KeyRotationHandler provides REST endpoints for cryptographic key lifecycle management.
type KeyRotationHandler struct {
	keyService *appKeyRotation.Service
	authorizer domainIdentity.Authorizer
}

func NewKeyRotationHandler(
	keyService *appKeyRotation.Service,
	authorizer domainIdentity.Authorizer,
) *KeyRotationHandler {
	return &KeyRotationHandler{
		keyService: keyService,
		authorizer: authorizer,
	}
}

func (h *KeyRotationHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/admin/keys", func(admin chi.Router) {
		admin.Use(authMiddleware)
		admin.Get("/rotations", h.ListRotations)
		admin.Post("/rotate", h.RotateKey)
		admin.Post("/{id}/revoke", h.RevokeKey)
	})
}

func (h *KeyRotationHandler) ListRotations(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin privilege required")
		return
	}

	keyType := domainKeyRotation.KeyType(r.URL.Query().Get("type"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	records, total, err := h.keyService.ListKeyRotations(r.Context(), sub, keyType, limit, offset)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"rotations": records,
		"total":     total,
		"limit":     limit,
		"offset":    offset,
	})
}

func (h *KeyRotationHandler) RotateKey(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin privilege required")
		return
	}

	var req struct {
		Type              domainKeyRotation.KeyType `json:"type"`
		Description       string                    `json:"description,omitempty"`
		GracePeriodHours  int                       `json:"gracePeriodHours,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Type == "" {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", "type is required")
		return
	}

	graceDuration := 24 * time.Hour
	if req.GracePeriodHours > 0 {
		graceDuration = time.Duration(req.GracePeriodHours) * time.Hour
	}

	record, err := h.keyService.RotateKey(r.Context(), sub, appKeyRotation.RotateKeyRequest{
		Type:                req.Type,
		Description:         req.Description,
		GracePeriodDuration: graceDuration,
	})
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "ROTATION_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, record)
}

func (h *KeyRotationHandler) RevokeKey(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin privilege required")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)

	record, err := h.keyService.RevokeKey(r.Context(), sub, id, req.Reason)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "REVOCATION_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, record)
}
