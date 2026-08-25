package migration_test

import (
	"context"
	"testing"
	"time"

	appMigration "github.com/aurora-vm/aurora/internal/app/migration"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	appScheduler "github.com/aurora-vm/aurora/internal/app/scheduler"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

type mockAuthorizer struct{}

func (m *mockAuthorizer) Authorize(ctx context.Context, sub *domainIdentity.Subject, action string, res *domainIdentity.Resource) error {
	return nil
}

type mockMigrationStreamSender struct {
	svc *appNode.Service
}

func (s *mockMigrationStreamSender) Send(cmd *domainNode.Command) error {
	go func() {
		time.Sleep(5 * time.Millisecond)
		s.svc.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       true,
			CompletedAt:   time.Now().UTC(),
		})
	}()
	return nil
}

func TestMigrationService_PreflightAndExecution(t *testing.T) {
	memStore := memory.NewMemoryStore()
	migRepo := memStore.Migrations()
	instRepo := memStore.Instances()
	nodeRepo := memStore.Nodes()
	enrollRepo := memStore.Enrollments()

	now := time.Now().UTC()

	// Source Node
	sourceNode := &domainNode.Node{
		ID:              "node-source",
		Name:            "source-hv",
		FQDN:            "source.aurora.local",
		Status:          domainNode.StatusOnline,
		CPUCores:        16,
		MemoryBytes:     64 * 1024 * 1024 * 1024,
		StorageBytes:    500 * 1024 * 1024 * 1024,
		LastHeartbeatAt: &now,
	}
	_ = nodeRepo.Create(context.Background(), sourceNode)

	// Destination Node
	destNode := &domainNode.Node{
		ID:              "node-dest",
		Name:            "dest-hv",
		FQDN:            "dest.aurora.local",
		Status:          domainNode.StatusOnline,
		CPUCores:        32,
		MemoryBytes:     128 * 1024 * 1024 * 1024,
		StorageBytes:    1000 * 1024 * 1024 * 1024,
		LastHeartbeatAt: &now,
	}
	_ = nodeRepo.Create(context.Background(), destNode)

	// Instance on Source Node
	inst := &domainCompute.Instance{
		ID:           "inst-app-1",
		UserID:       "user-1",
		NodeID:       "node-source",
		Name:         "prod-web",
		Type:         domainCompute.TypeContainer,
		Status:       domainCompute.StatusRunning,
		CPUCores:     2,
		MemoryBytes:  4 * 1024 * 1024 * 1024,
		StorageBytes: 20 * 1024 * 1024 * 1024,
		Image:        "ubuntu/24.04",
	}
	_ = instRepo.Create(context.Background(), inst)

	nodeService := appNode.NewService(nodeRepo, enrollRepo, nil, appNode.NewConnectionManager(), nil, "127.0.0.1:8443")
	_ = nodeService.OnStreamConnected(context.Background(), sourceNode.ID, &mockMigrationStreamSender{svc: nodeService})
	_ = nodeService.OnStreamConnected(context.Background(), destNode.ID, &mockMigrationStreamSender{svc: nodeService})

	sched := appScheduler.NewScheduler(nodeRepo, instRepo)

	service := appMigration.NewService(migRepo, instRepo, nodeRepo, nodeService, sched, &mockAuthorizer{}, nil)

	sub := &domainIdentity.Subject{UserID: "user-1", Roles: []string{"admin"}}

	// 1. Preflight Validation Check
	preflight, err := service.PreflightCheck(context.Background(), inst.ID, destNode.ID)
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if !preflight.Passed() {
		t.Fatalf("expected preflight to pass, failed with reason: %s", preflight.FailureReason)
	}

	// 2. Trigger Migration
	mig, err := service.Migrate(context.Background(), sub, appMigration.MigrateRequest{
		InstanceID: inst.ID,
		DestNodeID: destNode.ID,
		Type:       domainMigration.TypeCold,
	})
	if err != nil {
		t.Fatalf("unexpected migration trigger error: %v", err)
	}

	if mig.Status != domainMigration.StatusReserving {
		t.Fatalf("expected initial status reserving, got: %s", mig.Status)
	}

	// Wait for async execution
	time.Sleep(100 * time.Millisecond)

	updatedMig, _ := service.GetMigration(context.Background(), sub, mig.ID)
	if updatedMig.Status != domainMigration.StatusCompleted {
		t.Fatalf("expected completed migration status, got: %s (error: %s)", updatedMig.Status, updatedMig.Error)
	}

	// Verify instance now lives on destination node
	updatedInst, _ := instRepo.GetByID(context.Background(), inst.ID)
	if updatedInst.NodeID != destNode.ID {
		t.Fatalf("expected instance to reside on node %s, got %s", destNode.ID, updatedInst.NodeID)
	}
}
