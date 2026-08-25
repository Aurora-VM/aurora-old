package http

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"

	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/go-chi/chi/v5"
)

// MetricsCollector tracks real-time platform telemetry for Prometheus scraping.
type MetricsCollector struct {
	httpRequestsTotal int64
	httpErrorsTotal   int64
	jobRepo           domainJob.JobRepository
	nodeRepo          domainNode.NodeRepository
	instRepo          domainCompute.InstanceRepository
	startTime         time.Time
}

// NewMetricsCollector constructs a Prometheus metrics collector.
func NewMetricsCollector(
	jobRepo domainJob.JobRepository,
	nodeRepo domainNode.NodeRepository,
	instRepo domainCompute.InstanceRepository,
) *MetricsCollector {
	return &MetricsCollector{
		jobRepo:   jobRepo,
		nodeRepo:  nodeRepo,
		instRepo:  instRepo,
		startTime: time.Now().UTC(),
	}
}

// IncrementHTTPRequests increments request telemetry.
func (m *MetricsCollector) IncrementHTTPRequests(isError bool) {
	atomic.AddInt64(&m.httpRequestsTotal, 1)
	if isError {
		atomic.AddInt64(&m.httpErrorsTotal, 1)
	}
}

// MetricsHandler serves `/metrics` in standard Prometheus text exposition format.
type MetricsHandler struct {
	collector *MetricsCollector
}

// NewMetricsHandler constructs the MetricsHandler.
func NewMetricsHandler(collector *MetricsCollector) *MetricsHandler {
	return &MetricsHandler{collector: collector}
}

// RegisterRoutes mounts `/metrics` on the router.
func (h *MetricsHandler) RegisterRoutes(r chi.Router) {
	r.Get("/metrics", h.ServeMetrics)
}

// ServeMetrics formats and outputs Prometheus-compatible metrics text.
func (h *MetricsHandler) ServeMetrics(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	uptimeSeconds := time.Since(h.collector.startTime).Seconds()
	totalReqs := atomic.LoadInt64(&h.collector.httpRequestsTotal)
	totalErrors := atomic.LoadInt64(&h.collector.httpErrorsTotal)

	// Output Core Platform Metrics
	fmt.Fprintf(w, "# HELP aurora_uptime_seconds Total runtime uptime in seconds\n")
	fmt.Fprintf(w, "# TYPE aurora_uptime_seconds gauge\n")
	fmt.Fprintf(w, "aurora_uptime_seconds %.2f\n\n", uptimeSeconds)

	fmt.Fprintf(w, "# HELP aurora_http_requests_total Total HTTP requests processed\n")
	fmt.Fprintf(w, "# TYPE aurora_http_requests_total counter\n")
	fmt.Fprintf(w, "aurora_http_requests_total %d\n\n", totalReqs)

	fmt.Fprintf(w, "# HELP aurora_http_errors_total Total HTTP 4xx/5xx responses\n")
	fmt.Fprintf(w, "# TYPE aurora_http_errors_total counter\n")
	fmt.Fprintf(w, "aurora_http_errors_total %d\n\n", totalErrors)

	// Collect Node Health Metrics
	if h.collector.nodeRepo != nil {
		nodes, err := h.collector.nodeRepo.List(ctx)
		if err == nil {
			fmt.Fprintf(w, "# HELP aurora_nodes_total Total enrolled hypervisor nodes by status\n")
			fmt.Fprintf(w, "# TYPE aurora_nodes_total gauge\n")
			statusCounts := make(map[string]int)
			for _, n := range nodes {
				statusCounts[string(n.Status)]++
			}
			for st, count := range statusCounts {
				fmt.Fprintf(w, "aurora_nodes_total{status=\"%s\"} %d\n", st, count)
			}
			fmt.Fprintf(w, "\n")
		}
	}

	// Collect Instance Metrics
	if h.collector.instRepo != nil {
		instances, err := h.collector.instRepo.ListAll(ctx)
		if err == nil {
			fmt.Fprintf(w, "# HELP aurora_instances_total Total instances by status\n")
			fmt.Fprintf(w, "# TYPE aurora_instances_total gauge\n")
			statusCounts := make(map[string]int)
			for _, inst := range instances {
				statusCounts[string(inst.Status)]++
			}
			for st, count := range statusCounts {
				fmt.Fprintf(w, "aurora_instances_total{status=\"%s\"} %d\n", st, count)
			}
			fmt.Fprintf(w, "\n")
		}
	}

	// Collect Job Metrics
	if h.collector.jobRepo != nil {
		jobs, _, err := h.collector.jobRepo.List(ctx, domainJob.JobFilter{Limit: 500})
		if err == nil {
			fmt.Fprintf(w, "# HELP aurora_jobs_total Total asynchronous jobs by status\n")
			fmt.Fprintf(w, "# TYPE aurora_jobs_total gauge\n")
			statusCounts := make(map[string]int)
			for _, j := range jobs {
				statusCounts[string(j.Status)]++
			}
			for st, count := range statusCounts {
				fmt.Fprintf(w, "aurora_jobs_total{status=\"%s\"} %d\n", st, count)
			}
			fmt.Fprintf(w, "\n")
		}
	}
}
