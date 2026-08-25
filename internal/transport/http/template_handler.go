package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	appTmpl "github.com/aurora-vm/aurora/internal/app/template"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainTmpl "github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/go-chi/chi/v5"
)

// TemplateHandler handles REST API endpoints for OS templates and image artifacts.
type TemplateHandler struct {
	templateService *appTmpl.Service
	authorizer      identity.Authorizer
}

// NewTemplateHandler constructs a TemplateHandler.
func NewTemplateHandler(templateService *appTmpl.Service, authorizer identity.Authorizer) *TemplateHandler {
	return &TemplateHandler{
		templateService: templateService,
		authorizer:      authorizer,
	}
}

// RegisterRoutes registers template and image management routes on Chi router.
func (h *TemplateHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Customer / Discovery Routes
	r.Route("/api/v1/templates", func(r chi.Router) {
		r.Use(authMiddleware)
		r.With(RequirePermission(h.authorizer, "template:read")).Get("/", h.ListTemplates)
		r.With(RequirePermission(h.authorizer, "template:read")).Get("/{id}", h.GetTemplate)
	})

	// Templates Admin
	r.Route("/api/v1/admin/templates", func(r chi.Router) {
		r.Use(authMiddleware)
		r.With(RequirePermission(h.authorizer, "template:create")).Post("/", h.CreateTemplate)
		r.With(RequirePermission(h.authorizer, "template:update")).Patch("/{id}", h.UpdateTemplate)
		r.With(RequirePermission(h.authorizer, "template:delete")).Delete("/{id}", h.DeleteTemplate)
	})

	// Images Admin
	r.Route("/api/v1/admin/images", func(r chi.Router) {
		r.Use(authMiddleware)
		r.With(RequirePermission(h.authorizer, "image:read")).Get("/", h.ListImages)
		r.With(RequirePermission(h.authorizer, "image:read")).Get("/{id}", h.GetImage)
		r.With(RequirePermission(h.authorizer, "image:manage")).Post("/", h.RegisterImage)
		r.With(RequirePermission(h.authorizer, "image:manage")).Post("/{id}/sync", h.SyncImage)
		r.With(RequirePermission(h.authorizer, "image:manage")).Post("/{id}/verify", h.VerifyImage)
		r.With(RequirePermission(h.authorizer, "image:manage")).Post("/{id}/retire", h.RetireImage)
	})
}

func (h *TemplateHandler) ListTemplates(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	filter := domainTmpl.TemplateFilter{
		Distribution: r.URL.Query().Get("distribution"),
		Architecture: r.URL.Query().Get("architecture"),
		InstanceType: r.URL.Query().Get("instanceType"),
		Status:       domainTmpl.TemplateStatus(r.URL.Query().Get("status")),
		Search:       r.URL.Query().Get("search"),
		Limit:        50,
		Offset:       0,
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			filter.Limit = val
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			filter.Offset = val
		}
	}

	templates, total, err := h.templateService.ListTemplates(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"templates": templates,
		"total":     total,
		"limit":     filter.Limit,
		"offset":    filter.Offset,
	})
}

func (h *TemplateHandler) GetTemplate(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	t, err := h.templateService.GetTemplate(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainTmpl.ErrTemplateNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, t)
}

func (h *TemplateHandler) CreateTemplate(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appTmpl.CreateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	t, err := h.templateService.CreateTemplate(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainTmpl.ErrInvalidTemplateSpec) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		if errors.Is(err, domainTmpl.ErrTemplateSlugExists) {
			RespondError(w, r, http.StatusConflict, "SLUG_EXISTS", "template slug already exists")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "insufficient permissions")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, t)
}

func (h *TemplateHandler) UpdateTemplate(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req appTmpl.UpdateTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	t, err := h.templateService.UpdateTemplate(r.Context(), sub, id, req)
	if err != nil {
		if errors.Is(err, domainTmpl.ErrTemplateNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		if errors.Is(err, domainTmpl.ErrTemplateSlugExists) {
			RespondError(w, r, http.StatusConflict, "SLUG_EXISTS", "template slug already exists")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, t)
}

func (h *TemplateHandler) DeleteTemplate(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.templateService.DeleteTemplate(r.Context(), sub, id); err != nil {
		if errors.Is(err, domainTmpl.ErrTemplateNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "template not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"id":      id,
		"deleted": true,
	})
}

func (h *TemplateHandler) ListImages(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	filter := domainTmpl.ImageFilter{
		TemplateID:   r.URL.Query().Get("templateId"),
		Architecture: r.URL.Query().Get("architecture"),
		InstanceType: r.URL.Query().Get("instanceType"),
		Status:       domainTmpl.ImageStatus(r.URL.Query().Get("status")),
		NodeID:       r.URL.Query().Get("nodeId"),
		Limit:        50,
		Offset:       0,
	}

	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			filter.Limit = val
		}
	}
	if o := r.URL.Query().Get("offset"); o != "" {
		if val, err := strconv.Atoi(o); err == nil && val >= 0 {
			filter.Offset = val
		}
	}

	images, total, err := h.templateService.ListImages(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"images": images,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *TemplateHandler) GetImage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	img, err := h.templateService.GetImage(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainTmpl.ErrImageArtifactNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "image artifact not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, img)
}

func (h *TemplateHandler) RegisterImage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appTmpl.RegisterImageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	img, err := h.templateService.RegisterImage(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainTmpl.ErrInvalidImageSpec) || errors.Is(err, domainTmpl.ErrInvalidFingerprint) || errors.Is(err, domainTmpl.ErrUnsupportedInstanceType) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, img)
}

func (h *TemplateHandler) SyncImage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req struct {
		NodeID string `json:"nodeId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.NodeID == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "nodeId is required")
		return
	}

	err := h.templateService.SyncImageToNode(r.Context(), sub, appTmpl.SyncImageRequest{
		ImageID: id,
		NodeID:  req.NodeID,
	})
	if err != nil {
		if errors.Is(err, domainTmpl.ErrImageArtifactNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "image artifact not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusAccepted, map[string]interface{}{
		"imageId": id,
		"nodeId":  req.NodeID,
		"status":  "syncing",
	})
}

func (h *TemplateHandler) VerifyImage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	valid, err := h.templateService.VerifyImage(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainTmpl.ErrImageArtifactNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "image artifact not found")
			return
		}
		if errors.Is(err, domainTmpl.ErrFingerprintMismatch) {
			RespondError(w, r, http.StatusUnprocessableEntity, "VERIFICATION_FAILED", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"imageId": id,
		"valid":   valid,
	})
}

func (h *TemplateHandler) RetireImage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.templateService.RetireImage(r.Context(), sub, id); err != nil {
		if errors.Is(err, domainTmpl.ErrImageArtifactNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "image artifact not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"imageId": id,
		"status":  "retired",
	})
}
