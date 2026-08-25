package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	appBilling "github.com/aurora-vm/aurora/internal/app/billing"
	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// BillingHandler handles REST API endpoints for billing, subscriptions, quotas, and usage metering.
type BillingHandler struct {
	billingService *appBilling.Service
	authorizer     identity.Authorizer
}

func NewBillingHandler(billingService *appBilling.Service, authorizer identity.Authorizer) *BillingHandler {
	return &BillingHandler{
		billingService: billingService,
		authorizer:     authorizer,
	}
}

// RegisterRoutes attaches customer and administrative billing endpoints.
func (h *BillingHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Customer Billing Endpoints
	r.Route("/api/v1/billing", func(r chi.Router) {
		r.Use(authMiddleware)

		r.With(RequirePermission(h.authorizer, "billing:read")).Get("/plans", h.ListPlans)
		r.With(RequirePermission(h.authorizer, "billing:read")).Get("/subscription", h.GetSubscription)
		r.With(RequirePermission(h.authorizer, "billing:manage")).Post("/subscription", h.SubscribeTenant)
		r.With(RequirePermission(h.authorizer, "billing:manage")).Patch("/subscription", h.ChangePlan)
		r.With(RequirePermission(h.authorizer, "billing:manage")).Delete("/subscription", h.CancelSubscription)

		r.With(RequirePermission(h.authorizer, "billing:read")).Get("/usage", h.GetUsage)
		r.With(RequirePermission(h.authorizer, "billing:read")).Get("/quotas", h.GetQuotas)
		r.With(RequirePermission(h.authorizer, "billing:read")).Get("/invoices", h.ListInvoices)
		r.With(RequirePermission(h.authorizer, "billing:read")).Get("/invoices/{id}", h.GetInvoice)
	})

	// Administrative Billing Endpoints
	r.Route("/api/v1/admin/billing", func(r chi.Router) {
		r.Use(authMiddleware)

		r.With(RequirePermission(h.authorizer, "billing:plans")).Get("/plans", h.AdminListPlans)
		r.With(RequirePermission(h.authorizer, "billing:plans")).Post("/plans", h.AdminCreatePlan)
		r.With(RequirePermission(h.authorizer, "billing:plans")).Patch("/plans/{id}", h.AdminUpdatePlan)
		r.With(RequirePermission(h.authorizer, "billing:plans")).Delete("/plans/{id}", h.AdminDeletePlan)

		r.With(RequirePermission(h.authorizer, "billing:admin")).Get("/subscriptions", h.AdminListSubscriptions)
		r.With(RequirePermission(h.authorizer, "billing:admin")).Get("/usage", h.AdminGetUsage)
		r.With(RequirePermission(h.authorizer, "billing:admin")).Get("/invoices", h.AdminListInvoices)
		r.With(RequirePermission(h.authorizer, "billing:admin")).Post("/invoices/{id}/void", h.AdminVoidInvoice)
		r.With(RequirePermission(h.authorizer, "billing:admin")).Post("/invoices/{id}/regenerate", h.AdminRegenerateInvoice)
		r.With(RequirePermission(h.authorizer, "billing:admin")).Post("/invoices/regenerate", h.AdminRegenerateInvoice)
	})
}

// --- Customer Endpoints ---

func (h *BillingHandler) ListPlans(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	plans, err := h.billingService.ListPlans(r.Context(), sub, true)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"plans": plans,
		"total": len(plans),
	})
}

func (h *BillingHandler) GetSubscription(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	subscription, plan, err := h.billingService.GetSubscription(r.Context(), sub)
	if err != nil {
		if errors.Is(err, billing.ErrSubscriptionNotFound) {
			RespondJSON(w, r, http.StatusOK, map[string]interface{}{
				"subscription": nil,
				"plan":         plan,
			})
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"subscription": subscription,
		"plan":         plan,
	})
}

func (h *BillingHandler) SubscribeTenant(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req appBilling.SubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	subscription, err := h.billingService.SubscribeTenant(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, billing.ErrPlanNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		if errors.Is(err, billing.ErrSubscriptionAlreadyExists) {
			RespondError(w, r, http.StatusConflict, "CONFLICT", err.Error())
			return
		}
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusCreated, subscription)
}

func (h *BillingHandler) ChangePlan(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req appBilling.ChangePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	subscription, err := h.billingService.ChangePlan(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, billing.ErrPlanNotFound) || errors.Is(err, billing.ErrSubscriptionNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, subscription)
}

func (h *BillingHandler) CancelSubscription(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	if err := h.billingService.CancelSubscription(r.Context(), sub); err != nil {
		if errors.Is(err, billing.ErrSubscriptionNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "subscription canceled"})
}

func (h *BillingHandler) GetQuotas(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	quotas, plan, err := h.billingService.GetQuotas(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"quotas": quotas,
		"plan":   plan,
	})
}

func (h *BillingHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var start, end time.Time
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		start, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := r.URL.Query().Get("end"); endStr != "" {
		end, _ = time.Parse(time.RFC3339, endStr)
	}

	agg, err := h.billingService.GetUsage(r.Context(), sub, start, end)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, agg)
}

func (h *BillingHandler) ListInvoices(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	invoices, err := h.billingService.ListInvoices(r.Context(), sub, limit, offset)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"invoices": invoices,
		"total":    len(invoices),
	})
}

func (h *BillingHandler) GetInvoice(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	invoice, err := h.billingService.GetInvoice(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, billing.ErrInvoiceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "invoice not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, invoice)
}

// --- Admin Endpoints ---

func (h *BillingHandler) AdminListPlans(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	plans, err := h.billingService.ListPlans(r.Context(), sub, false)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"plans": plans,
		"total": len(plans),
	})
}

func (h *BillingHandler) AdminCreatePlan(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	var req appBilling.CreatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	plan, err := h.billingService.CreatePlan(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, billing.ErrPlanSlugExists) {
			RespondError(w, r, http.StatusConflict, "CONFLICT", err.Error())
			return
		}
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusCreated, plan)
}

func (h *BillingHandler) AdminUpdatePlan(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	var req appBilling.UpdatePlanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "invalid request body")
		return
	}

	plan, err := h.billingService.UpdatePlan(r.Context(), sub, id, req)
	if err != nil {
		if errors.Is(err, billing.ErrPlanNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, plan)
}

func (h *BillingHandler) AdminDeletePlan(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.billingService.DeactivatePlan(r.Context(), sub, id); err != nil {
		if errors.Is(err, billing.ErrPlanNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "plan deactivated"})
}

func (h *BillingHandler) AdminListSubscriptions(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	subs, err := h.billingService.ListAllSubscriptions(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"subscriptions": subs,
		"total":         len(subs),
	})
}

func (h *BillingHandler) AdminGetUsage(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	tenantID := r.URL.Query().Get("tenantId")
	if tenantID == "" {
		tenantID = sub.UserID
	}

	var start, end time.Time
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		start, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := r.URL.Query().Get("end"); endStr != "" {
		end, _ = time.Parse(time.RFC3339, endStr)
	}

	targetSubject := &identity.Subject{UserID: tenantID, Permissions: []string{"billing:read"}}
	agg, err := h.billingService.GetUsage(r.Context(), targetSubject, start, end)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, agg)
}

func (h *BillingHandler) AdminListInvoices(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	invoices, err := h.billingService.ListAllInvoices(r.Context(), sub, limit, offset)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"invoices": invoices,
		"total":    len(invoices),
	})
}

func (h *BillingHandler) AdminVoidInvoice(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.billingService.VoidInvoice(r.Context(), sub, id); err != nil {
		if errors.Is(err, billing.ErrInvoiceNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "invoice not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusOK, map[string]string{"message": "invoice voided"})
}

func (h *BillingHandler) AdminRegenerateInvoice(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "authentication required")
		return
	}

	tenantID := r.URL.Query().Get("tenantId")
	if tenantID == "" {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "tenantId query parameter is required")
		return
	}

	start := time.Now().AddDate(0, -1, 0)
	end := time.Now()
	if startStr := r.URL.Query().Get("start"); startStr != "" {
		start, _ = time.Parse(time.RFC3339, startStr)
	}
	if endStr := r.URL.Query().Get("end"); endStr != "" {
		end, _ = time.Parse(time.RFC3339, endStr)
	}

	inv, err := h.billingService.RegenerateInvoice(r.Context(), sub, tenantID, start, end)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}
	RespondJSON(w, r, http.StatusCreated, inv)
}
