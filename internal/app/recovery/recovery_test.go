package recovery_test

import (
	"context"
	"testing"

	appAuthz "github.com/aurora-vm/aurora/internal/app/authz"
	appBackup "github.com/aurora-vm/aurora/internal/app/backup"
	appReconcile "github.com/aurora-vm/aurora/internal/app/reconcile"
	appRecovery "github.com/aurora-vm/aurora/internal/app/recovery"
	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	infraMemory "github.com/aurora-vm/aurora/internal/infra/memory"
	infraStorage "github.com/aurora-vm/aurora/internal/infra/storage"
)

type mockEventPublisher struct{}

func (m *mockEventPublisher) Publish(ctx context.Context, event *domainEvents.Event) error {
	return nil
}

func setupRecoveryCoordinator(t *testing.T) (*appRecovery.Coordinator, *appBackup.Service, *domainIdentity.Subject) {
	t.Helper()
	memStore := infraMemory.NewMemoryStore()
	backupRepo := memStore.Backups()
	reconcileRepo := memStore.Reconcile()
	instRepo := memStore.Instances()
	nodeRepo := memStore.Nodes()
	jobRepo := memStore.Jobs()
	migRepo := memStore.Migrations()
	quotaRepo := memStore.Quotas()
	auditRepo := memStore.Audit()
	authorizer := appAuthz.NewAuthorizer(memStore.Roles())

	memStorage := infraStorage.NewMemoryObjectStorage()
	encStorage, _ := infraStorage.NewEncryptedStorageWrapper(memStorage, []byte("super-secret-32-byte-key-aurora"))
	eventPub := &mockEventPublisher{}

	backupSvc := appBackup.NewService(backupRepo, encStorage, authorizer, auditRepo, eventPub)
	reconcileSvc := appReconcile.NewService(reconcileRepo, instRepo, nodeRepo, jobRepo, migRepo, quotaRepo, authorizer, auditRepo)

	coord := appRecovery.NewCoordinator(backupSvc, reconcileSvc, backupRepo, auditRepo, eventPub, authorizer)

	adminSub := &domainIdentity.Subject{
		UserID: "superadmin-01",
		Roles:  []string{"superadmin"},
		Permissions: []string{"*"},
	}

	return coord, backupSvc, adminSub
}

func TestDisasterRecovery_DryRun_And_Execution(t *testing.T) {
	coord, backupSvc, adminSub := setupRecoveryCoordinator(t)
	ctx := context.Background()

	// 1. Create a verified backup
	backupRec, err := backupSvc.CreateBackup(ctx, adminSub, appBackup.CreateBackupRequest{
		TenantID:     "system",
		ResourceType: "cluster",
		Type:         domainBackup.TypeFull,
		Metadata:     map[string]interface{}{"note": "Pre-disaster test snapshot"},
	})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// 2. Perform Disaster Recovery Dry Run
	dryPlan, err := coord.DryRunRecovery(ctx, adminSub, backupRec.ID)
	if err != nil {
		t.Fatalf("DryRunRecovery failed: %v", err)
	}

	if !dryPlan.DryRun {
		t.Errorf("expected DryRun to be true")
	}
	if dryPlan.Status != "completed" {
		t.Errorf("expected dry run status completed, got %s", dryPlan.Status)
	}
	if len(dryPlan.Actions) == 0 {
		t.Errorf("expected forecast restore actions in dry run plan")
	}

	// 3. Execute Full Disaster Recovery
	livePlan, err := coord.ExecuteRestore(ctx, adminSub, backupRec.ID)
	if err != nil {
		t.Fatalf("ExecuteRestore failed: %v", err)
	}

	if livePlan.DryRun {
		t.Errorf("expected live restore DryRun to be false")
	}
	if livePlan.Status != "completed" {
		t.Errorf("expected live restore status completed, got %s", livePlan.Status)
	}
	if !livePlan.AuditHashVerified {
		t.Errorf("expected post-restore audit ledger hash chain verification to pass")
	}
	if len(livePlan.Actions) < 4 {
		t.Errorf("expected at least 4 executed recovery steps, got %d", len(livePlan.Actions))
	}

	// 4. Query plan by ID
	fetched, err := coord.GetRestorePlan(ctx, adminSub, livePlan.ID)
	if err != nil || fetched.ID != livePlan.ID {
		t.Errorf("failed to retrieve completed restore plan: %v", err)
	}
}
