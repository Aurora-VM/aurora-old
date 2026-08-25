package backup

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// Common errors in backup and disaster recovery.
var (
	ErrBackupNotFound               = errors.New("backup not found")
	ErrBackupCorrupted              = errors.New("backup failed cryptographic integrity checksum verification")
	ErrCannotDeleteLastGoodBackup   = errors.New("cannot delete the final verified recovery point: system must retain at least one valid recovery point")
	ErrInvalidBackupState           = errors.New("invalid backup lifecycle state for requested operation")
	ErrRestoreFailed                = errors.New("disaster recovery restore failed")
	ErrPolicyNotFound               = errors.New("backup retention policy not found")
	ErrUnauthorizedBackupAccess     = errors.New("unauthorized access to tenant backup artifact")
)

// Type defines full, incremental, or point-in-time backups.
type Type string

const (
	TypeFull         Type = "full"
	TypeIncremental  Type = "incremental"
	TypePointInTime  Type = "point_in_time"
)

// Status represents the lifecycle status of a backup.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusVerified  Status = "verified"
	StatusFailed    Status = "failed"
	StatusExpired   Status = "expired"
	StatusDeleted   Status = "deleted"
)

// Record encapsulates the metadata of an immutable backup artifact.
type Record struct {
	ID                   string                 `json:"id"`
	TenantID             string                 `json:"tenantId"` // "system" for full cluster database backups, or tenant UUID
	ResourceType         string                 `json:"resourceType"` // "database", "instance", "volume", "cluster"
	ResourceID           string                 `json:"resourceId,omitempty"`
	Type                 Type                   `json:"type"`
	Status               Status                 `json:"status"`
	StorageLocation      string                 `json:"storageLocation"` // e.g. "s3://backups/cluster-20260825.enc" or "local://..."
	ChecksumSHA256       string                 `json:"checksumSha256"`
	EncryptionKeyVersion string                 `json:"encryptionKeyVersion"` // Key ID used for AES-GCM-256 envelope encryption
	SizeBytes            int64                  `json:"sizeBytes"`
	RetentionExpiry      *time.Time             `json:"retentionExpiry,omitempty"`
	IsProtectedPoint     bool                   `json:"isProtectedPoint"` // true for last verified good backup
	Metadata             map[string]interface{} `json:"metadata,omitempty"`
	ErrorMessage         string                 `json:"errorMessage,omitempty"`
	CreatedAt            time.Time              `json:"createdAt"`
	CompletedAt          *time.Time             `json:"completedAt,omitempty"`
	VerifiedAt           *time.Time             `json:"verifiedAt,omitempty"`
}

// Resource converts Record into an identity.Resource for authorization checks.
func (r *Record) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "backup",
		ID:      r.ID,
		OwnerID: r.TenantID,
	}
}

// Policy defines automated backup scheduling and retention rules.
type Policy struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	ScheduleCron  string    `json:"scheduleCron"`  // e.g. "0 2 * * *" (daily at 2am)
	RetentionDays int       `json:"retentionDays"` // e.g. 30 days
	MaxBackups    int       `json:"maxBackups"`    // e.g. 14
	StorageTarget string    `json:"storageTarget"` // "s3", "local", "r2"
	Encrypt       bool      `json:"encrypt"`
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// RestoreAction describes an individual idempotent step during disaster recovery.
type RestoreAction struct {
	Step        string                 `json:"step"`
	Target      string                 `json:"target"`
	Description string                 `json:"description"`
	Status      string                 `json:"status"` // "pending", "simulated", "applied", "failed"
	Details     map[string]interface{} `json:"details,omitempty"`
}

// RestorePlan represents a dry-run or live disaster recovery execution.
type RestorePlan struct {
	ID                 string          `json:"id"`
	BackupID           string          `json:"backupId"`
	DryRun             bool            `json:"dryRun"`
	TargetState        string          `json:"targetState"`
	Status             string          `json:"status"` // "pending", "validating", "restoring", "verifying", "completed", "failed"
	Actions            []RestoreAction `json:"actions"`
	DiscrepanciesFound int             `json:"discrepanciesFound"`
	RepairsAttempted   int             `json:"repairsAttempted"`
	RepairsSucceeded   int             `json:"repairsSucceeded"`
	AuditHashVerified  bool            `json:"auditHashVerified"`
	ErrorMessage       string          `json:"errorMessage,omitempty"`
	CreatedAt          time.Time       `json:"createdAt"`
	CompletedAt        *time.Time      `json:"completedAt,omitempty"`
}

// Filter specifies query criteria for backups.
type Filter struct {
	TenantID     string
	ResourceType string
	ResourceID   string
	Type         Type
	Status       Status
	Limit        int
	Offset       int
}

// Repository defines persistence operations for backup records and policies.
type Repository interface {
	Create(ctx context.Context, b *Record) error
	GetByID(ctx context.Context, id string) (*Record, error)
	List(ctx context.Context, filter Filter) ([]*Record, int, error)
	UpdateStatus(ctx context.Context, id string, status Status, checksum string, size int64, err string) error
	SetProtectedPoint(ctx context.Context, id string, protected bool) error
	GetLatestVerified(ctx context.Context, tenantID, resourceType string) (*Record, error)
	CountVerified(ctx context.Context) (int, error)
	Delete(ctx context.Context, id string) error

	// Policy operations
	CreatePolicy(ctx context.Context, p *Policy) error
	GetPolicy(ctx context.Context, id string) (*Policy, error)
	ListPolicies(ctx context.Context) ([]*Policy, error)
	UpdatePolicy(ctx context.Context, p *Policy) error
	DeletePolicy(ctx context.Context, id string) error

	// Restore operations
	SaveRestorePlan(ctx context.Context, plan *RestorePlan) error
	GetRestorePlan(ctx context.Context, id string) (*RestorePlan, error)
	ListRestorePlans(ctx context.Context, limit, offset int) ([]*RestorePlan, int, error)
}
