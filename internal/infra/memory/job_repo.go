package memory

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	"github.com/google/uuid"
)

// MemoryJobStore manages in-memory storage for jobs, attempts, and worker leases.
type MemoryJobStore struct {
	mu       sync.RWMutex
	jobs     map[string]*domainJob.Job
	attempts map[string][]*domainJob.JobAttempt // key: jobID
	leases   map[string]*domainJob.WorkerLease  // key: workerID
}

// NewMemoryJobStore initializes a thread-safe in-memory job store.
func NewMemoryJobStore() *MemoryJobStore {
	return &MemoryJobStore{
		jobs:     make(map[string]*domainJob.Job),
		attempts: make(map[string][]*domainJob.JobAttempt),
		leases:   make(map[string]*domainJob.WorkerLease),
	}
}

func (s *MemoryJobStore) Jobs() *MemoryJobRepo         { return &MemoryJobRepo{s: s} }
func (s *MemoryJobStore) Leases() *MemoryWorkerLeaseRepo { return &MemoryWorkerLeaseRepo{s: s} }

// ---------------- JOB REPOSITORY ----------------

type MemoryJobRepo struct{ s *MemoryJobStore }

func (r *MemoryJobRepo) Create(ctx context.Context, job *domainJob.Job) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if job.ID == "" {
		job.ID = uuid.NewString()
	}
	if job.CreatedAt.IsZero() {
		job.CreatedAt = time.Now().UTC()
	}
	if job.Status == "" {
		job.Status = domainJob.StatusPending
	}
	if job.MaxRetries == 0 {
		job.MaxRetries = 3
	}

	copy := *job
	r.s.jobs[job.ID] = &copy
	return nil
}

func (r *MemoryJobRepo) GetByID(ctx context.Context, id string) (*domainJob.Job, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	j, exists := r.s.jobs[id]
	if !exists {
		return nil, domainJob.ErrJobNotFound
	}
	copy := *j
	return &copy, nil
}

func (r *MemoryJobRepo) List(ctx context.Context, filter domainJob.JobFilter) ([]*domainJob.Job, int, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var matched []*domainJob.Job
	for _, j := range r.s.jobs {
		if filter.TenantID != "" && j.TenantID != filter.TenantID {
			continue
		}
		if filter.Type != "" && j.Type != filter.Type {
			continue
		}
		if filter.Status != "" && j.Status != filter.Status {
			continue
		}
		if filter.ResourceType != "" && j.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && j.ResourceID != filter.ResourceID {
			continue
		}

		copy := *j
		matched = append(matched, &copy)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)
	if filter.Offset >= total {
		return []*domainJob.Job{}, total, nil
	}

	end := filter.Offset + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}

	return matched[filter.Offset:end], total, nil
}

func (r *MemoryJobRepo) ClaimNextPending(ctx context.Context, workerID string, leaseDuration time.Duration, types []domainJob.Type) (*domainJob.Job, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	now := time.Now().UTC()
	typeMap := make(map[domainJob.Type]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	for _, j := range r.s.jobs {
		if len(types) > 0 && !typeMap[j.Type] {
			continue
		}

		eligible := false
		if j.Status == domainJob.StatusPending {
			eligible = true
		} else if j.Status == domainJob.StatusRetrying {
			if j.NextRetryAt == nil || now.After(*j.NextRetryAt) || now.Equal(*j.NextRetryAt) {
				eligible = true
			}
		} else if j.Status == domainJob.StatusRunning {
			// Check if previous worker lease expired
			if j.LockedUntil != nil && now.After(*j.LockedUntil) {
				eligible = true
			}
		}

		if eligible {
			until := now.Add(leaseDuration)
			j.Status = domainJob.StatusRunning
			j.LockedByWorker = workerID
			j.LockedUntil = &until
			if j.StartedAt == nil {
				j.StartedAt = &now
			}

			copy := *j
			return &copy, nil
		}
	}

	return nil, nil // No pending jobs
}

func (r *MemoryJobRepo) RenewLease(ctx context.Context, jobID, workerID string, leaseDuration time.Duration) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	j, exists := r.s.jobs[jobID]
	if !exists {
		return domainJob.ErrJobNotFound
	}
	if j.LockedByWorker != workerID {
		return domainJob.ErrJobAlreadyClaimed
	}

	until := time.Now().UTC().Add(leaseDuration)
	j.LockedUntil = &until
	return nil
}

func (r *MemoryJobRepo) UpdateProgress(ctx context.Context, jobID string, progressPercent int) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	j, exists := r.s.jobs[jobID]
	if !exists {
		return domainJob.ErrJobNotFound
	}
	j.ProgressPercent = progressPercent
	return nil
}

func (r *MemoryJobRepo) MarkRunning(ctx context.Context, jobID, workerID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	j, exists := r.s.jobs[jobID]
	if !exists {
		return domainJob.ErrJobNotFound
	}
	now := time.Now().UTC()
	j.Status = domainJob.StatusRunning
	j.LockedByWorker = workerID
	j.StartedAt = &now
	return nil
}

func (r *MemoryJobRepo) MarkSucceeded(ctx context.Context, jobID string, result json.RawMessage) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	j, exists := r.s.jobs[jobID]
	if !exists {
		return domainJob.ErrJobNotFound
	}
	now := time.Now().UTC()
	j.Status = domainJob.StatusSucceeded
	j.Result = result
	j.Error = ""
	j.ProgressPercent = 100
	j.CompletedAt = &now
	j.LockedByWorker = ""
	j.LockedUntil = nil
	return nil
}

func (r *MemoryJobRepo) MarkFailed(ctx context.Context, jobID string, errStr string, retryIn *time.Duration) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	j, exists := r.s.jobs[jobID]
	if !exists {
		return domainJob.ErrJobNotFound
	}
	now := time.Now().UTC()
	j.Error = errStr
	j.RetryCount++

	if retryIn != nil && j.RetryCount <= j.MaxRetries {
		j.Status = domainJob.StatusRetrying
		retryTime := now.Add(*retryIn)
		j.NextRetryAt = &retryTime
		j.LockedByWorker = ""
		j.LockedUntil = nil
	} else {
		j.Status = domainJob.StatusFailed
		j.CompletedAt = &now
		j.LockedByWorker = ""
		j.LockedUntil = nil
	}
	return nil
}

func (r *MemoryJobRepo) MarkCanceled(ctx context.Context, jobID string, reason string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	j, exists := r.s.jobs[jobID]
	if !exists {
		return domainJob.ErrJobNotFound
	}
	if j.Status.IsTerminal() {
		return domainJob.ErrJobCannotCancel
	}

	now := time.Now().UTC()
	j.Status = domainJob.StatusCanceled
	j.Error = reason
	j.CompletedAt = &now
	j.LockedByWorker = ""
	j.LockedUntil = nil
	return nil
}

func (r *MemoryJobRepo) RecordAttempt(ctx context.Context, attempt *domainJob.JobAttempt) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if attempt.ID == "" {
		attempt.ID = uuid.NewString()
	}
	copy := *attempt
	r.s.attempts[attempt.JobID] = append(r.s.attempts[attempt.JobID], &copy)
	return nil
}

func (r *MemoryJobRepo) ReclaimAbandonedJobs(ctx context.Context, cutoff time.Time) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	var reclaimed int64
	for _, j := range r.s.jobs {
		if j.Status == domainJob.StatusRunning && j.LockedUntil != nil && j.LockedUntil.Before(cutoff) {
			j.Status = domainJob.StatusPending
			j.LockedByWorker = ""
			j.LockedUntil = nil
			reclaimed++
		}
	}
	return reclaimed, nil
}

// ---------------- WORKER LEASE REPOSITORY ----------------

type MemoryWorkerLeaseRepo struct{ s *MemoryJobStore }

func (r *MemoryWorkerLeaseRepo) RegisterOrHeartbeat(ctx context.Context, lease *domainJob.WorkerLease) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	copy := *lease
	r.s.leases[lease.WorkerID] = &copy
	return nil
}

func (r *MemoryWorkerLeaseRepo) GetStaleWorkers(ctx context.Context, cutoff time.Time) ([]*domainJob.WorkerLease, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var stale []*domainJob.WorkerLease
	for _, l := range r.s.leases {
		if l.HeartbeatAt.Before(cutoff) {
			copy := *l
			stale = append(stale, &copy)
		}
	}
	return stale, nil
}

func (r *MemoryWorkerLeaseRepo) Deregister(ctx context.Context, workerID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	delete(r.s.leases, workerID)
	return nil
}
