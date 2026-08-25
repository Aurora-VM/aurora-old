package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/monitoring"
	"github.com/aurora-vm/aurora/internal/infra/telemetry"
	"github.com/google/uuid"
)

// MemoryMetricsRepo implements monitoring.MetricsRepository in memory using a ring buffer.
type MemoryMetricsRepo struct {
	ringBuffer *telemetry.RingBuffer
}

func NewMemoryMetricsRepo() *MemoryMetricsRepo {
	return &MemoryMetricsRepo{
		ringBuffer: telemetry.NewRingBuffer(50000),
	}
}

func (r *MemoryMetricsRepo) InsertSamples(ctx context.Context, samples []*monitoring.MetricSample) error {
	r.ringBuffer.PushBatch(samples)
	return nil
}

func (r *MemoryMetricsRepo) QueryRange(ctx context.Context, resType monitoring.ResourceType, resID, metricName string, from, to time.Time, stepSeconds int) (*monitoring.TimeSeries, error) {
	return r.ringBuffer.QueryRange(resType, resID, metricName, from, to, stepSeconds), nil
}

// MemoryAlertThresholdRepo implements monitoring.AlertThresholdRepository in memory.
type MemoryAlertThresholdRepo struct {
	mu         sync.RWMutex
	thresholds map[string]*monitoring.AlertThreshold
}

func NewMemoryAlertThresholdRepo() *MemoryAlertThresholdRepo {
	return &MemoryAlertThresholdRepo{
		thresholds: make(map[string]*monitoring.AlertThreshold),
	}
}

func (r *MemoryAlertThresholdRepo) Create(ctx context.Context, t *monitoring.AlertThreshold) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	t.CreatedAt = now
	t.UpdatedAt = now

	cp := *t
	r.thresholds[t.ID] = &cp
	return nil
}

func (r *MemoryAlertThresholdRepo) GetByID(ctx context.Context, id string) (*monitoring.AlertThreshold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	t, ok := r.thresholds[id]
	if !ok {
		return nil, monitoring.ErrThresholdNotFound
	}
	cp := *t
	return &cp, nil
}

func (r *MemoryAlertThresholdRepo) ListByResource(ctx context.Context, resType monitoring.ResourceType, resID string) ([]*monitoring.AlertThreshold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*monitoring.AlertThreshold
	for _, t := range r.thresholds {
		if t.ResourceType == resType && (resID == "" || t.ResourceID == resID) {
			cp := *t
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *MemoryAlertThresholdRepo) ListAll(ctx context.Context) ([]*monitoring.AlertThreshold, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*monitoring.AlertThreshold
	for _, t := range r.thresholds {
		cp := *t
		result = append(result, &cp)
	}
	return result, nil
}

func (r *MemoryAlertThresholdRepo) Update(ctx context.Context, t *monitoring.AlertThreshold) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.thresholds[t.ID]; !ok {
		return monitoring.ErrThresholdNotFound
	}
	t.UpdatedAt = time.Now().UTC()
	cp := *t
	r.thresholds[t.ID] = &cp
	return nil
}

func (r *MemoryAlertThresholdRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.thresholds[id]; !ok {
		return monitoring.ErrThresholdNotFound
	}
	delete(r.thresholds, id)
	return nil
}

// MemoryAlertEventRepo implements monitoring.AlertEventRepository in memory.
type MemoryAlertEventRepo struct {
	mu     sync.RWMutex
	events map[string]*monitoring.AlertEvent
}

func NewMemoryAlertEventRepo() *MemoryAlertEventRepo {
	return &MemoryAlertEventRepo{
		events: make(map[string]*monitoring.AlertEvent),
	}
}

func (r *MemoryAlertEventRepo) Create(ctx context.Context, e *monitoring.AlertEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.TriggeredAt.IsZero() {
		e.TriggeredAt = time.Now().UTC()
	}

	cp := *e
	r.events[e.ID] = &cp
	return nil
}

func (r *MemoryAlertEventRepo) GetByID(ctx context.Context, id string) (*monitoring.AlertEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.events[id]
	if !ok {
		return nil, monitoring.ErrAlertEventNotFound
	}
	cp := *e
	return &cp, nil
}

func (r *MemoryAlertEventRepo) List(ctx context.Context, resType monitoring.ResourceType, resID string, state monitoring.AlertState) ([]*monitoring.AlertEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*monitoring.AlertEvent
	for _, e := range r.events {
		if (resType == "" || e.ResourceType == resType) &&
			(resID == "" || e.ResourceID == resID) &&
			(state == "" || e.State == state) {
			cp := *e
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *MemoryAlertEventRepo) Update(ctx context.Context, e *monitoring.AlertEvent) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.events[e.ID]; !ok {
		return monitoring.ErrAlertEventNotFound
	}
	cp := *e
	r.events[e.ID] = &cp
	return nil
}
