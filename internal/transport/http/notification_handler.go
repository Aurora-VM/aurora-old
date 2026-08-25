package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	appNotification "github.com/aurora-vm/aurora/internal/app/notification"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNotification "github.com/aurora-vm/aurora/internal/domain/notification"
	"github.com/go-chi/chi/v5"
)

type NotificationHandler struct {
	service    *appNotification.Service
	authorizer identity.Authorizer
}

func NewNotificationHandler(service *appNotification.Service, authorizer identity.Authorizer) *NotificationHandler {
	return &NotificationHandler{
		service:    service,
		authorizer: authorizer,
	}
}

func (h *NotificationHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/notifications", func(r chi.Router) {
		r.Use(authMiddleware)

		r.With(RequirePermission(h.authorizer, "notification:read")).Get("/", h.ListNotifications)
		r.With(RequirePermission(h.authorizer, "notification:read")).Get("/unread-count", h.GetUnreadCount)
		r.With(RequirePermission(h.authorizer, "notification:manage")).Post("/{id}/read", h.MarkRead)
		r.With(RequirePermission(h.authorizer, "notification:manage")).Post("/read-all", h.MarkAllRead)
		r.With(RequirePermission(h.authorizer, "notification:read")).Get("/preferences", h.GetPreferences)
		r.With(RequirePermission(h.authorizer, "notification:manage")).Put("/preferences", h.SetPreference)
	})
}

func (h *NotificationHandler) ListNotifications(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	filter := domainNotification.Filter{
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
	if unreadStr := r.URL.Query().Get("unreadOnly"); unreadStr == "true" {
		filter.UnreadOnly = true
	}
	if sevStr := r.URL.Query().Get("severity"); sevStr != "" {
		sev := domainNotification.Severity(sevStr)
		filter.Severity = &sev
	}

	notifs, total, err := h.service.ListNotifications(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"notifications": notifs,
		"total":         total,
		"limit":         filter.Limit,
		"offset":        filter.Offset,
	})
}

func (h *NotificationHandler) GetUnreadCount(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	count, err := h.service.GetUnreadCount(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"unreadCount": count,
	})
}

func (h *NotificationHandler) MarkRead(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.service.MarkAsRead(r.Context(), sub, id); err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "notification not found")
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"status": "read",
	})
}

func (h *NotificationHandler) MarkAllRead(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	count, err := h.service.MarkAllAsRead(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"markedRead": count,
	})
}

func (h *NotificationHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	prefs, err := h.service.GetPreferences(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"preferences": prefs,
	})
}

func (h *NotificationHandler) SetPreference(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var pref domainNotification.NotificationPreference
	if err := json.NewDecoder(r.Body).Decode(&pref); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_PAYLOAD", "invalid preference payload")
		return
	}

	if err := h.service.SetPreference(r.Context(), sub, &pref); err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"preference": pref,
	})
}
