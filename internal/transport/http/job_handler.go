package http

import (
	"encoding/json"
	"net/http"
	"strconv"

	appJob "github.com/aurora-vm/aurora/internal/app/job"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	"github.com/go-chi/chi/v5"
)

// JobHandler exposes REST endpoints for asynchronous jobs.
type JobHandler struct {
	engine     *appJob.Engine
	authorizer domainIdentity.Authorizer
}

// NewJobHandler constructs the JobHandler.
func NewJobHandler(engine *appJob.Engine, authorizer domainIdentity.Authorizer) *JobHandler {
	return &JobHandler{
		engine:     engine,
		authorizer: authorizer,
	}
}

// RegisterRoutes registers job routes on the router.
func (h *JobHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	// Customer & Tenant Routes
	r.Route("/api/v1/jobs", func(jobs chi.Router) {
		jobs.Use(authMiddleware)
		jobs.Get("/", h.ListJobs)
		jobs.Post("/", h.SubmitJob)
		jobs.Get("/{id}", h.GetJob)
		jobs.Post("/{id}/cancel", h.CancelJob)
		jobs.Post("/{id}/retry", h.RetryJob)
	})

	// Superadmin Administrative Job Hub
	r.Route("/api/v1/admin/jobs", func(adminJobs chi.Router) {
		adminJobs.Use(authMiddleware)
		adminJobs.Get("/", h.AdminListJobs)
		adminJobs.Get("/{id}", h.GetJob)
		adminJobs.Post("/{id}/cancel", h.CancelJob)
		adminJobs.Post("/{id}/retry", h.RetryJob)
	})
}

// ListJobs returns tenant-scoped jobs.
func (h *JobHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	status := domainJob.Status(r.URL.Query().Get("status"))
	jobType := domainJob.Type(r.URL.Query().Get("type"))

	filter := domainJob.JobFilter{
		Status: status,
		Type:   jobType,
		Limit:  limit,
		Offset: offset,
	}

	jobs, total, err := h.engine.ListJobs(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// AdminListJobs returns cross-tenant jobs for superadmins.
func (h *JobHandler) AdminListJobs(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil || !sub.IsSuperadmin() {
		RespondError(w, r, http.StatusForbidden, "FORBIDDEN", "Administrative privilege required")
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	filter := domainJob.JobFilter{
		TenantID: r.URL.Query().Get("tenantId"),
		Status:   domainJob.Status(r.URL.Query().Get("status")),
		Type:     domainJob.Type(r.URL.Query().Get("type")),
		Limit:    limit,
		Offset:   offset,
	}

	jobs, total, err := h.engine.ListJobs(r.Context(), sub, filter)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"jobs":   jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// GetJob returns detailed status of a specific job.
func (h *JobHandler) GetJob(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	job, err := h.engine.GetJob(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusNotFound, "NOT_FOUND", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, job)
}

// CancelJob cancels an active or pending job.
func (h *JobHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	reason := r.URL.Query().Get("reason")
	if reason == "" {
		reason = "Canceled by user request"
	}

	if err := h.engine.CancelJob(r.Context(), sub, id, reason); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"message": "Job cancellation requested",
	})
}

// RetryJob resets a failed/canceled job for re-execution.
func (h *JobHandler) RetryJob(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	job, err := h.engine.RetryJob(r.Context(), sub, id)
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, job)
}

// SubmitJob accepts an asynchronous job submission.
func (h *JobHandler) SubmitJob(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication required")
		return
	}

	var req struct {
		Type         domainJob.Type  `json:"type"`
		ResourceType string          `json:"resourceType"`
		ResourceID   string          `json:"resourceId"`
		Payload      json.RawMessage `json:"payload"`
		MaxRetries   int             `json:"maxRetries"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", "Invalid request body")
		return
	}

	job, err := h.engine.Submit(r.Context(), sub, &domainJob.Job{
		Type:         req.Type,
		ResourceType: req.ResourceType,
		ResourceID:   req.ResourceID,
		Payload:      req.Payload,
		MaxRetries:   req.MaxRetries,
	})
	if err != nil {
		RespondError(w, r, http.StatusBadRequest, "BAD_REQUEST", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusAccepted, job)
}
