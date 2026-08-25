package reconcile_test

import (
	"context"
	"testing"
	"time"

	appAuthz "github.com/aurora-vm/aurora/internal/app/authz"
	appReconcile "github.com/aurora-vm/aurora/internal/app/reconcile"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	infraMemory "github.com/aurora-vm/aurora/internal/infra/memory"
)

func setupReconcileService(t *testing.T) (*appReconcile.Service, *infraMemory.MemoryStore) {
	t.Helper()
	memStore := infraMemory.NewMemoryStore()
	svc := appReconcile.NewService(
		memStore.Reconcile(),
		memStore.Instances(),
		memStore.Nodes(),
		memStore.Jobs(),
		memStore.Migrations(),
		memStore.Quotas(),
		appAuthz.NewAuthorizer(memStore.Roles()),
		memStore.Audit(),
	)
	return svc, memStore
}

func TestReconcile_OrphanedInstance_And_StaleJob_AutoRepair(t *testing.T) {
	svc, store := setupReconcileService(t)
	ctx := context.Background()

	now := time.Now().UTC()
	nodeAlpha := &domainNode.Node{
		ID:              "node-alpha",
		Name:            "node-alpha",
		FQDN:            "node-alpha.aurora.local",
		Status:          domainNode.StatusOnline,
		LastHeartbeatAt: &now,
	}
	_ = store.Nodes().Create(ctx, nodeAlpha)

	// 2. Create Instance on non-existent node-beta -> should be detected as orphaned
	orphanedInst := &domainCompute.Instance{
		ID:           "inst-orphaned-01",
		UserID:       "tenant-01",
		NodeID:       "node-nonexistent",
		Name:         "dead-workload",
		Status:       domainCompute.StatusRunning,
		CPUCores:     2,
		MemoryBytes:  2 * 1024 * 1024 * 1024,
		StorageBytes: 10 * 1024 * 1024 * 1024,
	}
	_ = store.Instances().Create(ctx, orphanedInst)

	// 3. Create a Stale Job whose worker lease expired 2 minutes ago
	pastLease := time.Now().UTC().Add(-2 * time.Minute)
	staleJob := &domainJob.Job{
		ID:             "job-stale-01",
		TenantID:       "tenant-01",
		Type:           "instance.provision",
		Status:         domainJob.StatusRunning,
		LockedUntil:    &pastLease,
		LockedByWorker: "worker-dead",
	}
	_ = store.Jobs().Create(ctx, staleJob)

	// 4. Run Dry Run
	dryReport, err := svc.Reconcile(ctx, true, "test_dry_run")
	if err != nil {
		t.Fatalf("Dry run reconcile failed: %v", err)
	}

	if dryReport.TotalDiscrepancies < 2 {
		t.Errorf("expected at least 2 discrepancies, got %d", dryReport.TotalDiscrepancies)
	}
	if dryReport.RepairedCount != 0 {
		t.Errorf("dry run should not make repairs, got %d repaired", dryReport.RepairedCount)
	}

	// 5. Run Live Auto-Repair
	liveReport, err := svc.Reconcile(ctx, false, "test_live_repair")
	if err != nil {
		t.Fatalf("Live reconcile failed: %v", err)
	}

	if liveReport.RepairedCount < 2 {
		t.Errorf("expected at least 2 auto-repaired discrepancies, got %d", liveReport.RepairedCount)
	}

	// 6. Verify repaired state
	repairedInst, _ := store.Instances().GetByID(ctx, "inst-orphaned-01")
	if repairedInst.Status != domainCompute.StatusError {
		t.Errorf("expected orphaned instance status error, got %s", repairedInst.Status)
	}

	repairedJob, _ := store.Jobs().GetByID(ctx, "job-stale-01")
	if repairedJob.Status != domainJob.StatusPending {
		t.Errorf("expected stale job status reset to pending, got %s", repairedJob.Status)
	}
}
