package billing

import (
	"fmt"
	"time"
)

// Quota represents an active capacity limit and current allocation for a tenant.
type Quota struct {
	TenantID     string     `json:"tenantId"`
	Metric       MetricType `json:"metric"`
	Limit        int64      `json:"limit"`
	CurrentUsage int64      `json:"currentUsage"`
	ResetPeriod  string     `json:"resetPeriod,omitempty"` // "none", "monthly"
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// Available returns how many units remain under this quota.
func (q *Quota) Available() int64 {
	if q.Limit <= 0 {
		return 0
	}
	avail := q.Limit - q.CurrentUsage
	if avail < 0 {
		return 0
	}
	return avail
}

// CanAllocate checks if allocating 'delta' units is permitted under quota.
func (q *Quota) CanAllocate(delta int64) bool {
	if delta <= 0 {
		return true
	}
	return q.CurrentUsage+delta <= q.Limit
}

// QuotaSet maps metric types to quota objects for a tenant.
type QuotaSet map[MetricType]*Quota

// ResourceSpec holds allocation requirements for an instance or volume mutation.
type ResourceSpec struct {
	Instances int   `json:"instances"`
	VCPU      int   `json:"vcpu"`
	MemoryMB  int64 `json:"memoryMb"`
	StorageMB int64 `json:"storageMb"`
	IPv4      int   `json:"ipv4"`
	Snapshots int   `json:"snapshots"`
	Backups   int   `json:"backups"`
}

func (s *ResourceSpec) Validate() error {
	if s.Instances < 0 || s.VCPU < 0 || s.MemoryMB < 0 || s.StorageMB < 0 || s.IPv4 < 0 {
		return fmt.Errorf("%w: resource delta cannot contain negative values", ErrInvalidPlanSpec)
	}
	return nil
}
