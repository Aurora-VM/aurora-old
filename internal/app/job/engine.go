package job

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"os"
	"sync"
	"time"

	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	"github.com/google/uuid"
)

// ProgressReporter allows handlers to stream incremental percentage updates.
type ProgressReporter interface {
	UpdateProgress(ctx context.Context, percent int) error
}

type jobProgressReporter struct {
	engine *Engine
	jobID  string
}

func (r *jobProgressReporter) UpdateProgress(ctx context.Context, percent int) error {
	return r.engine.jobRepo.UpdateProgress(ctx, r.jobID, percent)
}

// Handler defines the execution signature for a registered job type.
type Handler func(ctx context.Context, job *domainJob.Job, reporter ProgressReporter) (json.RawMessage, error)

// EventPublisher abstracts domain event emission.
type EventPublisher interface {
	Publish(ctx context.Context, event *domainEvents.Event) error
}

// Engine coordinates durable distributed job queuing, worker leasing, retry backoffs, and recovery.
type Engine struct {
	jobRepo        domainJob.JobRepository
	leaseRepo      domainJob.WorkerLeaseRepository
	authorizer     domainIdentity.Authorizer
	auditRepo      domainAudit.Repository
	eventPublisher EventPublisher

	workerID       string
	workerCount    int
	leaseDuration  time.Duration
	handlers       map[domainJob.Type]Handler
	mu             sync.RWMutex

	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// NewEngine constructs a distributed job orchestrator.
func NewEngine(
	jobRepo domainJob.JobRepository,
	leaseRepo domainJob.WorkerLeaseRepository,
	authorizer domainIdentity.Authorizer,
	auditRepo domainAudit.Repository,
	workerCount int,
) *Engine {
	if workerCount <= 0 {
		workerCount = 4
	}

	hostname, _ := os.Hostname()
	workerID := fmt.Sprintf("worker-%s-%d-%s", hostname, os.Getpid(), uuid.NewString()[:8])

	ctx, cancel := context.WithCancel(context.Background())

	e := &Engine{
		jobRepo:       jobRepo,
		leaseRepo:     leaseRepo,
		authorizer:    authorizer,
		auditRepo:     auditRepo,
		workerID:      workerID,
		workerCount:   workerCount,
		leaseDuration: 30 * time.Second,
		handlers:      make(map[domainJob.Type]Handler),
		ctx:           ctx,
		cancel:        cancel,
	}

	// Start worker goroutines
	for i := 0; i < e.workerCount; i++ {
		e.wg.Add(1)
		go e.workerLoop(i)
	}

	// Start background lease heartbeater & abandoned job recovery
	e.wg.Add(1)
	go e.heartbeatAndRecoveryLoop()

	return e
}

// SetEventPublisher sets the event bus publisher.
func (e *Engine) SetEventPublisher(publisher EventPublisher) {
	e.eventPublisher = publisher
}

// RegisterHandler binds an execution handler to a job type.
func (e *Engine) RegisterHandler(jobType domainJob.Type, handler Handler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[jobType] = handler
}

// Submit enqueues a new asynchronous job into PostgreSQL.
func (e *Engine) Submit(ctx context.Context, sub *domainIdentity.Subject, job *domainJob.Job) (*domainJob.Job, error) {
	if job.TenantID == "" {
		if sub != nil {
			job.TenantID = sub.UserID
		} else {
			job.TenantID = "system"
		}
	}

	if err := e.jobRepo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("failed to submit job: %w", err)
	}

	// Emit Event
	if e.eventPublisher != nil {
		_ = e.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     job.TenantID,
			Type:         domainEvents.EventType(fmt.Sprintf("job.%s.submitted", job.Type)),
			ResourceType: "job",
			ResourceID:   job.ID,
			ActorID:      job.TenantID,
			Payload: map[string]interface{}{
				"jobType":      job.Type,
				"resourceType": job.ResourceType,
				"resourceId":   job.ResourceID,
			},
		})
	}

	// Audit log
	if e.auditRepo != nil {
		actorID := job.TenantID
		resID := job.ID
		_ = e.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       fmt.Sprintf("job.%s.submitted", job.Type),
			ResourceType: "job",
			ResourceID:   &resID,
			Details: map[string]interface{}{
				"jobType": job.Type,
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	return job, nil
}

// GetJob returns job details with tenant isolation check.
func (e *Engine) GetJob(ctx context.Context, sub *domainIdentity.Subject, id string) (*domainJob.Job, error) {
	job, err := e.jobRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if e.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := e.authorizer.Authorize(ctx, sub, "job:read", job.Resource()); err != nil {
			return nil, err
		}
	}

	return job, nil
}

// ListJobs queries jobs matching filter with tenant enforcement.
func (e *Engine) ListJobs(ctx context.Context, sub *domainIdentity.Subject, filter domainJob.JobFilter) ([]*domainJob.Job, int, error) {
	if sub != nil && !sub.IsSuperadmin() {
		filter.TenantID = sub.UserID
	}

	return e.jobRepo.List(ctx, filter)
}

// CancelJob requests graceful termination of an active or pending job.
func (e *Engine) CancelJob(ctx context.Context, sub *domainIdentity.Subject, id string, reason string) error {
	job, err := e.GetJob(ctx, sub, id)
	if err != nil {
		return err
	}

	if e.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := e.authorizer.Authorize(ctx, sub, "job:manage", job.Resource()); err != nil {
			return err
		}
	}

	if err := e.jobRepo.MarkCanceled(ctx, id, reason); err != nil {
		return err
	}

	// Emit Event
	if e.eventPublisher != nil {
		_ = e.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     job.TenantID,
			Type:         domainEvents.EventType("job.canceled"),
			ResourceType: "job",
			ResourceID:   job.ID,
			Payload: map[string]interface{}{
				"reason": reason,
			},
		})
	}

	return nil
}

// RetryJob resets a failed or canceled job to pending state for re-execution.
func (e *Engine) RetryJob(ctx context.Context, sub *domainIdentity.Subject, id string) (*domainJob.Job, error) {
	job, err := e.GetJob(ctx, sub, id)
	if err != nil {
		return nil, err
	}

	if e.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := e.authorizer.Authorize(ctx, sub, "job:manage", job.Resource()); err != nil {
			return nil, err
		}
	}

	if job.Status != domainJob.StatusFailed && job.Status != domainJob.StatusCanceled {
		return nil, domainJob.ErrJobCannotRetry
	}

	job.Status = domainJob.StatusPending
	job.Error = ""
	job.ProgressPercent = 0
	job.NextRetryAt = nil
	job.LockedByWorker = ""
	job.LockedUntil = nil

	if err := e.jobRepo.Create(ctx, job); err != nil {
		return nil, err
	}

	return job, nil
}

// Close gracefully stops the engine and waits for all active workers to finish.
func (e *Engine) Close() {
	e.cancel()
	e.wg.Wait()

	if e.leaseRepo != nil {
		_ = e.leaseRepo.Deregister(context.Background(), e.workerID)
	}
}

func (e *Engine) workerLoop(workerNum int) {
	defer e.wg.Done()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.processNextJob()
		}
	}
}

func (e *Engine) processNextJob() {
	e.mu.RLock()
	var registeredTypes []domainJob.Type
	for t := range e.handlers {
		registeredTypes = append(registeredTypes, t)
	}
	e.mu.RUnlock()

	if len(registeredTypes) == 0 {
		return
	}

	job, err := e.jobRepo.ClaimNextPending(e.ctx, e.workerID, e.leaseDuration, registeredTypes)
	if err != nil || job == nil {
		return
	}

	e.executeJob(job)
}

func (e *Engine) executeJob(job *domainJob.Job) {
	e.mu.RLock()
	handler, exists := e.handlers[job.Type]
	e.mu.RUnlock()

	if !exists {
		_ = e.jobRepo.MarkFailed(e.ctx, job.ID, fmt.Sprintf("no handler registered for job type %s", job.Type), nil)
		return
	}

	// Record Attempt Start
	attempt := &domainJob.JobAttempt{
		ID:            uuid.NewString(),
		JobID:         job.ID,
		AttemptNumber: job.RetryCount + 1,
		WorkerID:      e.workerID,
		StartedAt:     time.Now().UTC(),
	}

	// Start lease renewal goroutine
	renewCtx, renewCancel := context.WithCancel(e.ctx)
	defer renewCancel()

	go func() {
		renewTicker := time.NewTicker(e.leaseDuration / 2)
		defer renewTicker.Stop()
		for {
			select {
			case <-renewCtx.Done():
				return
			case <-renewTicker.C:
				_ = e.jobRepo.RenewLease(context.Background(), job.ID, e.workerID, e.leaseDuration)
			}
		}
	}()

	reporter := &jobProgressReporter{engine: e, jobID: job.ID}

	// Execute with panic safety
	var result json.RawMessage
	var execErr error

	func() {
		defer func() {
			if r := recover(); r != nil {
				execErr = fmt.Errorf("panic in job handler: %v", r)
				log.Printf("[ERROR] Job %s panic: %v", job.ID, r)
			}
		}()
		result, execErr = handler(e.ctx, job, reporter)
	}()

	finishedAt := time.Now().UTC()
	attempt.FinishedAt = &finishedAt

	if execErr != nil {
		attempt.Error = execErr.Error()
		_ = e.jobRepo.RecordAttempt(context.Background(), attempt)

		// Calculate exponential backoff with jitter
		retryIn := e.calculateBackoff(job.RetryCount + 1)
		_ = e.jobRepo.MarkFailed(context.Background(), job.ID, execErr.Error(), &retryIn)

		if e.eventPublisher != nil {
			_ = e.eventPublisher.Publish(context.Background(), &domainEvents.Event{
				TenantID:     job.TenantID,
				Type:         domainEvents.EventType("job.failed"),
				ResourceType: "job",
				ResourceID:   job.ID,
				Payload: map[string]interface{}{
					"error": execErr.Error(),
				},
			})
		}
	} else {
		_ = e.jobRepo.RecordAttempt(context.Background(), attempt)
		_ = e.jobRepo.MarkSucceeded(context.Background(), job.ID, result)

		if e.eventPublisher != nil {
			_ = e.eventPublisher.Publish(context.Background(), &domainEvents.Event{
				TenantID:     job.TenantID,
				Type:         domainEvents.EventType("job.succeeded"),
				ResourceType: "job",
				ResourceID:   job.ID,
			})
		}
	}
}

func (e *Engine) calculateBackoff(attempt int) time.Duration {
	// Attempt 1: ~5s, Attempt 2: ~30s, Attempt 3: ~2m, Attempt 4: ~10m, Attempt 5: ~30m
	baseDelays := []time.Duration{
		5 * time.Second,
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		30 * time.Minute,
	}

	idx := attempt - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(baseDelays) {
		idx = len(baseDelays) - 1
	}

	base := baseDelays[idx]
	// Add ±20% jitter
	jitter := float64(base) * (0.8 + 0.4*rand.Float64())
	return time.Duration(math.Round(jitter))
}

func (e *Engine) heartbeatAndRecoveryLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	hostname, _ := os.Hostname()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			now := time.Now().UTC()
			if e.leaseRepo != nil {
				_ = e.leaseRepo.RegisterOrHeartbeat(context.Background(), &domainJob.WorkerLease{
					WorkerID:    e.workerID,
					Hostname:    hostname,
					PID:         os.Getpid(),
					Status:      "active",
					HeartbeatAt: now,
					ExpiresAt:   now.Add(15 * time.Second),
					CreatedAt:   now,
				})
			}

			// Reclaim abandoned jobs locked by dead workers
			staleCutoff := now.Add(-e.leaseDuration)
			reclaimed, err := e.jobRepo.ReclaimAbandonedJobs(context.Background(), staleCutoff)
			if err == nil && reclaimed > 0 {
				log.Printf("[INFO] Reclaimed %d abandoned job(s) from expired worker leases", reclaimed)
			}
		}
	}
}
