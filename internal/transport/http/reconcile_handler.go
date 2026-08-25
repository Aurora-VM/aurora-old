package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	appReconcile "github.com/aurora-vm/aurora/internal/app/reconcile"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// ReconcileHandler exposes HTTP endpoints for control plane state reconciliation.
type ReconcileHandler struct {
	reconcileService *appReconcile.Service
	authorizer       domainIdentity.Authorizer
}

func NewReconcileHandler(
	reconcileService *appReconcile.Service,
	authorizer domainIdentity.Authorizer,
) *ReconcileHandler {
	return &ReconcileHandler{
		reconcileService: reconcileService,
		authorizer:       authorizer,
	}
}

func (h *ReconcileHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/admin/reconcile", func(admin chi.Router) {
		admin.Use(authMiddleware)
		admin.Post("/", h.TriggerReconciliation)
		admin.Get("/reports", h.ListReports)
		admin.Get("/latest", h.GetLatestReport)
	})
}

func (h *ReconcileHandler) TriggerReconciliation(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin privilege required")
		return
	}

	var req struct {
		DryRun  bool   `json:"dryRun"`
		Trigger string `json:"trigger"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req.Trigger == "" {
		req.Trigger = "manual_admin"
	}

	report, err := h.reconcileService.Reconcile(r.Context(), req.DryRun, req.Trigger)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "RECONCILIATION_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, report)
}

func (h *ReconcileHandler) ListReports(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin privilege required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	reports, total, err := h.reconcileService.ListReports(r.Context(), limit, offset)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"reports": reports,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	})
}

func (h *ReconcileHandler) GetLatestReport(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin privilege required")
		return
	}

	report, err := h.reconcileService.GetLatestReport(r.Context())
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}
	if report == nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "No reconciliation reports recorded yet")
		return
	}

	RespondJSON(w, r, http.StatusOK, report)
}
