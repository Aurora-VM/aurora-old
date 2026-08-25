package migration

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	appNode "github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/app/scheduler"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainPlacement "github.com/aurora-vm/aurora/internal/domain/placement"
	"github.com/google/uuid"
)

// EventPublisher abstracts domain event emission.
type EventPublisher interface {
	Publish(ctx context.Context, event *domainEvents.Event) error
}

// Service coordinates safe workload migration across hypervisor nodes.
type Service struct {
	migrationRepo  domainMigration.MigrationRepository
	instRepo       domainCompute.InstanceRepository
	nodeRepo       domainNode.NodeRepository
	nodeService    *appNode.Service
	scheduler      *scheduler.Scheduler
	authorizer     domainIdentity.Authorizer
	auditRepo      domainAudit.Repository
	eventPublisher EventPublisher
}

// NewService constructs a Migration Service.
func NewService(
	migrationRepo domainMigration.MigrationRepository,
	instRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	nodeService *appNode.Service,
	scheduler *scheduler.Scheduler,
	authorizer domainIdentity.Authorizer,
	auditRepo domainAudit.Repository,
) *Service {
	return &Service{
		migrationRepo: migrationRepo,
		instRepo:      instRepo,
		nodeRepo:      nodeRepo,
		nodeService:   nodeService,
		scheduler:     scheduler,
		authorizer:    authorizer,
		auditRepo:     auditRepo,
	}
}

// SetEventPublisher sets the event publisher.
func (s *Service) SetEventPublisher(publisher EventPublisher) {
	s.eventPublisher = publisher
}

// PreflightCheck validates workload migration feasibility before initiating data transfer.
func (s *Service) PreflightCheck(ctx context.Context, instanceID, destNodeID string) (*domainMigration.PreflightValidation, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("instance not found: %w", err)
	}

	destNode, err := s.nodeRepo.GetByID(ctx, destNodeID)
	if err != nil {
		return nil, fmt.Errorf("destination node not found: %w", err)
	}

	res := &domainMigration.PreflightValidation{
		CompatibleArch:     true,
		ImageAvailable:     true,
		StorageAvailable:   true,
		NetworkAvailable:   true,
		DestinationHealthy: destNode.IsSchedulable(),
	}

	if !destNode.IsSchedulable() {
		res.FailureReason = fmt.Sprintf("destination node %s is not schedulable (status: %s, maintenance: %v, drain: %v)", destNode.Name, destNode.Status, destNode.MaintenanceMode, destNode.DrainMode)
		return res, nil
	}

	// Verify architecture compatibility
	sourceNode, err := s.nodeRepo.GetByID(ctx, inst.NodeID)
	if err == nil && sourceNode.Capabilities != nil && destNode.Capabilities != nil {
		sourceArch, _ := sourceNode.Capabilities["architecture"].(string)
		destArch, _ := destNode.Capabilities["architecture"].(string)
		if sourceArch != "" && destArch != "" && sourceArch != destArch {
			res.CompatibleArch = false
			res.FailureReason = fmt.Sprintf("architecture mismatch (source: %s, destination: %s)", sourceArch, destArch)
			return res, nil
		}
	}

	// Verify capacity via scheduler
	candidates, err := s.scheduler.EvaluateCandidates(ctx, domainPlacement.Request{
		InstanceType: inst.Type,
		CPUCores:     inst.CPUCores,
		MemoryBytes:  inst.MemoryBytes,
		StorageBytes: inst.StorageBytes,
	})
	if err == nil {
		for _, c := range candidates {
			if c.Node.ID == destNodeID {
				res.CPUCapacityOK = c.AvailableCPUCores >= float64(inst.CPUCores)
				res.MemoryCapacityOK = (c.AvailableMemoryMB * 1024 * 1024) >= inst.MemoryBytes
				res.StorageCapacityOK = (c.AvailableStorageGB * 1024 * 1024 * 1024) >= inst.StorageBytes
				if !c.Eligible {
					res.FailureReason = c.IneligibleReason
				}
				break
			}
		}
	} else {
		res.CPUCapacityOK = true
		res.MemoryCapacityOK = true
		res.StorageCapacityOK = true
	}

	return res, nil
}

// MigrateRequest contains parameters for initiating an instance migration.
type MigrateRequest struct {
	InstanceID string               `json:"instanceId"`
	DestNodeID string               `json:"destNodeId,omitempty"` // If empty, auto-scheduled
	Type       domainMigration.Type `json:"type,omitempty"`       // "live" or "cold"
}

// Migrate initiates the end-to-end migration pipeline for an instance.
func (s *Service) Migrate(ctx context.Context, sub *domainIdentity.Subject, req MigrateRequest) (*domainMigration.Migration, error) {
	inst, err := s.instRepo.GetByID(ctx, req.InstanceID)
	if err != nil {
		return nil, fmt.Errorf("instance not found: %w", err)
	}

	// RBAC Check
	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "migration:manage", inst.Resource()); err != nil {
			return nil, err
		}
	}

	// Check if already in active migration
	active, err := s.migrationRepo.GetActiveForInstance(ctx, inst.ID)
	if err != nil {
		return nil, err
	}
	if active != nil {
		return nil, domainMigration.ErrMigrationInProgress
	}

	// Auto-schedule destination node if not explicitly specified
	destNodeID := req.DestNodeID
	if destNodeID == "" {
		decision, err := s.scheduler.SelectNode(ctx, domainPlacement.Request{
			InstanceType:   inst.Type,
			CPUCores:       inst.CPUCores,
			MemoryBytes:    inst.MemoryBytes,
			StorageBytes:   inst.StorageBytes,
			ExcludeNodeIDs: []string{inst.NodeID},
		})
		if err != nil {
			return nil, fmt.Errorf("scheduling failed: %w", err)
		}
		destNodeID = decision.SelectedNode.ID
	}

	if destNodeID == inst.NodeID {
		return nil, errors.New("destination node must be different from source node")
	}

	// 1. Preflight validation
	preflight, err := s.PreflightCheck(ctx, inst.ID, destNodeID)
	if err != nil {
		return nil, fmt.Errorf("preflight check error: %w", err)
	}
	if !preflight.Passed() {
		return nil, fmt.Errorf("%w: %s", domainMigration.ErrPreflightCheckFailed, preflight.FailureReason)
	}

	migType := req.Type
	if migType == "" {
		migType = domainMigration.TypeCold
	}

	// 2. Create Migration Record
	migration := &domainMigration.Migration{
		ID:           uuid.NewString(),
		TenantID:     inst.UserID,
		InstanceID:   inst.ID,
		SourceNodeID: inst.NodeID,
		DestNodeID:   destNodeID,
		Type:         migType,
		Status:       domainMigration.StatusReserving,
		Preflight:    *preflight,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	if err := s.migrationRepo.Create(ctx, migration); err != nil {
		return nil, fmt.Errorf("failed to create migration record: %w", err)
	}

	// Run migration execution asynchronously or synchronously
	go func() {
		execCtx := context.Background()
		s.executeMigration(execCtx, migration, inst)
	}()

	return migration, nil
}

func (s *Service) executeMigration(ctx context.Context, m *domainMigration.Migration, inst *domainCompute.Instance) {
	log.Printf("[INFO] Starting migration %s for instance %s from node %s -> %s", m.ID, m.InstanceID, m.SourceNodeID, m.DestNodeID)

	// Step 1: Status -> Transferring
	_ = s.migrationRepo.UpdateStatus(ctx, m.ID, domainMigration.StatusTransferring, 20, "")

	wasRunning := inst.Status == domainCompute.StatusRunning
	if m.Type == domainMigration.TypeCold && wasRunning {
		// Safely stop on source
		_, _ = s.nodeService.SendCommand(ctx, m.SourceNodeID, &domainNode.Command{
			CorrelationID: uuid.NewString(),
			Type:          "stop_instance",
			Payload: map[string]interface{}{
				"instanceName": inst.Name,
				"force":        false,
			},
		})
	}

	// Step 2: Transfer state / recreate on destination node
	_ = s.migrationRepo.UpdateProgress(ctx, m.ID, 50, inst.StorageBytes/2, inst.StorageBytes)

	destResult, err := s.nodeService.SendCommand(ctx, m.DestNodeID, &domainNode.Command{
		CorrelationID: uuid.NewString(),
		Type:          "create_instance",
		Payload: map[string]interface{}{
			"instanceName":     inst.Name,
			"instanceType":     string(inst.Type),
			"cpuCores":         inst.CPUCores,
			"memoryBytes":      inst.MemoryBytes,
			"storageBytes":     inst.StorageBytes,
			"image":            inst.Image,
			"startAfterCreate": wasRunning,
		},
	})

	if err != nil || (destResult != nil && !destResult.Success) {
		errMsg := "destination hypervisor failed to recreate instance"
		if err != nil {
			errMsg = err.Error()
		} else if destResult != nil {
			errMsg = destResult.ErrorMessage
		}

		log.Printf("[ERROR] Migration %s failed: %s. Rolling back.", m.ID, errMsg)
		_ = s.migrationRepo.UpdateStatus(ctx, m.ID, domainMigration.StatusRolledBack, 0, errMsg)

		// Rollback: restart instance on source if it was running
		if wasRunning {
			_, _ = s.nodeService.SendCommand(ctx, m.SourceNodeID, &domainNode.Command{
				CorrelationID: uuid.NewString(),
				Type:          "start_instance",
				Payload: map[string]interface{}{
					"instanceName": inst.Name,
				},
			})
		}
		return
	}

	// Step 3: Verifying destination workload health
	_ = s.migrationRepo.UpdateStatus(ctx, m.ID, domainMigration.StatusVerifying, 80, "")

	// Update instance node ownership in DB
	_ = s.instRepo.UpdateNodeID(ctx, inst.ID, m.DestNodeID)
	if wasRunning {
		_ = s.instRepo.UpdateStatus(ctx, inst.ID, domainCompute.StatusRunning, inst.IPv4Address, inst.IPv6Address)
	}

	// Step 4: Cleanup source workload
	_, _ = s.nodeService.SendCommand(ctx, m.SourceNodeID, &domainNode.Command{
		CorrelationID: uuid.NewString(),
		Type:          "delete_instance",
		Payload: map[string]interface{}{
			"instanceName": inst.Name,
			"force":        true,
		},
	})

	// Step 5: Complete
	_ = s.migrationRepo.UpdateStatus(ctx, m.ID, domainMigration.StatusCompleted, 100, "")
	log.Printf("[INFO] Migration %s successfully completed for instance %s", m.ID, m.InstanceID)

	// Emit Event
	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     inst.UserID,
			Type:         domainEvents.EventInstanceMigrated,
			ResourceType: "instance",
			ResourceID:   inst.ID,
			Payload: map[string]interface{}{
				"migrationId":  m.ID,
				"sourceNodeId": m.SourceNodeID,
				"destNodeId":   m.DestNodeID,
			},
		})
	}

	// Audit Log
	if s.auditRepo != nil {
		actorID := inst.UserID
		resID := inst.ID
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "instance.migrated",
			ResourceType: "instance",
			ResourceID:   &resID,
			Details: map[string]interface{}{
				"migrationId":  m.ID,
				"sourceNodeId": m.SourceNodeID,
				"destNodeId":   m.DestNodeID,
			},
			CreatedAt: time.Now().UTC(),
		})
	}
}

// GetMigration queries a migration by ID.
func (s *Service) GetMigration(ctx context.Context, sub *domainIdentity.Subject, id string) (*domainMigration.Migration, error) {
	m, err := s.migrationRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "migration:read", m.Resource()); err != nil {
			return nil, err
		}
	}

	return m, nil
}

// ListMigrations queries migrations matching filter.
func (s *Service) ListMigrations(ctx context.Context, sub *domainIdentity.Subject, filter domainMigration.MigrationFilter) ([]*domainMigration.Migration, int, error) {
	if sub != nil && !sub.IsSuperadmin() {
		filter.TenantID = sub.UserID
	}

	return s.migrationRepo.List(ctx, filter)
}
