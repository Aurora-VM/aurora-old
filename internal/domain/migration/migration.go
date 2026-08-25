package migration

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

var (
	ErrMigrationNotFound         = errors.New("migration not found")
	ErrIncompatibleDestination   = errors.New("destination node is incompatible with workload")
	ErrInsufficientCapacity      = errors.New("destination node has insufficient resources")
	ErrPreflightCheckFailed      = errors.New("migration preflight validation failed")
	ErrMigrationInProgress       = errors.New("workload is already undergoing active migration")
	ErrInvalidMigrationState     = errors.New("invalid migration state transition")
)

// Status represents the phase of a workload migration.
type Status string

const (
	StatusPending      Status = "pending"
	StatusValidating   Status = "validating"
	StatusReserving    Status = "reserving"
	StatusTransferring Status = "transferring"
	StatusVerifying    Status = "verifying"
	StatusCompleted    Status = "completed"
	StatusFailed       Status = "failed"
	StatusCanceled     Status = "canceled"
	StatusRolledBack   Status = "rolled_back"
)

// IsTerminal returns true if migration is finished.
func (s Status) IsTerminal() bool {
	return s == StatusCompleted || s == StatusFailed || s == StatusCanceled || s == StatusRolledBack
}

// Type represents the migration mechanism.
type Type string

const (
	TypeLive Type = "live" // Zero-downtime state transfer (e.g. CRIU / Incus live copy)
	TypeCold Type = "cold" // Safe stop -> transfer root volume -> recreate -> start
)

// PreflightValidation represents compatibility checks prior to migration.
type PreflightValidation struct {
	CompatibleArch       bool   `json:"compatibleArch"`
	ImageAvailable       bool   `json:"imageAvailable"`
	StorageAvailable     bool   `json:"storageAvailable"`
	NetworkAvailable     bool   `json:"networkAvailable"`
	DestinationHealthy   bool   `json:"destinationHealthy"`
	CPUCapacityOK        bool   `json:"cpuCapacityOk"`
	MemoryCapacityOK     bool   `json:"memoryCapacityOk"`
	StorageCapacityOK    bool   `json:"storageCapacityOk"`
	FailureReason        string `json:"failureReason,omitempty"`
}

// Passed returns true if all compatibility requirements are met.
func (p *PreflightValidation) Passed() bool {
	return p.CompatibleArch && p.ImageAvailable && p.StorageAvailable &&
		p.NetworkAvailable && p.DestinationHealthy && p.CPUCapacityOK &&
		p.MemoryCapacityOK && p.StorageCapacityOK && p.FailureReason == ""
}

// Migration tracks the progress and state of an instance moving between hypervisor nodes.
type Migration struct {
	ID              string              `json:"id"`
	TenantID        string              `json:"tenantId"`
	InstanceID      string              `json:"instanceId"`
	SourceNodeID    string              `json:"sourceNodeId"`
	DestNodeID      string              `json:"destNodeId"`
	Type            Type                `json:"type"`
	Status          Status              `json:"status"`
	Preflight       PreflightValidation `json:"preflight"`
	ProgressPercent int                 `json:"progressPercent"`
	BytesTransferred int64              `json:"bytesTransferred"`
	TotalBytes      int64               `json:"totalBytes"`
	Error           string              `json:"error,omitempty"`
	StartedAt       *time.Time          `json:"startedAt,omitempty"`
	CompletedAt     *time.Time          `json:"completedAt,omitempty"`
	CreatedAt       time.Time           `json:"createdAt"`
	UpdatedAt       time.Time           `json:"updatedAt"`
}

// Resource converts the migration into an RBAC Resource.
func (m *Migration) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "migration",
		ID:      m.ID,
		OwnerID: m.TenantID,
	}
}

// MigrationFilter represents query parameters for listing migrations.
type MigrationFilter struct {
	TenantID     string
	InstanceID   string
	SourceNodeID string
	DestNodeID   string
	Status       Status
	Limit        int
	Offset       int
}

// MigrationRepository defines the persistence port for workload migrations.
type MigrationRepository interface {
	Create(ctx context.Context, m *Migration) error
	GetByID(ctx context.Context, id string) (*Migration, error)
	GetActiveForInstance(ctx context.Context, instanceID string) (*Migration, error)
	List(ctx context.Context, filter MigrationFilter) ([]*Migration, int, error)
	UpdateStatus(ctx context.Context, id string, status Status, progress int, errStr string) error
	UpdateProgress(ctx context.Context, id string, progress int, transferred, total int64) error
}
