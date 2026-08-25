package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/google/uuid"
)

// MemoryUsageRepo implements billing.UsageRepository in memory.
type MemoryUsageRepo struct {
	mu              sync.RWMutex
	records         map[string]*billing.UsageRecord // id -> record
	idempotencyKeys map[string]bool                 // idempotencyKey -> exists
}

func NewMemoryUsageRepo() *MemoryUsageRepo {
	return &MemoryUsageRepo{
		records:         make(map[string]*billing.UsageRecord),
		idempotencyKeys: make(map[string]bool),
	}
}

func (r *MemoryUsageRepo) RecordUsage(ctx context.Context, record *billing.UsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if record.IdempotencyKey != "" {
		if r.idempotencyKeys[record.IdempotencyKey] {
			// Idempotent no-op: already recorded
			return nil
		}
		r.idempotencyKeys[record.IdempotencyKey] = true
	}

	if record.ID == "" {
		record.ID = uuid.NewString()
	}
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}

	cp := *record
	r.records[record.ID] = &cp
	return nil
}

func (r *MemoryUsageRepo) GetAggregate(ctx context.Context, tenantID string, start, end time.Time) (*billing.UsageAggregate, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agg := &billing.UsageAggregate{
		TenantID:    tenantID,
		PeriodStart: start,
		PeriodEnd:   end,
		Metrics:     make(map[billing.MetricType]float64),
	}

	for _, rec := range r.records {
		if rec.TenantID != tenantID {
			continue
		}
		// Check overlap with billing window
		if rec.PeriodEnd.Before(start) || rec.PeriodStart.After(end) {
			continue
		}
		agg.Metrics[rec.Metric] += rec.Quantity
	}

	return agg, nil
}

func (r *MemoryUsageRepo) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*billing.UsageRecord, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var tenantRecords []*billing.UsageRecord
	for _, rec := range r.records {
		if rec.TenantID == tenantID {
			cp := *rec
			tenantRecords = append(tenantRecords, &cp)
		}
	}

	// Sort newest first
	sort.Slice(tenantRecords, func(i, j int) bool {
		return tenantRecords[i].CreatedAt.After(tenantRecords[j].CreatedAt)
	})

	if offset >= len(tenantRecords) {
		return []*billing.UsageRecord{}, nil
	}

	end := offset + limit
	if limit <= 0 || end > len(tenantRecords) {
		end = len(tenantRecords)
	}

	return tenantRecords[offset:end], nil
}
