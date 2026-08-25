package monitoring

import (
	"context"
	"time"
)

// MetricsRepository handles persisting and querying telemetry samples.
type MetricsRepository interface {
	InsertSamples(ctx context.Context, samples []*MetricSample) error
	QueryRange(ctx context.Context, resType ResourceType, resID, metricName string, from, to time.Time, stepSeconds int) (*TimeSeries, error)
}

// AlertThresholdRepository handles CRUD operations for automated monitoring thresholds.
type AlertThresholdRepository interface {
	Create(ctx context.Context, threshold *AlertThreshold) error
	GetByID(ctx context.Context, id string) (*AlertThreshold, error)
	ListByResource(ctx context.Context, resType ResourceType, resID string) ([]*AlertThreshold, error)
	ListAll(ctx context.Context) ([]*AlertThreshold, error)
	Update(ctx context.Context, threshold *AlertThreshold) error
	Delete(ctx context.Context, id string) error
}

// AlertEventRepository handles lifecycle state and queries for triggered alerts.
type AlertEventRepository interface {
	Create(ctx context.Context, event *AlertEvent) error
	GetByID(ctx context.Context, id string) (*AlertEvent, error)
	List(ctx context.Context, resType ResourceType, resID string, state AlertState) ([]*AlertEvent, error)
	Update(ctx context.Context, event *AlertEvent) error
}
