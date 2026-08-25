package evacuation

import (
	"context"
	"fmt"
	"log"
	"time"

	appMigration "github.com/aurora-vm/aurora/internal/app/migration"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
)

// EvacuationResult summarizes the batch evacuation of a hypervisor node.
type EvacuationResult struct {
	NodeID          string                       `json:"nodeId"`
	TotalWorkloads  int                          `json:"totalWorkloads"`
	MigratedCount   int                          `json:"migratedCount"`
	FailedCount     int                          `json:"failedCount"`
	Migrations      []*domainMigration.Migration `json:"migrations"`
	Errors          []string                     `json:"errors,omitempty"`
}

// Service manages node draining and automated workload evacuation.
type Service struct {
	nodeRepo         domainNode.NodeRepository
	instRepo         domainCompute.InstanceRepository
	migrationService *appMigration.Service
	authorizer       domainIdentity.Authorizer
	auditRepo        domainAudit.Repository
	eventPublisher   appMigration.EventPublisher
}

// NewService constructs an Evacuation Service.
func NewService(
	nodeRepo domainNode.NodeRepository,
	instRepo domainCompute.InstanceRepository,
	migrationService *appMigration.Service,
	authorizer domainIdentity.Authorizer,
	auditRepo domainAudit.Repository,
) *Service {
	return &Service{
		nodeRepo:         nodeRepo,
		instRepo:         instRepo,
		migrationService: migrationService,
		authorizer:       authorizer,
		auditRepo:        auditRepo,
	}
}

// SetEventPublisher sets the event publisher.
func (s *Service) SetEventPublisher(publisher appMigration.EventPublisher) {
	s.eventPublisher = publisher
}

// DrainNode toggles the drain mode on a hypervisor node.
func (s *Service) DrainNode(ctx context.Context, sub *domainIdentity.Subject, nodeID string, drain bool) error {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return fmt.Errorf("node not found: %w", err)
	}

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "node:drain", n.Resource()); err != nil {
			return err
		}
	}

	if err := s.nodeRepo.UpdateDrainMode(ctx, nodeID, drain); err != nil {
		return err
	}

	// Audit log
	if s.auditRepo != nil {
		actorID := sub.UserID
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "node.drain_mode_updated",
			ResourceType: "node",
			ResourceID:   &nodeID,
			Details: map[string]interface{}{
				"drainMode": drain,
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	return nil
}

// EvacuateNode drains a hypervisor node and moves all running and stopped workloads to healthy target nodes.
func (s *Service) EvacuateNode(ctx context.Context, sub *domainIdentity.Subject, nodeID string, destNodeID string) (*EvacuationResult, error) {
	n, err := s.nodeRepo.GetByID(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("node not found: %w", err)
	}

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "node:evacuate", n.Resource()); err != nil {
			return nil, err
		}
	}

	// 1. Mark node as draining
	_ = s.nodeRepo.UpdateDrainMode(ctx, nodeID, true)

	// 2. Discover all hosted instances
	instances, err := s.instRepo.ListByNode(ctx, nodeID)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances for evacuation: %w", err)
	}

	result := &EvacuationResult{
		NodeID:         nodeID,
		TotalWorkloads: len(instances),
	}

	log.Printf("[INFO] Evacuating %d instance(s) from node %s", len(instances), nodeID)

	for _, inst := range instances {
		if inst.Status == domainCompute.StatusDeleted {
			continue
		}

		mig, err := s.migrationService.Migrate(ctx, sub, appMigration.MigrateRequest{
			InstanceID: inst.ID,
			DestNodeID: destNodeID,
			Type:       domainMigration.TypeCold,
		})

		if err != nil {
			log.Printf("[WARN] Failed to evacuate instance %s: %v", inst.ID, err)
			result.FailedCount++
			result.Errors = append(result.Errors, fmt.Sprintf("Instance %s (%s): %v", inst.Name, inst.ID, err))
		} else {
			result.MigratedCount++
			result.Migrations = append(result.Migrations, mig)
		}
	}

	// Emit Event
	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     "system",
			Type:         domainEvents.EventType("node.evacuated"),
			ResourceType: "node",
			ResourceID:   nodeID,
			Payload: map[string]interface{}{
				"totalWorkloads": result.TotalWorkloads,
				"migratedCount":  result.MigratedCount,
				"failedCount":    result.FailedCount,
			},
		})
	}

	return result, nil
}
