package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/google/uuid"
)

// MemoryPlanRepo implements billing.PlanRepository in memory.
type MemoryPlanRepo struct {
	mu    sync.RWMutex
	plans map[string]*billing.Plan
}

func NewMemoryPlanRepo() *MemoryPlanRepo {
	r := &MemoryPlanRepo{
		plans: make(map[string]*billing.Plan),
	}
	r.seedDefaultPlans()
	return r
}

func (r *MemoryPlanRepo) seedDefaultPlans() {
	now := time.Now().UTC()
	defaults := []*billing.Plan{
		{
			ID:                  "plan-starter",
			Name:                "Starter Developer",
			Slug:                "starter",
			Description:         "Entry level compute for staging and lightweight microservices",
			Currency:            "EUR",
			MonthlyPriceMinor:   500,  // €5.00
			YearlyPriceMinor:    5000, // €50.00
			IncludedVCPU:        1,
			IncludedMemoryMB:    1024,
			IncludedStorageMB:   10240,
			IncludedIPv4:        1,
			IncludedSnapshots:   2,
			IncludedBackups:     1,
			IncludedBandwidthGB: 1000,
			MaximumInstances:    5,
			MaximumVCPU:         4,
			MaximumMemoryMB:     8192,
			MaximumStorageMB:    81920,
			Features:            map[string]bool{"snapshots": true, "cloudinit": true, "ddos_protection": true},
			Active:              true,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
		{
			ID:                  "plan-pro",
			Name:                "Pro Production",
			Slug:                "pro",
			Description:         "High performance NVMe workloads for production APIs and databases",
			Currency:            "EUR",
			MonthlyPriceMinor:   2000,  // €20.00
			YearlyPriceMinor:    20000, // €200.00
			IncludedVCPU:        4,
			IncludedMemoryMB:    8192,
			IncludedStorageMB:   81920,
			IncludedIPv4:        2,
			IncludedSnapshots:   10,
			IncludedBackups:     5,
			IncludedBandwidthGB: 5000,
			MaximumInstances:    15,
			MaximumVCPU:         16,
			MaximumMemoryMB:     32768,
			MaximumStorageMB:    327680,
			Features:            map[string]bool{"snapshots": true, "backups": true, "cloudinit": true, "monitoring": true, "ddos_protection": true},
			Active:              true,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
		{
			ID:                  "plan-enterprise",
			Name:                "Enterprise High-Availability",
			Slug:                "enterprise",
			Description:         "Dedicated cluster resources, priority support, and multi-region failover",
			Currency:            "EUR",
			MonthlyPriceMinor:   8000,  // €80.00
			YearlyPriceMinor:    80000, // €800.00
			IncludedVCPU:        16,
			IncludedMemoryMB:    32768,
			IncludedStorageMB:   327680,
			IncludedIPv4:        5,
			IncludedSnapshots:   50,
			IncludedBackups:     20,
			IncludedBandwidthGB: 20000,
			MaximumInstances:    50,
			MaximumVCPU:         64,
			MaximumMemoryMB:     131072,
			MaximumStorageMB:    1310720,
			Features:            map[string]bool{"snapshots": true, "backups": true, "cloudinit": true, "monitoring": true, "siem": true, "priority_sla": true},
			Active:              true,
			CreatedAt:           now,
			UpdatedAt:           now,
		},
	}

	for _, p := range defaults {
		r.plans[p.ID] = p
	}
}

func (r *MemoryPlanRepo) Create(ctx context.Context, plan *billing.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.plans {
		if strings.EqualFold(p.Slug, plan.Slug) {
			return billing.ErrPlanSlugExists
		}
	}

	if plan.ID == "" {
		plan.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	plan.CreatedAt = now
	plan.UpdatedAt = now

	cp := *plan
	r.plans[plan.ID] = &cp
	return nil
}

func (r *MemoryPlanRepo) GetByID(ctx context.Context, id string) (*billing.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.plans[id]
	if !exists {
		return nil, billing.ErrPlanNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *MemoryPlanRepo) GetBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.plans {
		if strings.EqualFold(p.Slug, slug) {
			cp := *p
			return &cp, nil
		}
	}
	return nil, billing.ErrPlanNotFound
}

func (r *MemoryPlanRepo) List(ctx context.Context, activeOnly bool) ([]*billing.Plan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*billing.Plan
	for _, p := range r.plans {
		if !activeOnly || p.Active {
			cp := *p
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *MemoryPlanRepo) Update(ctx context.Context, plan *billing.Plan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.plans[plan.ID]
	if !exists {
		return billing.ErrPlanNotFound
	}

	for _, other := range r.plans {
		if other.ID != plan.ID && strings.EqualFold(other.Slug, plan.Slug) {
			return billing.ErrPlanSlugExists
		}
	}

	plan.CreatedAt = p.CreatedAt
	plan.UpdatedAt = time.Now().UTC()

	cp := *plan
	r.plans[plan.ID] = &cp
	return nil
}

func (r *MemoryPlanRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plans[id]; !exists {
		return billing.ErrPlanNotFound
	}
	delete(r.plans, id)
	return nil
}

// MemorySubscriptionRepo implements billing.SubscriptionRepository in memory.
type MemorySubscriptionRepo struct {
	mu            sync.RWMutex
	subscriptions map[string]*billing.Subscription // id -> subscription
	tenantSubs    map[string]string                // tenantId -> subscriptionId
}

func NewMemorySubscriptionRepo() *MemorySubscriptionRepo {
	return &MemorySubscriptionRepo{
		subscriptions: make(map[string]*billing.Subscription),
		tenantSubs:    make(map[string]string),
	}
}

func (r *MemorySubscriptionRepo) Create(ctx context.Context, sub *billing.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if sub.ID == "" {
		sub.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	sub.CreatedAt = now
	sub.UpdatedAt = now

	cp := *sub
	r.subscriptions[sub.ID] = &cp
	r.tenantSubs[sub.TenantID] = sub.ID
	return nil
}

func (r *MemorySubscriptionRepo) GetByID(ctx context.Context, id string) (*billing.Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	sub, exists := r.subscriptions[id]
	if !exists {
		return nil, billing.ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (r *MemorySubscriptionRepo) GetByTenantID(ctx context.Context, tenantID string) (*billing.Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	subID, exists := r.tenantSubs[tenantID]
	if !exists {
		return nil, billing.ErrSubscriptionNotFound
	}
	sub, exists := r.subscriptions[subID]
	if !exists {
		return nil, billing.ErrSubscriptionNotFound
	}
	cp := *sub
	return &cp, nil
}

func (r *MemorySubscriptionRepo) List(ctx context.Context) ([]*billing.Subscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*billing.Subscription
	for _, s := range r.subscriptions {
		cp := *s
		result = append(result, &cp)
	}
	return result, nil
}

func (r *MemorySubscriptionRepo) Update(ctx context.Context, sub *billing.Subscription) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.subscriptions[sub.ID]
	if !exists {
		return billing.ErrSubscriptionNotFound
	}

	sub.CreatedAt = existing.CreatedAt
	sub.UpdatedAt = time.Now().UTC()

	cp := *sub
	r.subscriptions[sub.ID] = &cp
	r.tenantSubs[sub.TenantID] = sub.ID
	return nil
}

func (r *MemorySubscriptionRepo) Cancel(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	sub, exists := r.subscriptions[id]
	if !exists {
		return billing.ErrSubscriptionNotFound
	}
	sub.Status = billing.SubscriptionStatusCanceled
	sub.UpdatedAt = time.Now().UTC()
	return nil
}

// MemoryQuotaRepo implements billing.QuotaRepository in memory.
type MemoryQuotaRepo struct {
	mu     sync.RWMutex
	quotas map[string]map[billing.MetricType]*billing.Quota // tenantId -> metric -> quota
}

func NewMemoryQuotaRepo() *MemoryQuotaRepo {
	return &MemoryQuotaRepo{
		quotas: make(map[string]map[billing.MetricType]*billing.Quota),
	}
}

func (r *MemoryQuotaRepo) GetQuota(ctx context.Context, tenantID string, metric billing.MetricType) (*billing.Quota, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tQuotas, exists := r.quotas[tenantID]
	if !exists {
		return nil, billing.ErrQuotaNotFound
	}
	q, exists := tQuotas[metric]
	if !exists {
		return nil, billing.ErrQuotaNotFound
	}
	cp := *q
	return &cp, nil
}

func (r *MemoryQuotaRepo) ListQuotas(ctx context.Context, tenantID string) (billing.QuotaSet, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	set := make(billing.QuotaSet)
	if tQuotas, exists := r.quotas[tenantID]; exists {
		for k, v := range tQuotas {
			cp := *v
			set[k] = &cp
		}
	}
	return set, nil
}

func (r *MemoryQuotaRepo) SetQuota(ctx context.Context, quota *billing.Quota) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.quotas[quota.TenantID]; !exists {
		r.quotas[quota.TenantID] = make(map[billing.MetricType]*billing.Quota)
	}
	quota.UpdatedAt = time.Now().UTC()
	cp := *quota
	r.quotas[quota.TenantID][quota.Metric] = &cp
	return nil
}

func (r *MemoryQuotaRepo) ReserveQuota(ctx context.Context, tenantID string, metric billing.MetricType, delta int64, limit int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.quotas[tenantID]; !exists {
		r.quotas[tenantID] = make(map[billing.MetricType]*billing.Quota)
	}

	q, exists := r.quotas[tenantID][metric]
	if !exists {
		q = &billing.Quota{
			TenantID:     tenantID,
			Metric:       metric,
			Limit:        limit,
			CurrentUsage: 0,
			ResetPeriod:  "none",
			UpdatedAt:    time.Now().UTC(),
		}
		r.quotas[tenantID][metric] = q
	}

	if limit > 0 && q.Limit != limit {
		q.Limit = limit
	}

	if q.Limit > 0 && q.CurrentUsage+delta > q.Limit {
		return fmt.Errorf("%w: metric %s limit is %d, requested allocation would reach %d",
			billing.ErrQuotaExceeded, metric, q.Limit, q.CurrentUsage+delta)
	}

	q.CurrentUsage += delta
	q.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryQuotaRepo) ReleaseQuota(ctx context.Context, tenantID string, metric billing.MetricType, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if tQuotas, exists := r.quotas[tenantID]; exists {
		if q, exists := tQuotas[metric]; exists {
			q.CurrentUsage -= delta
			if q.CurrentUsage < 0 {
				q.CurrentUsage = 0
			}
			q.UpdatedAt = time.Now().UTC()
		}
	}
	return nil
}
