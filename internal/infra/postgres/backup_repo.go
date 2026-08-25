package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// BackupRepository implements domainBackup.Repository using PostgreSQL.
type BackupRepository struct {
	pool *pgxpool.Pool
}

func NewBackupRepository(pool *pgxpool.Pool) *BackupRepository {
	return &BackupRepository{pool: pool}
}

func (r *BackupRepository) Create(ctx context.Context, b *domainBackup.Record) error {
	query := `
		INSERT INTO backups (
			id, tenant_id, resource_type, resource_id, type, status,
			storage_location, checksum_sha256, encryption_key_version,
			size_bytes, retention_expiry, is_protected_point, metadata,
			error_message, created_at, completed_at, verified_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17
		);
	`
	metaBytes, _ := json.Marshal(b.Metadata)
	if len(metaBytes) == 0 {
		metaBytes = []byte("{}")
	}

	_, err := r.pool.Exec(ctx, query,
		b.ID, b.TenantID, b.ResourceType, b.ResourceID, string(b.Type), string(b.Status),
		b.StorageLocation, b.ChecksumSHA256, b.EncryptionKeyVersion,
		b.SizeBytes, b.RetentionExpiry, b.IsProtectedPoint, metaBytes,
		b.ErrorMessage, b.CreatedAt, b.CompletedAt, b.VerifiedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to create backup record: %w", err)
	}
	return nil
}

func (r *BackupRepository) GetByID(ctx context.Context, id string) (*domainBackup.Record, error) {
	query := `
		SELECT id, tenant_id, resource_type, COALESCE(resource_id, ''), type, status,
		       storage_location, checksum_sha256, encryption_key_version,
		       size_bytes, retention_expiry, is_protected_point, metadata,
		       COALESCE(error_message, ''), created_at, completed_at, verified_at
		FROM backups
		WHERE id = $1;
	`
	var b domainBackup.Record
	var resID, errMsg, bType, bStatus string
	var metaBytes []byte

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&b.ID, &b.TenantID, &b.ResourceType, &resID, &bType, &bStatus,
		&b.StorageLocation, &b.ChecksumSHA256, &b.EncryptionKeyVersion,
		&b.SizeBytes, &b.RetentionExpiry, &b.IsProtectedPoint, &metaBytes,
		&errMsg, &b.CreatedAt, &b.CompletedAt, &b.VerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainBackup.ErrBackupNotFound
		}
		return nil, fmt.Errorf("failed to query backup %s: %w", id, err)
	}

	b.ResourceID = resID
	b.ErrorMessage = errMsg
	b.Type = domainBackup.Type(bType)
	b.Status = domainBackup.Status(bStatus)
	_ = json.Unmarshal(metaBytes, &b.Metadata)

	return &b, nil
}

func (r *BackupRepository) List(ctx context.Context, filter domainBackup.Filter) ([]*domainBackup.Record, int, error) {
	baseQuery := `
		SELECT id, tenant_id, resource_type, COALESCE(resource_id, ''), type, status,
		       storage_location, checksum_sha256, encryption_key_version,
		       size_bytes, retention_expiry, is_protected_point, metadata,
		       COALESCE(error_message, ''), created_at, completed_at, verified_at
		FROM backups
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM backups WHERE 1=1`

	var args []interface{}
	idx := 1

	if filter.TenantID != "" && filter.TenantID != "system" {
		clause := fmt.Sprintf(" AND tenant_id = $%d", idx)
		baseQuery += clause
		countQuery += clause
		args = append(args, filter.TenantID)
		idx++
	}
	if filter.ResourceType != "" {
		clause := fmt.Sprintf(" AND resource_type = $%d", idx)
		baseQuery += clause
		countQuery += clause
		args = append(args, filter.ResourceType)
		idx++
	}
	if filter.ResourceID != "" {
		clause := fmt.Sprintf(" AND resource_id = $%d", idx)
		baseQuery += clause
		countQuery += clause
		args = append(args, filter.ResourceID)
		idx++
	}
	if filter.Type != "" {
		clause := fmt.Sprintf(" AND type = $%d", idx)
		baseQuery += clause
		countQuery += clause
		args = append(args, string(filter.Type))
		idx++
	}
	if filter.Status != "" {
		clause := fmt.Sprintf(" AND status = $%d", idx)
		baseQuery += clause
		countQuery += clause
		args = append(args, string(filter.Status))
		idx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count backups: %w", err)
	}

	baseQuery += " ORDER BY created_at DESC"
	if filter.Limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT $%d", idx)
		args = append(args, filter.Limit)
		idx++
	}
	if filter.Offset > 0 {
		baseQuery += fmt.Sprintf(" OFFSET $%d", idx)
		args = append(args, filter.Offset)
		idx++
	}

	rows, err := r.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list backups: %w", err)
	}
	defer rows.Close()

	var records []*domainBackup.Record
	for rows.Next() {
		var b domainBackup.Record
		var resID, errMsg, bType, bStatus string
		var metaBytes []byte

		if err := rows.Scan(
			&b.ID, &b.TenantID, &b.ResourceType, &resID, &bType, &bStatus,
			&b.StorageLocation, &b.ChecksumSHA256, &b.EncryptionKeyVersion,
			&b.SizeBytes, &b.RetentionExpiry, &b.IsProtectedPoint, &metaBytes,
			&errMsg, &b.CreatedAt, &b.CompletedAt, &b.VerifiedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("failed to scan backup row: %w", err)
		}

		b.ResourceID = resID
		b.ErrorMessage = errMsg
		b.Type = domainBackup.Type(bType)
		b.Status = domainBackup.Status(bStatus)
		_ = json.Unmarshal(metaBytes, &b.Metadata)

		records = append(records, &b)
	}

	return records, total, nil
}

func (r *BackupRepository) UpdateStatus(ctx context.Context, id string, status domainBackup.Status, checksum string, size int64, errStr string) error {
	query := `
		UPDATE backups
		SET status = $2,
		    checksum_sha256 = CASE WHEN $3 <> '' THEN $3 ELSE checksum_sha256 END,
		    size_bytes = CASE WHEN $4 > 0 THEN $4 ELSE size_bytes END,
		    error_message = CASE WHEN $5 <> '' THEN $5 ELSE error_message END,
		    verified_at = CASE WHEN $2 = 'verified' THEN NOW() ELSE verified_at END,
		    completed_at = CASE WHEN $2 IN ('verified', 'failed') THEN NOW() ELSE completed_at END
		WHERE id = $1;
	`
	cmdTag, err := r.pool.Exec(ctx, query, id, string(status), checksum, size, errStr)
	if err != nil {
		return fmt.Errorf("failed to update backup status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domainBackup.ErrBackupNotFound
	}
	return nil
}

func (r *BackupRepository) SetProtectedPoint(ctx context.Context, id string, protected bool) error {
	query := `UPDATE backups SET is_protected_point = $2 WHERE id = $1;`
	cmdTag, err := r.pool.Exec(ctx, query, id, protected)
	if err != nil {
		return fmt.Errorf("failed to set backup protection: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domainBackup.ErrBackupNotFound
	}
	return nil
}

func (r *BackupRepository) GetLatestVerified(ctx context.Context, tenantID, resourceType string) (*domainBackup.Record, error) {
	query := `
		SELECT id, tenant_id, resource_type, COALESCE(resource_id, ''), type, status,
		       storage_location, checksum_sha256, encryption_key_version,
		       size_bytes, retention_expiry, is_protected_point, metadata,
		       COALESCE(error_message, ''), created_at, completed_at, verified_at
		FROM backups
		WHERE status = 'verified'
	`
	var args []interface{}
	idx := 1
	if tenantID != "" {
		query += fmt.Sprintf(" AND tenant_id = $%d", idx)
		args = append(args, tenantID)
		idx++
	}
	if resourceType != "" {
		query += fmt.Sprintf(" AND resource_type = $%d", idx)
		args = append(args, resourceType)
		idx++
	}
	query += " ORDER BY created_at DESC LIMIT 1;"

	var b domainBackup.Record
	var resID, errMsg, bType, bStatus string
	var metaBytes []byte

	err := r.pool.QueryRow(ctx, query, args...).Scan(
		&b.ID, &b.TenantID, &b.ResourceType, &resID, &bType, &bStatus,
		&b.StorageLocation, &b.ChecksumSHA256, &b.EncryptionKeyVersion,
		&b.SizeBytes, &b.RetentionExpiry, &b.IsProtectedPoint, &metaBytes,
		&errMsg, &b.CreatedAt, &b.CompletedAt, &b.VerifiedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainBackup.ErrBackupNotFound
		}
		return nil, fmt.Errorf("failed to get latest verified backup: %w", err)
	}

	b.ResourceID = resID
	b.ErrorMessage = errMsg
	b.Type = domainBackup.Type(bType)
	b.Status = domainBackup.Status(bStatus)
	_ = json.Unmarshal(metaBytes, &b.Metadata)

	return &b, nil
}

func (r *BackupRepository) CountVerified(ctx context.Context) (int, error) {
	var count int
	err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM backups WHERE status = 'verified';").Scan(&count)
	return count, err
}

func (r *BackupRepository) Delete(ctx context.Context, id string) error {
	// 1. Check protection
	b, err := r.GetByID(ctx, id)
	if err != nil {
		return err
	}
	if b.IsProtectedPoint {
		return domainBackup.ErrCannotDeleteLastGoodBackup
	}

	// 2. Check if this is the only verified backup
	var remainingVerified int
	err = r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM backups WHERE status = 'verified' AND id <> $1;", id).Scan(&remainingVerified)
	if err == nil && b.Status == domainBackup.StatusVerified && remainingVerified == 0 {
		return domainBackup.ErrCannotDeleteLastGoodBackup
	}

	query := `DELETE FROM backups WHERE id = $1;`
	cmdTag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete backup: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domainBackup.ErrBackupNotFound
	}
	return nil
}

// Policy operations
func (r *BackupRepository) CreatePolicy(ctx context.Context, p *domainBackup.Policy) error {
	query := `
		INSERT INTO backup_policies (id, name, schedule_cron, retention_days, max_backups, storage_target, encrypt, enabled, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10);
	`
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query, p.ID, p.Name, p.ScheduleCron, p.RetentionDays, p.MaxBackups, p.StorageTarget, p.Encrypt, p.Enabled, p.CreatedAt, p.UpdatedAt)
	return err
}

func (r *BackupRepository) GetPolicy(ctx context.Context, id string) (*domainBackup.Policy, error) {
	query := `SELECT id, name, schedule_cron, retention_days, max_backups, storage_target, encrypt, enabled, created_at, updated_at FROM backup_policies WHERE id = $1;`
	var p domainBackup.Policy
	err := r.pool.QueryRow(ctx, query, id).Scan(&p.ID, &p.Name, &p.ScheduleCron, &p.RetentionDays, &p.MaxBackups, &p.StorageTarget, &p.Encrypt, &p.Enabled, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainBackup.ErrPolicyNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *BackupRepository) ListPolicies(ctx context.Context) ([]*domainBackup.Policy, error) {
	query := `SELECT id, name, schedule_cron, retention_days, max_backups, storage_target, encrypt, enabled, created_at, updated_at FROM backup_policies ORDER BY created_at DESC;`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var policies []*domainBackup.Policy
	for rows.Next() {
		var p domainBackup.Policy
		if err := rows.Scan(&p.ID, &p.Name, &p.ScheduleCron, &p.RetentionDays, &p.MaxBackups, &p.StorageTarget, &p.Encrypt, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, err
		}
		policies = append(policies, &p)
	}
	return policies, nil
}

func (r *BackupRepository) UpdatePolicy(ctx context.Context, p *domainBackup.Policy) error {
	query := `
		UPDATE backup_policies
		SET name = $2, schedule_cron = $3, retention_days = $4, max_backups = $5, storage_target = $6, encrypt = $7, enabled = $8, updated_at = NOW()
		WHERE id = $1;
	`
	cmdTag, err := r.pool.Exec(ctx, query, p.ID, p.Name, p.ScheduleCron, p.RetentionDays, p.MaxBackups, p.StorageTarget, p.Encrypt, p.Enabled)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return domainBackup.ErrPolicyNotFound
	}
	return nil
}

func (r *BackupRepository) DeletePolicy(ctx context.Context, id string) error {
	cmdTag, err := r.pool.Exec(ctx, "DELETE FROM backup_policies WHERE id = $1;", id)
	if err != nil {
		return err
	}
	if cmdTag.RowsAffected() == 0 {
		return domainBackup.ErrPolicyNotFound
	}
	return nil
}

// Restore Plan CRUD
func (r *BackupRepository) SaveRestorePlan(ctx context.Context, plan *domainBackup.RestorePlan) error {
	query := `
		INSERT INTO restore_plans (
			id, backup_id, dry_run, target_state, status, actions,
			discrepancies_found, repairs_attempted, repairs_succeeded,
			audit_hash_verified, error_message, created_at, completed_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			actions = EXCLUDED.actions,
			discrepancies_found = EXCLUDED.discrepancies_found,
			repairs_attempted = EXCLUDED.repairs_attempted,
			repairs_succeeded = EXCLUDED.repairs_succeeded,
			audit_hash_verified = EXCLUDED.audit_hash_verified,
			error_message = EXCLUDED.error_message,
			completed_at = EXCLUDED.completed_at;
	`
	actionsBytes, _ := json.Marshal(plan.Actions)
	if len(actionsBytes) == 0 {
		actionsBytes = []byte("[]")
	}

	_, err := r.pool.Exec(ctx, query,
		plan.ID, plan.BackupID, plan.DryRun, plan.TargetState, plan.Status, actionsBytes,
		plan.DiscrepanciesFound, plan.RepairsAttempted, plan.RepairsSucceeded,
		plan.AuditHashVerified, plan.ErrorMessage, plan.CreatedAt, plan.CompletedAt,
	)
	return err
}

func (r *BackupRepository) GetRestorePlan(ctx context.Context, id string) (*domainBackup.RestorePlan, error) {
	query := `
		SELECT id, backup_id, dry_run, target_state, status, actions,
		       discrepancies_found, repairs_attempted, repairs_succeeded,
		       audit_hash_verified, COALESCE(error_message, ''), created_at, completed_at
		FROM restore_plans
		WHERE id = $1;
	`
	var p domainBackup.RestorePlan
	var actionsBytes []byte

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.BackupID, &p.DryRun, &p.TargetState, &p.Status, &actionsBytes,
		&p.DiscrepanciesFound, &p.RepairsAttempted, &p.RepairsSucceeded,
		&p.AuditHashVerified, &p.ErrorMessage, &p.CreatedAt, &p.CompletedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainBackup.ErrRestoreFailed
		}
		return nil, err
	}

	_ = json.Unmarshal(actionsBytes, &p.Actions)
	return &p, nil
}

func (r *BackupRepository) ListRestorePlans(ctx context.Context, limit, offset int) ([]*domainBackup.RestorePlan, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM restore_plans;").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, backup_id, dry_run, target_state, status, actions,
		       discrepancies_found, repairs_attempted, repairs_succeeded,
		       audit_hash_verified, COALESCE(error_message, ''), created_at, completed_at
		FROM restore_plans
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2;
	`
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var plans []*domainBackup.RestorePlan
	for rows.Next() {
		var p domainBackup.RestorePlan
		var actionsBytes []byte

		if err := rows.Scan(
			&p.ID, &p.BackupID, &p.DryRun, &p.TargetState, &p.Status, &actionsBytes,
			&p.DiscrepanciesFound, &p.RepairsAttempted, &p.RepairsSucceeded,
			&p.AuditHashVerified, &p.ErrorMessage, &p.CreatedAt, &p.CompletedAt,
		); err != nil {
			return nil, 0, err
		}

		_ = json.Unmarshal(actionsBytes, &p.Actions)
		plans = append(plans, &p)
	}

	return plans, total, nil
}
