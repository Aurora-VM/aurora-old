package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	appWebhook "github.com/aurora-vm/aurora/internal/app/webhook"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainWebhook "github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/go-chi/chi/v5"
)

type WebhookHandler struct {
	service    *appWebhook.Service
	authorizer identity.Authorizer
}

func NewWebhookHandler(service *appWebhook.Service, authorizer identity.Authorizer) *WebhookHandler {
	return &WebhookHandler{
		service:    service,
		authorizer: authorizer,
	}
}

func (h *WebhookHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/webhooks", func(r chi.Router) {
		r.Use(authMiddleware)

		r.With(RequirePermission(h.authorizer, "webhook:read")).Get("/", h.ListWebhooks)
		r.With(RequirePermission(h.authorizer, "webhook:create")).Post("/", h.CreateWebhook)
		r.With(RequirePermission(h.authorizer, "webhook:read")).Get("/{id}", h.GetWebhook)
		r.With(RequirePermission(h.authorizer, "webhook:update")).Patch("/{id}", h.UpdateWebhook)
		r.With(RequirePermission(h.authorizer, "webhook:delete")).Delete("/{id}", h.DeleteWebhook)

		r.With(RequirePermission(h.authorizer, "webhook:rotate")).Post("/{id}/rotate-secret", h.RotateSecret)
		r.With(RequirePermission(h.authorizer, "webhook:test")).Post("/{id}/test", h.TestWebhook)
		r.With(RequirePermission(h.authorizer, "webhook:read")).Get("/{id}/deliveries", h.ListDeliveries)
	})
}

func (h *WebhookHandler) ListWebhooks(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	filter := domainWebhook.WebhookFilter{
		Limit:  50,
		Offset: 0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			filter.Limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			filter.Offset = o
		}
	}
	if activeStr := r.URL.Query().Get("active"); activeStr != "" {
		act := activeStr == "true"
		filter.Active = &act
	}

	webhooks, total, err := h.service.ListEndpoints(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"webhooks": webhooks,
		"total":    total,
		"limit":    filter.Limit,
		"offset":   filter.Offset,
	})
}

func (h *WebhookHandler) CreateWebhook(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var input appWebhook.CreateWebhookInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid webhook payload")
		return
	}

	res, err := h.service.CreateEndpoint(r.Context(), sub, input)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, res)
}

func (h *WebhookHandler) GetWebhook(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	ep, err := h.service.GetEndpoint(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "webhook endpoint not found")
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"webhook": ep,
	})
}

func (h *WebhookHandler) UpdateWebhook(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var input appWebhook.UpdateWebhookInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid update payload")
		return
	}

	ep, err := h.service.UpdateEndpoint(r.Context(), sub, id, input)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"webhook": ep,
	})
}

func (h *WebhookHandler) DeleteWebhook(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.service.DeleteEndpoint(r.Context(), sub, id); err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "webhook endpoint not found")
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"deleted": true,
	})
}

func (h *WebhookHandler) RotateSecret(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	newSecret, err := h.service.RotateSecret(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "webhook endpoint not found")
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"secret":  newSecret,
		"message": "Secret rotated successfully. Please store this key securely; it will not be displayed again.",
	})
}

func (h *WebhookHandler) TestWebhook(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	delivery, err := h.service.TestEndpoint(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "DELIVERY_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"delivery": delivery,
	})
}

func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	filter := domainWebhook.DeliveryFilter{
		WebhookID: id,
		Limit:     50,
		Offset:    0,
	}

	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			filter.Limit = l
		}
	}
	if offsetStr := r.URL.Query().Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			filter.Offset = o
		}
	}
	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		st := domainWebhook.DeliveryStatus(statusStr)
		filter.Status = &st
	}

	deliveries, total, err := h.service.ListDeliveries(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"deliveries": deliveries,
		"total":      total,
		"limit":      filter.Limit,
		"offset":     filter.Offset,
	})
}
