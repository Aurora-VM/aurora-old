package nodehealth

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
)

// EventPublisher abstracts domain event emission.
type EventPublisher interface {
	Publish(ctx context.Context, event *domainEvents.Event) error
}

// Supervisor monitors hypervisor node connectivity, detects stale heartbeats, and manages self-healing state transitions.
type Supervisor struct {
	nodeRepo       domainNode.NodeRepository
	auditRepo      domainAudit.Repository
	eventPublisher EventPublisher

	missedHeartbeats map[string]int // key: nodeID -> consecutive missed check count
	mu               sync.Mutex

	checkInterval time.Duration
	staleLimit    time.Duration
	unhealthyLimit time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSupervisor constructs a node health supervisor.
func NewSupervisor(
	nodeRepo domainNode.NodeRepository,
	auditRepo domainAudit.Repository,
	checkInterval time.Duration,
) *Supervisor {
	if checkInterval <= 0 {
		checkInterval = 5 * time.Second
	}

	ctx, cancel := context.WithCancel(context.Background())

	s := &Supervisor{
		nodeRepo:         nodeRepo,
		auditRepo:        auditRepo,
		missedHeartbeats: make(map[string]int),
		checkInterval:    checkInterval,
		staleLimit:       15 * time.Second,
		unhealthyLimit:   35 * time.Second,
		ctx:              ctx,
		cancel:           cancel,
	}

	s.wg.Add(1)
	go s.supervisionLoop()

	return s
}

// SetEventPublisher sets the event publisher.
func (s *Supervisor) SetEventPublisher(publisher EventPublisher) {
	s.eventPublisher = publisher
}

// Close gracefully stops the health supervisor.
func (s *Supervisor) Close() {
	s.cancel()
	s.wg.Wait()
}

func (s *Supervisor) supervisionLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.evaluateNodes()
		}
	}
}

func (s *Supervisor) evaluateNodes() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	nodes, err := s.nodeRepo.List(ctx)
	if err != nil {
		return
	}

	now := time.Now().UTC()

	s.mu.Lock()
	defer s.mu.Unlock()

	for _, n := range nodes {
		if n.Status == domainNode.StatusRevoked || n.Status == domainNode.StatusEnrolling || n.MaintenanceMode {
			delete(s.missedHeartbeats, n.ID)
			continue
		}

		var lastSeen time.Time
		if n.LastHeartbeatAt != nil {
			lastSeen = *n.LastHeartbeatAt
		}

		age := now.Sub(lastSeen)

		if n.LastHeartbeatAt == nil || age > s.staleLimit {
			s.missedHeartbeats[n.ID]++
			missedCount := s.missedHeartbeats[n.ID]

			if age > s.unhealthyLimit || missedCount >= 3 {
				if n.Status != domainNode.StatusUnhealthy {
					s.transitionNode(ctx, n, domainNode.StatusUnhealthy, fmt.Sprintf("Heartbeat timed out (last seen %s ago)", age.Round(time.Second)))
				}
			} else if missedCount >= 2 {
				if n.Status != domainNode.StatusDegraded && n.Status != domainNode.StatusUnhealthy {
					s.transitionNode(ctx, n, domainNode.StatusDegraded, fmt.Sprintf("Delayed heartbeat (last seen %s ago)", age.Round(time.Second)))
				}
			}
		} else {
			// Heartbeat is fresh
			s.missedHeartbeats[n.ID] = 0
			if n.Status == domainNode.StatusUnhealthy || n.Status == domainNode.StatusDegraded {
				if !n.DrainMode {
					s.transitionNode(ctx, n, domainNode.StatusOnline, "Healthy heartbeats restored")
				}
			}
		}
	}
}

func (s *Supervisor) transitionNode(ctx context.Context, n *domainNode.Node, targetStatus domainNode.Status, reason string) {
	oldStatus := n.Status
	log.Printf("[WARN] Node %s (%s) health state transition: %s -> %s (Reason: %s)", n.Name, n.ID, oldStatus, targetStatus, reason)

	if err := s.nodeRepo.UpdateHealthState(ctx, n.ID, targetStatus, reason); err != nil {
		log.Printf("[ERROR] Failed to update health state for node %s: %v", n.ID, err)
		return
	}

	n.Status = targetStatus
	n.UnhealthyReason = reason

	eventType := domainEvents.EventType(fmt.Sprintf("node.%s", targetStatus))
	if targetStatus == domainNode.StatusOnline && oldStatus != domainNode.StatusOnline {
		eventType = domainEvents.EventType("node.recovered")
	}

	// Emit Event
	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     "system",
			Type:         eventType,
			ResourceType: "node",
			ResourceID:   n.ID,
			Payload: map[string]interface{}{
				"oldStatus": string(oldStatus),
				"newStatus": string(targetStatus),
				"reason":    reason,
			},
		})
	}

	// Audit Log
	if s.auditRepo != nil {
		actorID := "system:supervisor"
		nodeID := n.ID
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       string(eventType),
			ResourceType: "node",
			ResourceID:   &nodeID,
			Details: map[string]interface{}{
				"oldStatus": string(oldStatus),
				"newStatus": string(targetStatus),
				"reason":    reason,
			},
			CreatedAt: time.Now().UTC(),
		})
	}
}
