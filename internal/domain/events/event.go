package events

import (
	"context"
	"errors"
	"time"
)

var (
	ErrEventNotFound     = errors.New("event not found")
	ErrInvalidEventType  = errors.New("invalid event type")
	ErrEventQueueFull    = errors.New("event queue full")
	ErrEventBusClosed    = errors.New("event bus is closed")
	ErrInvalidEventData  = errors.New("invalid event data")
)

// EventType represents standard domain event types across Aurora subsystems.
type EventType string

const (
	// Compute Instance Events
	EventInstanceCreated   EventType = "instance.created"
	EventInstanceDeleted   EventType = "instance.deleted"
	EventInstanceStarted   EventType = "instance.started"
	EventInstanceStopped   EventType = "instance.stopped"
	EventInstanceRestarted EventType = "instance.restarted"
	EventInstanceResized   EventType = "instance.resized"
	EventInstanceMigrated  EventType = "instance.migrated"
	EventInstanceError     EventType = "instance.error"

	// Storage & Backup Events
	EventBackupCreated   EventType = "backup.created"
	EventBackupVerified  EventType = "backup.verified"
	EventBackupFailed    EventType = "backup.failed"
	EventBackupRestored  EventType = "backup.restored"
	EventBackupDeleted   EventType = "backup.deleted"
	EventSnapshotCreated EventType = "snapshot.created"
	EventSnapshotRestored EventType = "snapshot.restored"
	EventSnapshotDeleted EventType = "snapshot.deleted"
	EventDisasterRecoveryCompleted EventType = "recovery.completed"

	// Key Rotation Events
	EventKeyRotated      EventType = "key.rotated"
	EventKeyRevoked      EventType = "key.revoked"

	// Node Hypervisor Events
	EventNodeEnrolled EventType = "node.enrolled"
	EventNodeOffline  EventType = "node.offline"
	EventNodeOnline   EventType = "node.online"

	// Billing & Quota Events
	EventSubscriptionCreated  EventType = "billing.subscription.created"
	EventSubscriptionChanged  EventType = "billing.subscription.changed"
	EventSubscriptionCanceled EventType = "billing.subscription.canceled"
	EventInvoiceCreated       EventType = "billing.invoice.created"
	EventInvoicePaid          EventType = "billing.invoice.paid"
	EventInvoiceVoided        EventType = "billing.invoice.voided"
	EventQuotaExceeded        EventType = "billing.quota.exceeded"
	EventUsageThresholdReached EventType = "usage.threshold_reached"

	// Audit & Security Events
	EventAuditIntegrityFailure EventType = "audit.integrity_failure"

	// Async Job Events
	EventJobCreated   EventType = "job.created"
	EventJobCompleted EventType = "job.completed"
	EventJobFailed    EventType = "job.failed"

	// Telemetry & Monitoring Events
	EventMonitoringAlert EventType = "monitoring.alert"
)

// Event represents an immutable domain event emitted by any system mutation.
type Event struct {
	ID           string                 `json:"id"`
	TenantID     string                 `json:"tenantId"`
	Type         EventType              `json:"type"`
	ResourceType string                 `json:"resourceType"` // e.g. "instance", "subscription", "node"
	ResourceID   string                 `json:"resourceId"`
	ActorID      string                 `json:"actorId,omitempty"` // UserID or "system"
	Timestamp    time.Time              `json:"timestamp"`
	Payload      map[string]interface{} `json:"payload"`
	Metadata     map[string]string      `json:"metadata,omitempty"`
	Version      string                 `json:"version"` // e.g. "1.0"
}

// EventFilter specifies query options for retrieving event history.
type EventFilter struct {
	TenantID     string     `json:"tenantId,omitempty"`
	Type         EventType  `json:"type,omitempty"`
	ResourceType string     `json:"resourceType,omitempty"`
	ResourceID   string     `json:"resourceId,omitempty"`
	ActorID      string     `json:"actorId,omitempty"`
	StartTime    *time.Time `json:"startTime,omitempty"`
	EndTime      *time.Time `json:"endTime,omitempty"`
	Limit        int        `json:"limit"`
	Offset       int        `json:"offset"`
}

// Repository handles persistence and historical querying of domain events.
type Repository interface {
	Store(ctx context.Context, event *Event) error
	GetByID(ctx context.Context, id string) (*Event, error)
	List(ctx context.Context, filter EventFilter) ([]*Event, int64, error)
}
