package http

import (
	"net/http"

	"github.com/aurora-vm/aurora/internal/app/health"
	domainHealth "github.com/aurora-vm/aurora/internal/domain/health"
	"github.com/go-chi/chi/v5"
)

// HealthHandler handles health check HTTP requests.
type HealthHandler struct {
	service *health.Service
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(service *health.Service) *HealthHandler {
	return &HealthHandler{service: service}
}

// RegisterRoutes mounts health check endpoints on the router.
func (h *HealthHandler) RegisterRoutes(r chi.Router) {
	// Liveness probes
	r.Get("/healthz", h.LivenessCheck)
	r.Get("/health/live", h.LivenessCheck)

	// Readiness probes
	r.Get("/readyz", h.ReadinessCheck)
	r.Get("/health/ready", h.ReadinessCheck)

	// API v1 detailed diagnostic health check
	r.Route("/api/v1", func(api chi.Router) {
		api.Get("/health", h.HealthCheck)
	})
}

// LivenessCheck returns a quick 200 OK for Kubernetes/container process liveness probes.
func (h *HealthHandler) LivenessCheck(w http.ResponseWriter, r *http.Request) {
	if h.service.CheckLiveness(r.Context()) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}
	w.WriteHeader(http.StatusServiceUnavailable)
	_, _ = w.Write([]byte("FAIL"))
}

// ReadinessCheck verifies that critical infrastructure dependencies (e.g. DB) are ready.
func (h *HealthHandler) ReadinessCheck(w http.ResponseWriter, r *http.Request) {
	ready, components := h.service.CheckReadiness(r.Context())
	if ready {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("READY"))
		return
	}
	RespondError(w, r, http.StatusServiceUnavailable, "DEPENDENCIES_UNREADY", "Required system dependencies are not ready")
	_ = components
}

// HealthCheck returns detailed component health and diagnostic status.
func (h *HealthHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	report := h.service.GetHealth(r.Context())
	statusCode := http.StatusOK
	if report.Status == domainHealth.StatusUnhealthy {
		statusCode = http.StatusServiceUnavailable
	}
	RespondJSON(w, r, statusCode, report)
}
