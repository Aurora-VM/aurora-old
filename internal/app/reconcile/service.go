package reconcile

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainBilling "github.com/aurora-vm/aurora/internal/domain/billing"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainReconcile "github.com/aurora-vm/aurora/internal/domain/reconcile"
	"github.com/google/uuid"
)

// AuditRecorder logs reconciliation events.
type AuditRecorder interface {
	Record(ctx context.Context, log *domainAudit.AuditLog) error
}

// Service executes control-plane state reconciliation and auto-repairs.
type Service struct {
	reconcileRepo domainReconcile.Repository
	instRepo      domainCompute.InstanceRepository
	nodeRepo      domainNode.NodeRepository
	jobRepo       domainJob.JobRepository
	migRepo       domainMigration.MigrationRepository
	quotaRepo     domainBilling.QuotaRepository
	authorizer    domainIdentity.Authorizer
	auditRepo     AuditRecorder
	mu            sync.Mutex
}

func NewService(
	reconcileRepo domainReconcile.Repository,
	instRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	jobRepo domainJob.JobRepository,
	migRepo domainMigration.MigrationRepository,
	quotaRepo domainBilling.QuotaRepository,
	authorizer domainIdentity.Authorizer,
	auditRepo AuditRecorder,
) *Service {
	return &Service{
		reconcileRepo: reconcileRepo,
		instRepo:      instRepo,
		nodeRepo:      nodeRepo,
		jobRepo:       jobRepo,
		migRepo:       migRepo,
		quotaRepo:     quotaRepo,
		authorizer:    authorizer,
		auditRepo:     auditRepo,
	}
}

// Reconcile performs a complete state discrepancy analysis and repairs safe inconsistencies.
func (s *Service) Reconcile(ctx context.Context, dryRun bool, trigger string) (*domainReconcile.Report, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	startTime := time.Now().UTC()
	reportID := uuid.NewString()

	report := &domainReconcile.Report{
		ID:            reportID,
		Trigger:       trigger,
		DryRun:        dryRun,
		CreatedAt:     startTime,
		Discrepancies: make([]domainReconcile.Discrepancy, 0),
	}

	// 1. Fetch live nodes
	nodes, err := s.nodeRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes for reconciliation: %w", err)
	}
	nodeMap := make(map[string]*domainNode.Node)
	for _, n := range nodes {
		nodeMap[n.ID] = n
	}

	// 2. Check Instances for Orphaned / Missing Node state
	instances, err := s.instRepo.ListAll(ctx)
	if err == nil {
		tenantUsageMap := make(map[string]map[string]int64) // tenant -> resource -> count

		for _, inst := range instances {
			if inst.Status == domainCompute.StatusDeleted {
				continue
			}

			// Aggregate active quota usage
			if inst.Status == domainCompute.StatusRunning || inst.Status == domainCompute.StatusStopped {
				if _, ok := tenantUsageMap[inst.UserID]; !ok {
					tenantUsageMap[inst.UserID] = make(map[string]int64)
				}
				tenantUsageMap[inst.UserID]["instances"]++
				tenantUsageMap[inst.UserID]["vcpu"] += int64(inst.CPUCores)
				tenantUsageMap[inst.UserID]["memory_mb"] += inst.MemoryBytes / (1024 * 1024)
				tenantUsageMap[inst.UserID]["storage_mb"] += inst.StorageBytes / (1024 * 1024)
			}

			// Check hosting node
			n, nodeExists := nodeMap[inst.NodeID]
			isOrphaned := !nodeExists || (n.Status == domainNode.StatusOffline && inst.Status == domainCompute.StatusRunning)

			if isOrphaned {
				report.OrphanedInstancesCount++
				report.TotalDiscrepancies++

				disc := domainReconcile.Discrepancy{
					ResourceType: "instance",
					ResourceID:   inst.ID,
					ResourceName: inst.Name,
					Expected:     fmt.Sprintf("node %s online", inst.NodeID),
					Actual:       "node unreachable or offline",
					Severity:     domainReconcile.SeveritySafeAutoRepair,
					Reason:       "instance marked running on unreachable hypervisor host",
				}

				if !dryRun {
					// Safe auto-repair: transition instance status to error/stopped
					_ = s.instRepo.UpdateStatus(ctx, inst.ID, domainCompute.StatusError, "", "")
					disc.AutoRepaired = true
					disc.ActionTaken = "updated status to error"
					report.RepairedCount++
				}
				report.Discrepancies = append(report.Discrepancies, disc)
			}
		}

		// Reconcile Quotas
		if s.quotaRepo != nil && !dryRun {
			for tenantID, usage := range tenantUsageMap {
				quotas, err := s.quotaRepo.ListQuotas(ctx, tenantID)
				if err == nil && quotas != nil {
					for resKey, expectedVal := range usage {
						metric := domainBilling.MetricType(resKey)
						if qItem, ok := quotas[metric]; ok && qItem.CurrentUsage != expectedVal {
							report.InconsistentQuotas++
							report.TotalDiscrepancies++
							disc := domainReconcile.Discrepancy{
								ResourceType: "quota",
								ResourceID:   fmt.Sprintf("%s:%s", tenantID, resKey),
								Expected:     fmt.Sprintf("%d", expectedVal),
								Actual:       fmt.Sprintf("%d", qItem.CurrentUsage),
								Severity:     domainReconcile.SeveritySafeAutoRepair,
								Reason:       "quota usage counter drifted from actual provisioned instances",
							}
							qItem.CurrentUsage = expectedVal
							_ = s.quotaRepo.SetQuota(ctx, qItem)
							disc.AutoRepaired = true
							disc.ActionTaken = "recalculated and updated quota usage"
							report.RepairedCount++
							report.Discrepancies = append(report.Discrepancies, disc)
						}
					}
				}
			}
		}
	}

	// 3. Reconcile Stale / Abandoned Async Jobs
	if s.jobRepo != nil {
		jobs, _, err := s.jobRepo.List(ctx, domainJob.JobFilter{Status: domainJob.StatusRunning, Limit: 500})
		if err == nil {
			for _, j := range jobs {
				// If lockedUntil is expired by more than 30 seconds
				if j.LockedUntil != nil && j.LockedUntil.Before(time.Now().UTC().Add(-30*time.Second)) {
					report.StaleJobsCount++
					report.TotalDiscrepancies++
					disc := domainReconcile.Discrepancy{
						ResourceType: "job",
						ResourceID:   j.ID,
						Expected:     "active worker lease",
						Actual:       fmt.Sprintf("lease expired at %s", j.LockedUntil.Format(time.RFC3339)),
						Severity:     domainReconcile.SeveritySafeAutoRepair,
						Reason:       "background worker lease timed out or worker crashed",
					}

					if !dryRun {
						_, _ = s.jobRepo.ReclaimAbandonedJobs(ctx, time.Now().UTC())
						disc.AutoRepaired = true
						disc.ActionTaken = "reclaimed abandoned job and reset to pending"
						report.RepairedCount++
					}
					report.Discrepancies = append(report.Discrepancies, disc)
				}
			}
		}
	}

	// 4. Reconcile Abandoned Migrations
	if s.migRepo != nil {
		migs, _, err := s.migRepo.List(ctx, domainMigration.MigrationFilter{Status: domainMigration.StatusTransferring, Limit: 100})
		if err == nil {
			for _, m := range migs {
				if m.UpdatedAt.Before(time.Now().UTC().Add(-5 * time.Minute)) {
					report.AbandonedMigrations++
					report.TotalDiscrepancies++
					disc := domainReconcile.Discrepancy{
						ResourceType: "migration",
						ResourceID:   m.ID,
						Expected:     "active migration transfer",
						Actual:       "stalled transfer with no progress for > 5 minutes",
						Severity:     domainReconcile.SeverityUnsafeAdminRequired,
						Reason:       "migration stalled, requires administrative review",
					}
					report.UnsafeCount++
					report.Discrepancies = append(report.Discrepancies, disc)
				}
			}
		}
	}

	report.DurationMs = time.Since(startTime).Milliseconds()

	// Persist report
	_ = s.reconcileRepo.SaveReport(ctx, report)

	// Audit Log
	if s.auditRepo != nil {
		actorID := "system:reconciler"
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "system.reconciled",
			ResourceType: "system",
			ResourceID:   &reportID,
			StatusCode:   200,
			Details: map[string]interface{}{
				"dryRun":             dryRun,
				"trigger":            trigger,
				"totalDiscrepancies": report.TotalDiscrepancies,
				"repairedCount":      report.RepairedCount,
				"unsafeCount":        report.UnsafeCount,
				"durationMs":         report.DurationMs,
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	log.Printf("[INFO] State reconciliation complete (%s, dryRun=%v): %d discrepancies, %d repaired, %d unsafe (took %dms)",
		trigger, dryRun, report.TotalDiscrepancies, report.RepairedCount, report.UnsafeCount, report.DurationMs)

	return report, nil
}

// GetLatestReport returns the most recent reconciliation audit report.
func (s *Service) GetLatestReport(ctx context.Context) (*domainReconcile.Report, error) {
	return s.reconcileRepo.GetLatestReport(ctx)
}

// ListReports returns paginated historical reconciliation reports.
func (s *Service) ListReports(ctx context.Context, limit, offset int) ([]*domainReconcile.Report, int, error) {
	return s.reconcileRepo.ListReports(ctx, limit, offset)
}
