package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	appAudit "github.com/aurora-vm/aurora/internal/app/audit"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// AuditHandler handles REST API endpoints for audit logs, compliance reports, and SIEM forwarders.
type AuditHandler struct {
	auditService *appAudit.Service
	authorizer   identity.Authorizer
}

// NewAuditHandler constructs an AuditHandler.
func NewAuditHandler(auditService *appAudit.Service, authorizer identity.Authorizer) *AuditHandler {
	return &AuditHandler{
		auditService: auditService,
		authorizer:   authorizer,
	}
}

// RegisterRoutes registers audit routes on Chi router.
func (h *AuditHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/audit", func(r chi.Router) {
		r.Use(authMiddleware)

		// List Audit Logs
		r.With(RequirePermission(h.authorizer, "audit:read")).
			Get("/logs", h.ListAuditLogs)

		// Cryptographic Chain Verification
		r.With(RequirePermission(h.authorizer, "audit:read")).
			Get("/verify", h.VerifyChain)

		// Compliance Export
		r.With(RequirePermission(h.authorizer, "audit:read")).
			Get("/export", h.ExportCompliance)

		// SIEM Destinations
		r.With(RequirePermission(h.authorizer, "audit:manage")).
			Post("/siem", h.CreateSIEMDestination)

		r.With(RequirePermission(h.authorizer, "audit:read")).
			Get("/siem", h.ListSIEMDestinations)

		r.With(RequirePermission(h.authorizer, "audit:manage")).
			Delete("/siem/{id}", h.DeleteSIEMDestination)
	})
}

func (h *AuditHandler) ListAuditLogs(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	filter := parseAuditFilter(r)
	logs, total, err := h.auditService.ListAuditLogs(r.Context(), sub, filter)
	if err != nil {
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to view audit logs")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"logs":   logs,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

func (h *AuditHandler) VerifyChain(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	limit := 1000
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	res, err := h.auditService.VerifyAuditChain(r.Context(), sub, limit)
	if err != nil {
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have permission to verify audit chain")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, res)
}

func (h *AuditHandler) ExportCompliance(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "json"
	}

	filter := parseAuditFilter(r)
	data, mimeType, err := h.auditService.ExportComplianceReport(r.Context(), sub, filter, format)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	w.Header().Set("Content-Type", mimeType)
	if format == "csv" {
		w.Header().Set("Content-Disposition", "attachment; filename=\"audit-compliance-report.csv\"")
	} else {
		w.Header().Set("Content-Disposition", "attachment; filename=\"audit-compliance-report.json\"")
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *AuditHandler) CreateSIEMDestination(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appAudit.CreateSIEMRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	dest, err := h.auditService.CreateSIEMDestination(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainAudit.ErrInvalidSIEMSpec) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, dest)
}

func (h *AuditHandler) ListSIEMDestinations(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	dests, err := h.auditService.ListSIEMDestinations(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, dests)
}

func (h *AuditHandler) DeleteSIEMDestination(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	if err := h.auditService.DeleteSIEMDestination(r.Context(), sub, id); err != nil {
		if errors.Is(err, domainAudit.ErrSIEMDestinationNotFound) {
			RespondError(w, r, http.StatusNotFound, "NOT_FOUND", "siem destination not found")
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

func parseAuditFilter(r *http.Request) domainAudit.AuditFilter {
	filter := domainAudit.AuditFilter{
		ActorID:      r.URL.Query().Get("actorId"),
		Action:       r.URL.Query().Get("action"),
		ResourceType: r.URL.Query().Get("resourceType"),
		ResourceID:   r.URL.Query().Get("resourceId"),
		Severity:     domainAudit.Severity(r.URL.Query().Get("severity")),
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

	if f := r.URL.Query().Get("from"); f != "" {
		if t, err := time.Parse(time.RFC3339, f); err == nil {
			filter.From = &t
		}
	}

	if tStr := r.URL.Query().Get("to"); tStr != "" {
		if t, err := time.Parse(time.RFC3339, tStr); err == nil {
			filter.To = &t
		}
	}

	return filter
}
