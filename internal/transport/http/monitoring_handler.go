package http

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	appMonitoring "github.com/aurora-vm/aurora/internal/app/monitoring"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainMonitoring "github.com/aurora-vm/aurora/internal/domain/monitoring"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/go-chi/chi/v5"
)

// MonitoringHandler handles REST API endpoints for telemetry metrics and alerts.
type MonitoringHandler struct {
	monitoringService *appMonitoring.Service
	authorizer        identity.Authorizer
}

// NewMonitoringHandler constructs a MonitoringHandler.
func NewMonitoringHandler(monitoringService *appMonitoring.Service, authorizer identity.Authorizer) *MonitoringHandler {
	return &MonitoringHandler{
		monitoringService: monitoringService,
		authorizer:        authorizer,
	}
}

// RegisterRoutes registers monitoring routes onto the Chi router.
func (h *MonitoringHandler) RegisterRoutes(r chi.Router, authMiddleware func(http.Handler) http.Handler) {
	r.Route("/api/v1/monitoring", func(r chi.Router) {
		r.Use(authMiddleware)

		// Ingest Metrics
		r.With(RequirePermission(h.authorizer, "monitoring:manage")).
			Post("/metrics", h.IngestMetrics)

		// Instance Metrics
		r.With(RequirePermission(h.authorizer, "instance:read")).
			Get("/instances/{id}/metrics", h.GetInstanceMetrics)

		// Node Metrics
		r.With(RequirePermission(h.authorizer, "node:read")).
			Get("/nodes/{id}/metrics", h.GetNodeMetrics)

		// Alert Thresholds
		r.With(RequirePermission(h.authorizer, "monitoring:manage")).
			Post("/thresholds", h.CreateThreshold)

		r.With(RequirePermission(h.authorizer, "monitoring:read")).
			Get("/thresholds", h.ListThresholds)

		// Alert Events
		r.With(RequirePermission(h.authorizer, "monitoring:read")).
			Get("/alerts", h.ListAlertEvents)

		r.With(RequirePermission(h.authorizer, "monitoring:manage")).
			Post("/alerts/{id}/ack", h.AcknowledgeAlert)

		r.With(RequirePermission(h.authorizer, "monitoring:manage")).
			Post("/alerts/{id}/resolve", h.ResolveAlert)
	})
}

type IngestMetricsRequest struct {
	Samples []*domainMonitoring.MetricSample `json:"samples"`
}

func (h *MonitoringHandler) IngestMetrics(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req IngestMetricsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	if err := h.monitoringService.IngestMetrics(r.Context(), req.Samples); err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, map[string]interface{}{
		"ingested": len(req.Samples),
	})
}

func (h *MonitoringHandler) GetInstanceMetrics(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	instanceID := chi.URLParam(r, "id")
	from, to, step := parseTimeRangeParams(r)

	var metricNames []string
	if m := r.URL.Query().Get("metrics"); m != "" {
		metricNames = strings.Split(m, ",")
	}

	series, err := h.monitoringService.QueryInstanceMetrics(r.Context(), sub, instanceID, metricNames, from, to, step)
	if err != nil {
		if errors.Is(err, domainCompute.ErrInstanceNotFound) {
			RespondError(w, r, http.StatusNotFound, "INSTANCE_NOT_FOUND", "instance not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to this instance")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, series)
}

func (h *MonitoringHandler) GetNodeMetrics(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	nodeID := chi.URLParam(r, "id")
	from, to, step := parseTimeRangeParams(r)

	var metricNames []string
	if m := r.URL.Query().Get("metrics"); m != "" {
		metricNames = strings.Split(m, ",")
	}

	series, err := h.monitoringService.QueryNodeMetrics(r.Context(), sub, nodeID, metricNames, from, to, step)
	if err != nil {
		if errors.Is(err, domainNode.ErrNodeNotFound) {
			RespondError(w, r, http.StatusNotFound, "NODE_NOT_FOUND", "node not found")
			return
		}
		if errors.Is(err, identity.ErrInsufficientPermission) || errors.Is(err, identity.ErrResourceForbidden) {
			RespondError(w, r, http.StatusForbidden, "AUTH_FORBIDDEN", "you do not have access to this node")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, series)
}

func (h *MonitoringHandler) CreateThreshold(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	var req appMonitoring.CreateThresholdRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		RespondError(w, r, http.StatusBadRequest, "INVALID_REQUEST", "malformed JSON body")
		return
	}

	t, err := h.monitoringService.CreateThreshold(r.Context(), sub, req)
	if err != nil {
		if errors.Is(err, domainMonitoring.ErrInvalidThresholdSpec) {
			RespondError(w, r, http.StatusBadRequest, "INVALID_SPEC", err.Error())
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusCreated, t)
}

func (h *MonitoringHandler) ListThresholds(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	resType := domainMonitoring.ResourceType(r.URL.Query().Get("resourceType"))
	resID := r.URL.Query().Get("resourceId")

	thresholds, err := h.monitoringService.ListThresholds(r.Context(), sub, resType, resID)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, thresholds)
}

func (h *MonitoringHandler) ListAlertEvents(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	resType := domainMonitoring.ResourceType(r.URL.Query().Get("resourceType"))
	resID := r.URL.Query().Get("resourceId")
	state := domainMonitoring.AlertState(r.URL.Query().Get("state"))

	events, err := h.monitoringService.ListAlertEvents(r.Context(), sub, resType, resID, state)
	if err != nil {
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, events)
}

func (h *MonitoringHandler) AcknowledgeAlert(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	event, err := h.monitoringService.AcknowledgeAlert(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainMonitoring.ErrAlertEventNotFound) {
			RespondError(w, r, http.StatusNotFound, "ALERT_NOT_FOUND", "alert event not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, event)
}

func (h *MonitoringHandler) ResolveAlert(w http.ResponseWriter, r *http.Request) {
	sub := GetSubject(r.Context())
	if sub == nil {
		RespondError(w, r, http.StatusUnauthorized, "AUTH_UNAUTHORIZED", "authentication required")
		return
	}

	id := chi.URLParam(r, "id")
	event, err := h.monitoringService.ResolveAlert(r.Context(), sub, id)
	if err != nil {
		if errors.Is(err, domainMonitoring.ErrAlertEventNotFound) {
			RespondError(w, r, http.StatusNotFound, "ALERT_NOT_FOUND", "alert event not found")
			return
		}
		RespondError(w, r, http.StatusInternalServerError, "INTERNAL_ERROR", err.Error())
		return
	}

	RespondJSON(w, r, http.StatusOK, event)
}

func parseTimeRangeParams(r *http.Request) (time.Time, time.Time, int) {
	now := time.Now().UTC()
	from := now.Add(-1 * time.Hour)
	to := now
	step := 10

	if f := r.URL.Query().Get("from"); f != "" {
		if tVal, err := time.Parse(time.RFC3339, f); err == nil {
			from = tVal
		} else if sec, err := strconv.ParseInt(f, 10, 64); err == nil {
			from = time.Unix(sec, 0).UTC()
		}
	}

	if t := r.URL.Query().Get("to"); t != "" {
		if tVal, err := time.Parse(time.RFC3339, t); err == nil {
			to = tVal
		} else if sec, err := strconv.ParseInt(t, 10, 64); err == nil {
			to = time.Unix(sec, 0).UTC()
		}
	}

	if s := r.URL.Query().Get("stepSeconds"); s != "" {
		if sVal, err := strconv.Atoi(s); err == nil && sVal > 0 {
			step = sVal
		}
	}

	return from, to, step
}
