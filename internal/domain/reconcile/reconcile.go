package reconcile

import (
	"context"
	"time"
)

// DiscrepancySeverity categorizes whether an inconsistency can be safely auto-repaired or requires admin action.
type DiscrepancySeverity string

const (
	SeveritySafeAutoRepair DiscrepancySeverity = "safe_auto_repair"
	SeverityUnsafeAdminRequired DiscrepancySeverity = "unsafe_admin_required"
)

// Discrepancy records an observed mismatch between DB state and physical hypervisor reality.
type Discrepancy struct {
	ResourceType string              `json:"resourceType"` // "instance", "node", "job", "migration", "quota"
	ResourceID   string              `json:"resourceId"`
	ResourceName string              `json:"resourceName,omitempty"`
	Expected     string              `json:"expected"`
	Actual       string              `json:"actual"`
	Severity     DiscrepancySeverity `json:"severity"`
	AutoRepaired bool                `json:"autoRepaired"`
	ActionTaken  string              `json:"actionTaken,omitempty"`
	Reason       string              `json:"reason"`
}

// Report summarizes a reconciliation cycle executed at boot or on-demand.
type Report struct {
	ID                     string        `json:"id"`
	Trigger                string        `json:"trigger"` // "startup", "scheduled", "manual", "disaster_recovery"
	DryRun                 bool          `json:"dryRun"`
	OrphanedInstancesCount int           `json:"orphanedInstancesCount"`
	MissingNodesCount      int           `json:"missingNodesCount"`
	StaleJobsCount         int           `json:"staleJobsCount"`
	AbandonedMigrations    int           `json:"abandonedMigrations"`
	InconsistentQuotas     int           `json:"inconsistentQuotas"`
	TotalDiscrepancies     int           `json:"totalDiscrepancies"`
	RepairedCount          int           `json:"repairedCount"`
	UnsafeCount            int           `json:"unsafeCount"`
	Discrepancies          []Discrepancy `json:"discrepancies"`
	DurationMs             int64         `json:"durationMs"`
	CreatedAt              time.Time     `json:"createdAt"`
}

// Engine defines the contract for control plane state reconciliation.
type Engine interface {
	Reconcile(ctx context.Context, dryRun bool, trigger string) (*Report, error)
	GetLatestReport(ctx context.Context) (*Report, error)
	ListReports(ctx context.Context, limit, offset int) ([]*Report, int, error)
}

// Repository defines persistence for reconciliation audit reports.
type Repository interface {
	SaveReport(ctx context.Context, r *Report) error
	GetLatestReport(ctx context.Context) (*Report, error)
	ListReports(ctx context.Context, limit, offset int) ([]*Report, int, error)
}
