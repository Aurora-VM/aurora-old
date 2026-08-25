package billing

import (
	"context"
	"fmt"
	"sync"

	"github.com/aurora-vm/aurora/internal/domain/billing"
)

// QuotaEngine provides race-safe, atomic resource capacity verification and reservation.
type QuotaEngine struct {
	mu        sync.Mutex
	quotaRepo billing.QuotaRepository
	planRepo  billing.PlanRepository
	subRepo   billing.SubscriptionRepository
}

func NewQuotaEngine(
	quotaRepo billing.QuotaRepository,
	planRepo billing.PlanRepository,
	subRepo billing.SubscriptionRepository,
) *QuotaEngine {
	return &QuotaEngine{
		quotaRepo: quotaRepo,
		planRepo:  planRepo,
		subRepo:   subRepo,
	}
}

// GetTenantPlan loads the active plan for a tenant, or nil if none.
func (e *QuotaEngine) GetTenantPlan(ctx context.Context, tenantID string) (*billing.Plan, error) {
	sub, err := e.subRepo.GetByTenantID(ctx, tenantID)
	if err != nil {
		// Fallback to default starter plan if no explicit subscription yet
		plans, listErr := e.planRepo.List(ctx, true)
		if listErr == nil && len(plans) > 0 {
			return plans[0], nil
		}
		return nil, err
	}
	return e.planRepo.GetByID(ctx, sub.PlanID)
}

// CheckQuota validates if a tenant has sufficient quota for a requested resource allocation.
func (e *QuotaEngine) CheckQuota(ctx context.Context, tenantID string, spec billing.ResourceSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	plan, err := e.GetTenantPlan(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("unable to resolve tenant plan: %w", err)
	}

	quotas, err := e.quotaRepo.ListQuotas(ctx, tenantID)
	if err != nil {
		return err
	}

	// 1. Instance Count
	if spec.Instances > 0 {
		q := quotas[billing.MetricInstanceCount]
		current := int64(0)
		if q != nil {
			current = q.CurrentUsage
		}
		limit := int64(plan.MaximumInstances)
		if limit > 0 && current+int64(spec.Instances) > limit {
			return fmt.Errorf("%w: max instances limit (%d) reached (current: %d, requested: +%d)",
				billing.ErrQuotaExceeded, limit, current, spec.Instances)
		}
	}

	// 2. vCPU Cores
	if spec.VCPU > 0 {
		q := quotas[billing.MetricVCPUHours]
		current := int64(0)
		if q != nil {
			current = q.CurrentUsage
		}
		limit := int64(plan.MaximumVCPU)
		if limit > 0 && current+int64(spec.VCPU) > limit {
			return fmt.Errorf("%w: max vCPU limit (%d) reached (current: %d, requested: +%d)",
				billing.ErrQuotaExceeded, limit, current, spec.VCPU)
		}
	}

	// 3. Memory MB
	if spec.MemoryMB > 0 {
		q := quotas[billing.MetricRAMGBHours]
		current := int64(0)
		if q != nil {
			current = q.CurrentUsage
		}
		limit := plan.MaximumMemoryMB
		if limit > 0 && current+spec.MemoryMB > limit {
			return fmt.Errorf("%w: max RAM limit (%d MB) reached (current: %d MB, requested: +%d MB)",
				billing.ErrQuotaExceeded, limit, current, spec.MemoryMB)
		}
	}

	// 4. Storage MB
	if spec.StorageMB > 0 {
		q := quotas[billing.MetricStorageGBMonths]
		current := int64(0)
		if q != nil {
			current = q.CurrentUsage
		}
		limit := plan.MaximumStorageMB
		if limit > 0 && current+spec.StorageMB > limit {
			return fmt.Errorf("%w: max storage limit (%d MB) reached (current: %d MB, requested: +%d MB)",
				billing.ErrQuotaExceeded, limit, current, spec.StorageMB)
		}
	}

	return nil
}

// ReserveQuota atomically reserves resources for a tenant, returning an error if capacity is exceeded.
func (e *QuotaEngine) ReserveQuota(ctx context.Context, tenantID string, spec billing.ResourceSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	plan, err := e.GetTenantPlan(ctx, tenantID)
	if err != nil {
		return fmt.Errorf("unable to resolve tenant plan: %w", err)
	}

	// Atomic reservations per metric
	if spec.Instances > 0 {
		if err := e.quotaRepo.ReserveQuota(ctx, tenantID, billing.MetricInstanceCount, int64(spec.Instances), int64(plan.MaximumInstances)); err != nil {
			return err
		}
	}
	if spec.VCPU > 0 {
		if err := e.quotaRepo.ReserveQuota(ctx, tenantID, billing.MetricVCPUHours, int64(spec.VCPU), int64(plan.MaximumVCPU)); err != nil {
			// Rollback instance quota if vCPU failed
			if spec.Instances > 0 {
				_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricInstanceCount, int64(spec.Instances))
			}
			return err
		}
	}
	if spec.MemoryMB > 0 {
		if err := e.quotaRepo.ReserveQuota(ctx, tenantID, billing.MetricRAMGBHours, spec.MemoryMB, plan.MaximumMemoryMB); err != nil {
			if spec.Instances > 0 {
				_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricInstanceCount, int64(spec.Instances))
			}
			if spec.VCPU > 0 {
				_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricVCPUHours, int64(spec.VCPU))
			}
			return err
		}
	}
	if spec.StorageMB > 0 {
		if err := e.quotaRepo.ReserveQuota(ctx, tenantID, billing.MetricStorageGBMonths, spec.StorageMB, plan.MaximumStorageMB); err != nil {
			if spec.Instances > 0 {
				_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricInstanceCount, int64(spec.Instances))
			}
			if spec.VCPU > 0 {
				_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricVCPUHours, int64(spec.VCPU))
			}
			if spec.MemoryMB > 0 {
				_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricRAMGBHours, spec.MemoryMB)
			}
			return err
		}
	}

	return nil
}

// ReleaseQuota decrements tenant resource allocations upon destruction or downsizing.
func (e *QuotaEngine) ReleaseQuota(ctx context.Context, tenantID string, spec billing.ResourceSpec) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if spec.Instances > 0 {
		_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricInstanceCount, int64(spec.Instances))
	}
	if spec.VCPU > 0 {
		_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricVCPUHours, int64(spec.VCPU))
	}
	if spec.MemoryMB > 0 {
		_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricRAMGBHours, spec.MemoryMB)
	}
	if spec.StorageMB > 0 {
		_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricStorageGBMonths, spec.StorageMB)
	}
	if spec.IPv4 > 0 {
		_ = e.quotaRepo.ReleaseQuota(ctx, tenantID, billing.MetricIPv4Addresses, int64(spec.IPv4))
	}
	return nil
}

// GetTenantQuotas returns all current allocations and limits for a tenant.
func (e *QuotaEngine) GetTenantQuotas(ctx context.Context, tenantID string) (billing.QuotaSet, *billing.Plan, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	plan, err := e.GetTenantPlan(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}

	quotas, err := e.quotaRepo.ListQuotas(ctx, tenantID)
	if err != nil {
		return nil, nil, err
	}

	// Ensure standard metrics exist in quota set with plan limits
	if quotas[billing.MetricInstanceCount] == nil {
		quotas[billing.MetricInstanceCount] = &billing.Quota{
			TenantID:     tenantID,
			Metric:       billing.MetricInstanceCount,
			Limit:        int64(plan.MaximumInstances),
			CurrentUsage: 0,
		}
	}
	if quotas[billing.MetricVCPUHours] == nil {
		quotas[billing.MetricVCPUHours] = &billing.Quota{
			TenantID:     tenantID,
			Metric:       billing.MetricVCPUHours,
			Limit:        int64(plan.MaximumVCPU),
			CurrentUsage: 0,
		}
	}
	if quotas[billing.MetricRAMGBHours] == nil {
		quotas[billing.MetricRAMGBHours] = &billing.Quota{
			TenantID:     tenantID,
			Metric:       billing.MetricRAMGBHours,
			Limit:        plan.MaximumMemoryMB,
			CurrentUsage: 0,
		}
	}
	if quotas[billing.MetricStorageGBMonths] == nil {
		quotas[billing.MetricStorageGBMonths] = &billing.Quota{
			TenantID:     tenantID,
			Metric:       billing.MetricStorageGBMonths,
			Limit:        plan.MaximumStorageMB,
			CurrentUsage: 0,
		}
	}

	return quotas, plan, nil
}
