package job

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

var (
	ErrJobNotFound         = errors.New("job not found")
	ErrJobAlreadyClaimed   = errors.New("job is already claimed by another worker")
	ErrJobCannotCancel     = errors.New("job is in a non-cancellable state")
	ErrJobCannotRetry      = errors.New("job is not eligible for retry")
	ErrInvalidJobState     = errors.New("invalid job state transition")
	ErrMaxRetriesExceeded  = errors.New("maximum job retries exceeded")
)

// Status represents the state machine lifecycle of an asynchronous job.
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusRetrying  Status = "retrying"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
)

// IsTerminal returns true if the job has finished its lifecycle.
func (s Status) IsTerminal() bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCanceled
}

// Type represents the categorized operational task to be executed.
type Type string

const (
	TypeInstanceProvision   Type = "instance.provision"
	TypeInstanceDelete      Type = "instance.delete"
	TypeInstanceResize      Type = "instance.resize"
	TypeInstanceMigrate     Type = "instance.migrate"
	TypeSnapshotCreate      Type = "snapshot.create"
	TypeSnapshotRestore     Type = "snapshot.restore"
	TypeBackupCreate        Type = "backup.create"
	TypeBackupRestore       Type = "backup.restore"
	TypeImageSync           Type = "image.sync"
	TypeImageVerify         Type = "image.verify"
	TypeNodeDrain           Type = "node.drain"
	TypeWorkloadEvacuate    Type = "workload.evacuate"
)

// Job represents a durable asynchronous unit of work coordinated across Aurora control plane nodes.
type Job struct {
	ID              string          `json:"id"`
	TenantID        string          `json:"tenantId"`
	Type            Type            `json:"type"`
	ResourceType    string          `json:"resourceType,omitempty"`
	ResourceID      string          `json:"resourceId,omitempty"`
	Status          Status          `json:"status"`
	Payload         json.RawMessage `json:"payload,omitempty"`
	Result          json.RawMessage `json:"result,omitempty"`
	Error           string          `json:"error,omitempty"`
	RetryCount      int             `json:"retryCount"`
	MaxRetries      int             `json:"maxRetries"`
	NextRetryAt     *time.Time      `json:"nextRetryAt,omitempty"`
	LockedByWorker  string          `json:"lockedByWorker,omitempty"`
	LockedUntil     *time.Time      `json:"lockedUntil,omitempty"`
	CreatedAt       time.Time       `json:"createdAt"`
	StartedAt       *time.Time      `json:"startedAt,omitempty"`
	CompletedAt     *time.Time      `json:"completedAt,omitempty"`
	ProgressPercent int             `json:"progressPercent"`
}

// Resource converts the job into an RBAC Resource for tenant isolation checks.
func (j *Job) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "job",
		ID:      j.ID,
		OwnerID: j.TenantID,
	}
}

// JobAttempt records execution telemetry for each individual retry or run of a job.
type JobAttempt struct {
	ID            string     `json:"id"`
	JobID         string     `json:"jobId"`
	AttemptNumber int        `json:"attemptNumber"`
	WorkerID      string     `json:"workerId"`
	Error         string     `json:"error,omitempty"`
	StartedAt     time.Time  `json:"startedAt"`
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
}

// WorkerLease represents an active distributed worker registration holding execution leases.
type WorkerLease struct {
	WorkerID     string    `json:"workerId"`
	Hostname     string    `json:"hostname"`
	PID          int       `json:"pid"`
	Status       string    `json:"status"` // "active", "draining", "stopped"
	HeartbeatAt  time.Time `json:"heartbeatAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
	CreatedAt    time.Time `json:"createdAt"`
}

// JobFilter specifies criteria for querying historical or active jobs.
type JobFilter struct {
	TenantID     string
	Type         Type
	Status       Status
	ResourceType string
	ResourceID   string
	Limit        int
	Offset       int
}

// JobRepository defines the persistence port for durable jobs and execution attempts.
type JobRepository interface {
	Create(ctx context.Context, job *Job) error
	GetByID(ctx context.Context, id string) (*Job, error)
	List(ctx context.Context, filter JobFilter) ([]*Job, int, error)
	ClaimNextPending(ctx context.Context, workerID string, leaseDuration time.Duration, types []Type) (*Job, error)
	RenewLease(ctx context.Context, jobID, workerID string, leaseDuration time.Duration) error
	UpdateProgress(ctx context.Context, jobID string, progressPercent int) error
	MarkRunning(ctx context.Context, jobID, workerID string) error
	MarkSucceeded(ctx context.Context, jobID string, result json.RawMessage) error
	MarkFailed(ctx context.Context, jobID string, errStr string, retryIn *time.Duration) error
	MarkCanceled(ctx context.Context, jobID string, reason string) error
	RecordAttempt(ctx context.Context, attempt *JobAttempt) error
	ReclaimAbandonedJobs(ctx context.Context, cutoff time.Time) (int64, error)
}

// WorkerLeaseRepository defines distributed heartbeat and coordination ports for workers.
type WorkerLeaseRepository interface {
	RegisterOrHeartbeat(ctx context.Context, lease *WorkerLease) error
	GetStaleWorkers(ctx context.Context, cutoff time.Time) ([]*WorkerLease, error)
	Deregister(ctx context.Context, workerID string) error
}
