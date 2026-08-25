package scheduler_test

import (
	"context"
	"testing"
	"time"

	appScheduler "github.com/aurora-vm/aurora/internal/app/scheduler"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainPlacement "github.com/aurora-vm/aurora/internal/domain/placement"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

func TestScheduler_EvaluatesAndSelectsBestNode(t *testing.T) {
	memStore := memory.NewMemoryStore()
	nodeRepo := memStore.Nodes()
	instRepo := memStore.Instances()

	now := time.Now().UTC()

	// Node 1: Healthy with low capacity (2 cores, 4GB RAM)
	node1 := &domainNode.Node{
		ID:                    "node-1",
		Name:                  "node-small",
		FQDN:                  "node1.aurora.local",
		Status:                domainNode.StatusOnline,
		CPUCores:              2,
		MemoryBytes:           4 * 1024 * 1024 * 1024,
		StorageBytes:          100 * 1024 * 1024 * 1024,
		CPUOvercommitRatio:    1.0,
		MemoryOvercommitRatio: 1.0,
		LastHeartbeatAt:       &now,
	}
	_ = nodeRepo.Create(context.Background(), node1)

	// Node 2: Healthy with high capacity (32 cores, 128GB RAM)
	node2 := &domainNode.Node{
		ID:                    "node-2",
		Name:                  "node-large",
		FQDN:                  "node2.aurora.local",
		Status:                domainNode.StatusOnline,
		CPUCores:              32,
		MemoryBytes:           128 * 1024 * 1024 * 1024,
		StorageBytes:          1000 * 1024 * 1024 * 1024,
		CPUOvercommitRatio:    1.0,
		MemoryOvercommitRatio: 1.0,
		LastHeartbeatAt:       &now,
	}
	_ = nodeRepo.Create(context.Background(), node2)

	// Node 3: In Maintenance mode
	node3 := &domainNode.Node{
		ID:              "node-3",
		Name:            "node-maint",
		FQDN:            "node3.aurora.local",
		Status:          domainNode.StatusMaintenance,
		MaintenanceMode: true,
		CPUCores:        64,
		MemoryBytes:     256 * 1024 * 1024 * 1024,
		StorageBytes:    2000 * 1024 * 1024 * 1024,
	}
	_ = nodeRepo.Create(context.Background(), node3)

	// Node 4: Draining
	node4 := &domainNode.Node{
		ID:           "node-4",
		Name:         "node-drain",
		FQDN:         "node4.aurora.local",
		Status:       domainNode.StatusDraining,
		DrainMode:    true,
		CPUCores:     64,
		MemoryBytes:  256 * 1024 * 1024 * 1024,
		StorageBytes: 2000 * 1024 * 1024 * 1024,
	}
	_ = nodeRepo.Create(context.Background(), node4)

	sched := appScheduler.NewScheduler(nodeRepo, instRepo)

	req := domainPlacement.Request{
		InstanceName: "web-app",
		InstanceType: domainCompute.TypeContainer,
		CPUCores:     4,
		MemoryBytes:  8 * 1024 * 1024 * 1024,
		StorageBytes: 20 * 1024 * 1024 * 1024,
	}

	decision, err := sched.SelectNode(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected scheduling error: %v", err)
	}

	// Should select Node 2 because Node 1 has insufficient CPU (2 < 4), Node 3 is in maintenance, Node 4 is draining
	if decision.SelectedNode.ID != "node-2" {
		t.Fatalf("expected node-2 to be selected, got: %s", decision.SelectedNode.ID)
	}
}

func TestScheduler_ArchitectureMismatchExclusion(t *testing.T) {
	memStore := memory.NewMemoryStore()
	nodeRepo := memStore.Nodes()
	instRepo := memStore.Instances()

	now := time.Now().UTC()

	// ARM64 Node
	nodeARM := &domainNode.Node{
		ID:              "node-arm",
		Name:            "node-arm64",
		FQDN:            "arm.aurora.local",
		Status:          domainNode.StatusOnline,
		CPUCores:        16,
		MemoryBytes:     64 * 1024 * 1024 * 1024,
		StorageBytes:    500 * 1024 * 1024 * 1024,
		Capabilities:    map[string]interface{}{"architecture": "aarch64"},
		LastHeartbeatAt: &now,
	}
	_ = nodeRepo.Create(context.Background(), nodeARM)

	sched := appScheduler.NewScheduler(nodeRepo, instRepo)

	req := domainPlacement.Request{
		InstanceName: "x86-workload",
		InstanceType: domainCompute.TypeContainer,
		CPUCores:     2,
		MemoryBytes:  4 * 1024 * 1024 * 1024,
		StorageBytes: 10 * 1024 * 1024 * 1024,
		Architecture: "x86_64", // Explicit x86 requirement
	}

	_, err := sched.SelectNode(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error due to architecture mismatch, got none")
	}
}
