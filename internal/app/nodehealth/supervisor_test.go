package nodehealth_test

import (
	"context"
	"testing"
	"time"

	appNodeHealth "github.com/aurora-vm/aurora/internal/app/nodehealth"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

func TestNodeHealthSupervisor_HeartbeatStaleDetectionAndRecovery(t *testing.T) {
	memStore := memory.NewMemoryStore()
	nodeRepo := memStore.Nodes()

	// 1. Create healthy node with an old heartbeat (60s ago)
	staleHeartbeat := time.Now().UTC().Add(-60 * time.Second)
	node := &domainNode.Node{
		ID:              "node-flap-1",
		Name:            "node-flapper",
		FQDN:            "flap.aurora.local",
		Status:          domainNode.StatusOnline,
		CPUCores:        8,
		MemoryBytes:     16 * 1024 * 1024 * 1024,
		StorageBytes:    100 * 1024 * 1024 * 1024,
		LastHeartbeatAt: &staleHeartbeat,
	}
	_ = nodeRepo.Create(context.Background(), node)

	supervisor := appNodeHealth.NewSupervisor(nodeRepo, nil, 20*time.Millisecond)
	defer supervisor.Close()

	// Wait for supervisor loop to transition node to unhealthy
	var unhealthyNode *domainNode.Node
	for i := 0; i < 20; i++ {
		time.Sleep(30 * time.Millisecond)
		unhealthyNode, _ = nodeRepo.GetByID(context.Background(), node.ID)
		if unhealthyNode.Status == domainNode.StatusUnhealthy {
			break
		}
	}

	if unhealthyNode.Status != domainNode.StatusUnhealthy {
		t.Fatalf("expected node to transition to unhealthy, got: %s", unhealthyNode.Status)
	}

	// 2. Restore fresh heartbeat
	freshHeartbeat := time.Now().UTC()
	_ = nodeRepo.UpdateHeartbeat(context.Background(), node.ID, freshHeartbeat, nil)

	// Wait for supervisor to restore node to online
	var recoveredNode *domainNode.Node
	for i := 0; i < 20; i++ {
		time.Sleep(30 * time.Millisecond)
		recoveredNode, _ = nodeRepo.GetByID(context.Background(), node.ID)
		if recoveredNode.Status == domainNode.StatusOnline {
			break
		}
	}

	if recoveredNode.Status != domainNode.StatusOnline {
		t.Fatalf("expected node to recover to online, got: %s", recoveredNode.Status)
	}
}
