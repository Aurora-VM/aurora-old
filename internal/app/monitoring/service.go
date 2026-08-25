package monitoring

import (
	"context"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainMonitoring "github.com/aurora-vm/aurora/internal/domain/monitoring"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
)

// Service manages metrics ingestion, historical time-series querying, and automated alerting.
type Service struct {
	metricsRepo    domainMonitoring.MetricsRepository
	thresholdRepo  domainMonitoring.AlertThresholdRepository
	alertEventRepo domainMonitoring.AlertEventRepository
	instRepo       domainCompute.InstanceRepository
	nodeRepo       domainNode.NodeRepository
	authorizer     identity.Authorizer
	auditRepo      audit.Repository
}

// NewService constructs a Monitoring Service.
func NewService(
	metricsRepo domainMonitoring.MetricsRepository,
	thresholdRepo domainMonitoring.AlertThresholdRepository,
	alertEventRepo domainMonitoring.AlertEventRepository,
	instRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		metricsRepo:    metricsRepo,
		thresholdRepo:  thresholdRepo,
		alertEventRepo: alertEventRepo,
		instRepo:       instRepo,
		nodeRepo:       nodeRepo,
		authorizer:     authorizer,
		auditRepo:      auditRepo,
	}
}

// IngestMetrics processes inbound telemetry samples and evaluates alert rules.
func (s *Service) IngestMetrics(ctx context.Context, samples []*domainMonitoring.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	if err := s.metricsRepo.InsertSamples(ctx, samples); err != nil {
		return err
	}

	// Evaluate alert thresholds for each sample
	for _, sample := range samples {
		if sample == nil {
			continue
		}
		thresholds, err := s.thresholdRepo.ListByResource(ctx, sample.ResourceType, sample.ResourceID)
		if err != nil {
			continue
		}

		for _, t := range thresholds {
			if t.MetricName == sample.MetricName && t.Evaluate(sample.Value) {
				_ = s.alertEventRepo.Create(ctx, &domainMonitoring.AlertEvent{
					ThresholdID:    t.ID,
					ResourceType:   t.ResourceType,
					ResourceID:     t.ResourceID,
					TriggeredValue: sample.Value,
					Severity:       t.Severity,
					Message: fmt.Sprintf(
						"Threshold exceeded on %s %s: %s is %.2f (operator: %s %.2f)",
						t.ResourceType, t.ResourceID, t.MetricName, sample.Value, t.Operator, t.ThresholdValue,
					),
					State:       domainMonitoring.AlertStateFiring,
					TriggeredAt: time.Now().UTC(),
				})
			}
		}
	}

	return nil
}

// QueryInstanceMetrics queries historical metrics for an instance with tenant RBAC checks.
func (s *Service) QueryInstanceMetrics(
	ctx context.Context,
	sub *identity.Subject,
	instanceID string,
	metricNames []string,
	from, to time.Time,
	stepSeconds int,
) (map[string]*domainMonitoring.TimeSeries, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
		return nil, err
	}

	if len(metricNames) == 0 {
		metricNames = []string{"cpu_percent", "memory_used_bytes", "disk_used_bytes", "net_rx_bytes", "net_tx_bytes"}
	}

	result := make(map[string]*domainMonitoring.TimeSeries)
	for _, name := range metricNames {
		ts, err := s.metricsRepo.QueryRange(ctx, domainMonitoring.ResourceTypeInstance, instanceID, name, from, to, stepSeconds)
		if err != nil {
			return nil, err
		}
		result[name] = ts
	}

	return result, nil
}

// QueryNodeMetrics queries historical telemetry for a hypervisor node.
func (s *Service) QueryNodeMetrics(
	ctx context.Context,
	sub *identity.Subject,
	nodeID string,
	metricNames []string,
	from, to time.Time,
	stepSeconds int,
) (map[string]*domainMonitoring.TimeSeries, error) {
	node, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "node:read", node.Resource()); err != nil {
		return nil, err
	}

	if len(metricNames) == 0 {
		metricNames = []string{"cpu_percent", "memory_used_bytes", "disk_used_bytes", "load_1m", "load_5m", "load_15m"}
	}

	result := make(map[string]*domainMonitoring.TimeSeries)
	for _, name := range metricNames {
		ts, err := s.metricsRepo.QueryRange(ctx, domainMonitoring.ResourceTypeNode, nodeID, name, from, to, stepSeconds)
		if err != nil {
			return nil, err
		}
		result[name] = ts
	}

	return result, nil
}

type CreateThresholdRequest struct {
	ResourceType    domainMonitoring.ResourceType       `json:"resourceType"`
	ResourceID      string                              `json:"resourceId"`
	MetricName      string                              `json:"metricName"`
	Operator        domainMonitoring.ComparisonOperator `json:"operator"`
	ThresholdValue  float64                             `json:"thresholdValue"`
	DurationSeconds int                                 `json:"durationSeconds"`
	Severity        domainMonitoring.AlertSeverity      `json:"severity"`
	Enabled         bool                                `json:"enabled"`
}

func (s *Service) CreateThreshold(ctx context.Context, sub *identity.Subject, req CreateThresholdRequest) (*domainMonitoring.AlertThreshold, error) {
	if req.ResourceID == "" || req.MetricName == "" || req.Operator == "" {
		return nil, domainMonitoring.ErrInvalidThresholdSpec
	}

	if err := s.authorizer.Authorize(ctx, sub, "monitoring:manage", nil); err != nil {
		return nil, err
	}

	sev := req.Severity
	if sev == "" {
		sev = domainMonitoring.SeverityWarning
	}

	dur := req.DurationSeconds
	if dur <= 0 {
		dur = 60
	}

	t := &domainMonitoring.AlertThreshold{
		UserID:          sub.UserID,
		ResourceType:    req.ResourceType,
		ResourceID:      req.ResourceID,
		MetricName:      req.MetricName,
		Operator:        req.Operator,
		ThresholdValue:  req.ThresholdValue,
		DurationSeconds: dur,
		Severity:        sev,
		Enabled:         true,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.thresholdRepo.Create(ctx, t); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "alert_threshold:create", t.ID, map[string]interface{}{
		"resourceType":   t.ResourceType,
		"resourceId":     t.ResourceID,
		"metricName":     t.MetricName,
		"thresholdValue": t.ThresholdValue,
	})

	return t, nil
}

func (s *Service) ListThresholds(ctx context.Context, sub *identity.Subject, resType domainMonitoring.ResourceType, resID string) ([]*domainMonitoring.AlertThreshold, error) {
	if err := s.authorizer.Authorize(ctx, sub, "monitoring:read", nil); err != nil {
		return nil, err
	}
	if resType != "" {
		return s.thresholdRepo.ListByResource(ctx, resType, resID)
	}
	return s.thresholdRepo.ListAll(ctx)
}

func (s *Service) ListAlertEvents(ctx context.Context, sub *identity.Subject, resType domainMonitoring.ResourceType, resID string, state domainMonitoring.AlertState) ([]*domainMonitoring.AlertEvent, error) {
	if err := s.authorizer.Authorize(ctx, sub, "monitoring:read", nil); err != nil {
		return nil, err
	}
	return s.alertEventRepo.List(ctx, resType, resID, state)
}

func (s *Service) AcknowledgeAlert(ctx context.Context, sub *identity.Subject, id string) (*domainMonitoring.AlertEvent, error) {
	if err := s.authorizer.Authorize(ctx, sub, "monitoring:manage", nil); err != nil {
		return nil, err
	}

	event, err := s.alertEventRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	event.State = domainMonitoring.AlertStateAcknowledged
	if err := s.alertEventRepo.Update(ctx, event); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "alert_event:acknowledge", event.ID, nil)
	return event, nil
}

func (s *Service) ResolveAlert(ctx context.Context, sub *identity.Subject, id string) (*domainMonitoring.AlertEvent, error) {
	if err := s.authorizer.Authorize(ctx, sub, "monitoring:manage", nil); err != nil {
		return nil, err
	}

	event, err := s.alertEventRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	now := time.Now().UTC()
	event.State = domainMonitoring.AlertStateResolved
	event.ResolvedAt = &now

	if err := s.alertEventRepo.Update(ctx, event); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "alert_event:resolve", event.ID, nil)
	return event, nil
}

func (s *Service) logAudit(ctx context.Context, sub *identity.Subject, action, resourceID string, details map[string]interface{}) {
	if s.auditRepo == nil {
		return
	}
	var actorID *string
	if sub != nil && sub.UserID != "" {
		actorID = &sub.UserID
	}
	var rID *string
	if resourceID != "" {
		rID = &resourceID
	}
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:    actorID,
		Action:     action,
		ResourceID: rID,
		Details:    details,
		CreatedAt:  time.Now().UTC(),
	})
}
