package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domainJob "github.com/aurora-vm/aurora/internal/domain/job"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// JobRepository implements domainJob.JobRepository using PostgreSQL.
type JobRepository struct {
	pool *pgxpool.Pool
}

// NewJobRepository constructs a PostgreSQL Job Repository.
func NewJobRepository(pool *pgxpool.Pool) *JobRepository {
	return &JobRepository{pool: pool}
}

func (r *JobRepository) Create(ctx context.Context, job *domainJob.Job) error {
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

	query := `
	INSERT INTO jobs (
		id, tenant_id, type, resource_type, resource_id, status,
		payload, result, error, retry_count, max_retries, next_retry_at,
		locked_by_worker, locked_until, progress_percent, created_at, started_at, completed_at
	) VALUES (
		$1, $2, $3, $4, $5, $6,
		$7, $8, $9, $10, $11, $12,
		$13, $14, $15, $16, $17, $18
	);
	`

	_, err := r.pool.Exec(ctx, query,
		job.ID, job.TenantID, string(job.Type), job.ResourceType, job.ResourceID, string(job.Status),
		job.Payload, job.Result, job.Error, job.RetryCount, job.MaxRetries, job.NextRetryAt,
		job.LockedByWorker, job.LockedUntil, job.ProgressPercent, job.CreatedAt, job.StartedAt, job.CompletedAt,
	)
	return err
}

func (r *JobRepository) GetByID(ctx context.Context, id string) (*domainJob.Job, error) {
	query := `
	SELECT id, tenant_id, type, resource_type, resource_id, status,
	       payload, result, error, retry_count, max_retries, next_retry_at,
	       locked_by_worker, locked_until, progress_percent, created_at, started_at, completed_at
	FROM jobs WHERE id = $1;
	`
	return r.scanJob(r.pool.QueryRow(ctx, query, id))
}

func (r *JobRepository) List(ctx context.Context, filter domainJob.JobFilter) ([]*domainJob.Job, int, error) {
	var whereClauses []string
	var args []interface{}
	idx := 1

	if filter.TenantID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("tenant_id = $%d", idx))
		args = append(args, filter.TenantID)
		idx++
	}
	if filter.Type != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("type = $%d", idx))
		args = append(args, string(filter.Type))
		idx++
	}
	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(filter.Status))
		idx++
	}
	if filter.ResourceType != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("resource_type = $%d", idx))
		args = append(args, filter.ResourceType)
		idx++
	}
	if filter.ResourceID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("resource_id = $%d", idx))
		args = append(args, filter.ResourceID)
		idx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM jobs %s;", whereSQL)
	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset

	query := fmt.Sprintf(`
	SELECT id, tenant_id, type, resource_type, resource_id, status,
	       payload, result, error, retry_count, max_retries, next_retry_at,
	       locked_by_worker, locked_until, progress_percent, created_at, started_at, completed_at
	FROM jobs
	%s
	ORDER BY created_at DESC
	LIMIT $%d OFFSET $%d;
	`, whereSQL, idx, idx+1)

	args = append(args, limit, offset)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var jobs []*domainJob.Job
	for rows.Next() {
		j, err := r.scanJob(rows)
		if err != nil {
			return nil, 0, err
		}
		jobs = append(jobs, j)
	}

	return jobs, total, nil
}

func (r *JobRepository) ClaimNextPending(ctx context.Context, workerID string, leaseDuration time.Duration, types []domainJob.Type) (*domainJob.Job, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var typeStrings []string
	for _, t := range types {
		typeStrings = append(typeStrings, string(t))
	}

	var typeFilterSQL string
	var args []interface{}
	if len(typeStrings) > 0 {
		typeFilterSQL = "AND type = ANY($1)"
		args = append(args, typeStrings)
	}

	selectQuery := fmt.Sprintf(`
	SELECT id FROM jobs
	WHERE (
		status = 'pending'
		OR (status = 'retrying' AND next_retry_at <= NOW())
		OR (status = 'running' AND locked_until < NOW())
	)
	%s
	ORDER BY created_at ASC
	FOR UPDATE SKIP LOCKED
	LIMIT 1;
	`, typeFilterSQL)

	var jobID string
	err = tx.QueryRow(ctx, selectQuery, args...).Scan(&jobID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil // No pending jobs
		}
		return nil, err
	}

	now := time.Now().UTC()
	lockedUntil := now.Add(leaseDuration)

	updateQuery := `
	UPDATE jobs
	SET status = 'running',
	    locked_by_worker = $1,
	    locked_until = $2,
	    started_at = COALESCE(started_at, $3)
	WHERE id = $4
	RETURNING id, tenant_id, type, resource_type, resource_id, status,
	          payload, result, error, retry_count, max_retries, next_retry_at,
	          locked_by_worker, locked_until, progress_percent, created_at, started_at, completed_at;
	`

	job, err := r.scanJob(tx.QueryRow(ctx, updateQuery, workerID, lockedUntil, now, jobID))
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return job, nil
}

func (r *JobRepository) RenewLease(ctx context.Context, jobID, workerID string, leaseDuration time.Duration) error {
	until := time.Now().UTC().Add(leaseDuration)
	query := `
	UPDATE jobs
	SET locked_until = $1
	WHERE id = $2 AND locked_by_worker = $3;
	`
	cmd, err := r.pool.Exec(ctx, query, until, jobID, workerID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domainJob.ErrJobAlreadyClaimed
	}
	return nil
}

func (r *JobRepository) UpdateProgress(ctx context.Context, jobID string, progressPercent int) error {
	query := `UPDATE jobs SET progress_percent = $1 WHERE id = $2;`
	_, err := r.pool.Exec(ctx, query, progressPercent, jobID)
	return err
}

func (r *JobRepository) MarkRunning(ctx context.Context, jobID, workerID string) error {
	query := `UPDATE jobs SET status = 'running', locked_by_worker = $1, started_at = COALESCE(started_at, NOW()) WHERE id = $2;`
	_, err := r.pool.Exec(ctx, query, workerID, jobID)
	return err
}

func (r *JobRepository) MarkSucceeded(ctx context.Context, jobID string, result json.RawMessage) error {
	query := `
	UPDATE jobs
	SET status = 'succeeded',
	    result = $1,
	    error = NULL,
	    progress_percent = 100,
	    completed_at = NOW(),
	    locked_by_worker = NULL,
	    locked_until = NULL
	WHERE id = $2;
	`
	_, err := r.pool.Exec(ctx, query, result, jobID)
	return err
}

func (r *JobRepository) MarkFailed(ctx context.Context, jobID string, errStr string, retryIn *time.Duration) error {
	now := time.Now().UTC()
	if retryIn != nil {
		retryTime := now.Add(*retryIn)
		query := `
		UPDATE jobs
		SET status = 'retrying',
		    error = $1,
		    retry_count = retry_count + 1,
		    next_retry_at = $2,
		    locked_by_worker = NULL,
		    locked_until = NULL
		WHERE id = $3;
		`
		_, err := r.pool.Exec(ctx, query, errStr, retryTime, jobID)
		return err
	}

	query := `
	UPDATE jobs
	SET status = 'failed',
	    error = $1,
	    retry_count = retry_count + 1,
	    completed_at = NOW(),
	    locked_by_worker = NULL,
	    locked_until = NULL
	WHERE id = $2;
	`
	_, err := r.pool.Exec(ctx, query, errStr, jobID)
	return err
}

func (r *JobRepository) MarkCanceled(ctx context.Context, jobID string, reason string) error {
	query := `
	UPDATE jobs
	SET status = 'canceled',
	    error = $1,
	    completed_at = NOW(),
	    locked_by_worker = NULL,
	    locked_until = NULL
	WHERE id = $2 AND status NOT IN ('succeeded', 'failed', 'canceled');
	`
	cmd, err := r.pool.Exec(ctx, query, reason, jobID)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return domainJob.ErrJobCannotCancel
	}
	return nil
}

func (r *JobRepository) RecordAttempt(ctx context.Context, attempt *domainJob.JobAttempt) error {
	if attempt.ID == "" {
		attempt.ID = uuid.NewString()
	}
	if attempt.StartedAt.IsZero() {
		attempt.StartedAt = time.Now().UTC()
	}

	query := `
	INSERT INTO job_attempts (id, job_id, attempt_number, worker_id, error, started_at, finished_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7);
	`
	_, err := r.pool.Exec(ctx, query, attempt.ID, attempt.JobID, attempt.AttemptNumber, attempt.WorkerID, attempt.Error, attempt.StartedAt, attempt.FinishedAt)
	return err
}

func (r *JobRepository) ReclaimAbandonedJobs(ctx context.Context, cutoff time.Time) (int64, error) {
	query := `
	UPDATE jobs
	SET status = 'pending',
	    locked_by_worker = NULL,
	    locked_until = NULL
	WHERE status = 'running' AND locked_until < $1;
	`
	cmd, err := r.pool.Exec(ctx, query, cutoff)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func (r *JobRepository) scanJob(row pgx.Row) (*domainJob.Job, error) {
	var j domainJob.Job
	var typeStr, statusStr string
	var resType, resID, errStr, workerStr *string
	var payloadBytes, resultBytes []byte

	err := row.Scan(
		&j.ID, &j.TenantID, &typeStr, &resType, &resID, &statusStr,
		&payloadBytes, &resultBytes, &errStr, &j.RetryCount, &j.MaxRetries, &j.NextRetryAt,
		&workerStr, &j.LockedUntil, &j.ProgressPercent, &j.CreatedAt, &j.StartedAt, &j.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainJob.ErrJobNotFound
		}
		return nil, err
	}

	j.Type = domainJob.Type(typeStr)
	j.Status = domainJob.Status(statusStr)
	if resType != nil {
		j.ResourceType = *resType
	}
	if resID != nil {
		j.ResourceID = *resID
	}
	if errStr != nil {
		j.Error = *errStr
	}
	if workerStr != nil {
		j.LockedByWorker = *workerStr
	}
	if len(payloadBytes) > 0 {
		j.Payload = payloadBytes
	}
	if len(resultBytes) > 0 {
		j.Result = resultBytes
	}

	return &j, nil
}

// ---------------- WORKER LEASE REPOSITORY ----------------

type WorkerLeaseRepository struct {
	pool *pgxpool.Pool
}

// NewWorkerLeaseRepository constructs a PostgreSQL WorkerLeaseRepository.
func NewWorkerLeaseRepository(pool *pgxpool.Pool) *WorkerLeaseRepository {
	return &WorkerLeaseRepository{pool: pool}
}

func (r *WorkerLeaseRepository) RegisterOrHeartbeat(ctx context.Context, lease *domainJob.WorkerLease) error {
	query := `
	INSERT INTO worker_leases (worker_id, hostname, pid, status, heartbeat_at, expires_at, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7)
	ON CONFLICT (worker_id) DO UPDATE
	SET heartbeat_at = EXCLUDED.heartbeat_at,
	    expires_at = EXCLUDED.expires_at,
	    status = EXCLUDED.status;
	`
	_, err := r.pool.Exec(ctx, query,
		lease.WorkerID, lease.Hostname, lease.PID, lease.Status, lease.HeartbeatAt, lease.ExpiresAt, lease.CreatedAt,
	)
	return err
}

func (r *WorkerLeaseRepository) GetStaleWorkers(ctx context.Context, cutoff time.Time) ([]*domainJob.WorkerLease, error) {
	query := `
	SELECT worker_id, hostname, pid, status, heartbeat_at, expires_at, created_at
	FROM worker_leases
	WHERE expires_at < $1;
	`
	rows, err := r.pool.Query(ctx, query, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var leases []*domainJob.WorkerLease
	for rows.Next() {
		var l domainJob.WorkerLease
		if err := rows.Scan(&l.WorkerID, &l.Hostname, &l.PID, &l.Status, &l.HeartbeatAt, &l.ExpiresAt, &l.CreatedAt); err != nil {
			return nil, err
		}
		leases = append(leases, &l)
	}
	return leases, nil
}

func (r *WorkerLeaseRepository) Deregister(ctx context.Context, workerID string) error {
	query := `DELETE FROM worker_leases WHERE worker_id = $1;`
	_, err := r.pool.Exec(ctx, query, workerID)
	return err
}
