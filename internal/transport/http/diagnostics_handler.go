package http

import (
	"net/http"

	appDiagnostics "github.com/aurora-vm/aurora/internal/app/diagnostics"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	"github.com/go-chi/chi/v5"
)

// DiagnosticsHandler provides machine-readable diagnostic status and operational runbooks.
type DiagnosticsHandler struct {
	diagService *appDiagnostics.Service
	authorizer  domainIdentity.Authorizer
}

func NewDiagnosticsHandler(
	diagService *appDiagnostics.Service,
	authorizer domainIdentity.Authorizer,
) *DiagnosticsHandler {
	return &DiagnosticsHandler{
		diagService: diagService,
		authorizer:  authorizer,
	}
}

func (h *DiagnosticsHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/admin/diagnostics", func(admin chi.Router) {
		admin.Use(authMiddleware)
		admin.Get("/", h.GetDiagnostics)
		admin.Get("/runbooks", h.GetRunbooks)
	})
}

func (h *DiagnosticsHandler) GetDiagnostics(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	report, err := h.diagService.GetDiagnostics(r.Context(), sub)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "DIAGNOSTICS_FAILED", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, report)
}

func (h *DiagnosticsHandler) GetRunbooks(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Superadmin access required")
		return
	}

	runbooks := h.diagService.GetRunbooks()
	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"runbooks": runbooks,
		"total":    len(runbooks),
	})
}
