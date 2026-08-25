package http

import (
	"net/http"
	"strconv"

	"github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/go-chi/chi/v5"
)

type EventHandler struct {
	eventRepo    events.Repository
	deliveryRepo webhook.DeliveryRepository
	authorizer   identity.Authorizer
}

func NewEventHandler(eventRepo events.Repository, deliveryRepo webhook.DeliveryRepository, authorizer identity.Authorizer) *EventHandler {
	return &EventHandler{
		eventRepo:    eventRepo,
		deliveryRepo: deliveryRepo,
		authorizer:   authorizer,
	}
}

func (h *EventHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/admin/events", func(r chi.Router) {
		r.Use(authMiddleware)
		r.With(RequirePermission(h.authorizer, "audit:read")).Get("/", h.AdminListEvents)
	})

	r.Route("/api/v1/admin/webhooks/deliveries", func(r chi.Router) {
		r.Use(authMiddleware)
		r.With(RequirePermission(h.authorizer, "audit:read")).Get("/", h.AdminListDeliveries)
	})
}

func (h *EventHandler) AdminListEvents(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	if err := h.authorizer.Authorize(r.Context(), sub, "audit:read", nil); err != nil {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "administrator access required")
		return
	}

	filter := events.EventFilter{
		Limit:  50,
		Offset: 0,
	}

	if tID := r.URL.Query().Get("tenantId"); tID != "" {
		filter.TenantID = tID
	}
	if evType := r.URL.Query().Get("type"); evType != "" {
		filter.Type = events.EventType(evType)
	}
	if resType := r.URL.Query().Get("resourceType"); resType != "" {
		filter.ResourceType = resType
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

	evts, total, err := h.eventRepo.List(r.Context(), filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"events": evts,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *EventHandler) AdminListDeliveries(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}
	if err := h.authorizer.Authorize(r.Context(), sub, "audit:read", nil); err != nil {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "administrator access required")
		return
	}

	filter := webhook.DeliveryFilter{
		Limit:  50,
		Offset: 0,
	}

	if tID := r.URL.Query().Get("tenantId"); tID != "" {
		filter.TenantID = tID
	}
	if whID := r.URL.Query().Get("webhookId"); whID != "" {
		filter.WebhookID = whID
	}
	if statusStr := r.URL.Query().Get("status"); statusStr != "" {
		st := webhook.DeliveryStatus(statusStr)
		filter.Status = &st
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

	deliveries, total, err := h.deliveryRepo.List(r.Context(), filter)
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
