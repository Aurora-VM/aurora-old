package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainKeyRotation "github.com/aurora-vm/aurora/internal/domain/keyrotation"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KeyRotationRepository implements domainKeyRotation.Repository using PostgreSQL.
type KeyRotationRepository struct {
	pool *pgxpool.Pool
}

func NewKeyRotationRepository(pool *pgxpool.Pool) *KeyRotationRepository {
	return &KeyRotationRepository{pool: pool}
}

func (r *KeyRotationRepository) Save(ctx context.Context, rec *domainKeyRotation.Record) error {
	query := `
		INSERT INTO key_rotations (
			id, type, key_id, status, version, algorithm, description,
			rotated_by, grace_period_expires_at, revoked_at, revocation_reason,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13
		)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			grace_period_expires_at = EXCLUDED.grace_period_expires_at,
			revoked_at = EXCLUDED.revoked_at,
			revocation_reason = EXCLUDED.revocation_reason,
			updated_at = EXCLUDED.updated_at;
	`
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now

	_, err := r.pool.Exec(ctx, query,
		rec.ID, string(rec.Type), rec.KeyID, string(rec.Status), rec.Version,
		rec.Algorithm, rec.Description, rec.RotatedBy, rec.GracePeriodExpiresAt,
		rec.RevokedAt, rec.RevocationReason, rec.CreatedAt, rec.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save key rotation record: %w", err)
	}
	return nil
}

func (r *KeyRotationRepository) GetByID(ctx context.Context, id string) (*domainKeyRotation.Record, error) {
	query := `
		SELECT id, type, key_id, status, version, algorithm, COALESCE(description, ''),
		       rotated_by, grace_period_expires_at, revoked_at, COALESCE(revocation_reason, ''),
		       created_at, updated_at
		FROM key_rotations
		WHERE id = $1;
	`
	var rec domainKeyRotation.Record
	var kType, kStatus string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&rec.ID, &kType, &rec.KeyID, &kStatus, &rec.Version, &rec.Algorithm,
		&rec.Description, &rec.RotatedBy, &rec.GracePeriodExpiresAt, &rec.RevokedAt,
		&rec.RevocationReason, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainKeyRotation.ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to get key rotation %s: %w", id, err)
	}

	rec.Type = domainKeyRotation.KeyType(kType)
	rec.Status = domainKeyRotation.KeyStatus(kStatus)
	return &rec, nil
}

func (r *KeyRotationRepository) GetActive(ctx context.Context, keyType domainKeyRotation.KeyType) (*domainKeyRotation.Record, error) {
	query := `
		SELECT id, type, key_id, status, version, algorithm, COALESCE(description, ''),
		       rotated_by, grace_period_expires_at, revoked_at, COALESCE(revocation_reason, ''),
		       created_at, updated_at
		FROM key_rotations
		WHERE type = $1 AND status IN ('active', 'grace_period')
		ORDER BY version DESC
		LIMIT 1;
	`
	var rec domainKeyRotation.Record
	var kType, kStatus string

	err := r.pool.QueryRow(ctx, query, string(keyType)).Scan(
		&rec.ID, &kType, &rec.KeyID, &kStatus, &rec.Version, &rec.Algorithm,
		&rec.Description, &rec.RotatedBy, &rec.GracePeriodExpiresAt, &rec.RevokedAt,
		&rec.RevocationReason, &rec.CreatedAt, &rec.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainKeyRotation.ErrKeyNotFound
		}
		return nil, fmt.Errorf("failed to get active key for %s: %w", keyType, err)
	}

	rec.Type = domainKeyRotation.KeyType(kType)
	rec.Status = domainKeyRotation.KeyStatus(kStatus)
	return &rec, nil
}

func (r *KeyRotationRepository) List(ctx context.Context, keyType domainKeyRotation.KeyType, limit, offset int) ([]*domainKeyRotation.Record, int, error) {
	baseQuery := `
		SELECT id, type, key_id, status, version, algorithm, COALESCE(description, ''),
		       rotated_by, grace_period_expires_at, revoked_at, COALESCE(revocation_reason, ''),
		       created_at, updated_at
		FROM key_rotations
		WHERE 1=1
	`
	countQuery := `SELECT COUNT(*) FROM key_rotations WHERE 1=1`

	var args []interface{}
	idx := 1

	if keyType != "" {
		clause := fmt.Sprintf(" AND type = $%d", idx)
		baseQuery += clause
		countQuery += clause
		args = append(args, string(keyType))
		idx++
	}

	var total int
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	baseQuery += " ORDER BY created_at DESC"
	if limit > 0 {
		baseQuery += fmt.Sprintf(" LIMIT $%d", idx)
		args = append(args, limit)
		idx++
	}
	if offset > 0 {
		baseQuery += fmt.Sprintf(" OFFSET $%d", idx)
		args = append(args, offset)
		idx++
	}

	rows, err := r.pool.Query(ctx, baseQuery, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var records []*domainKeyRotation.Record
	for rows.Next() {
		var rec domainKeyRotation.Record
		var kType, kStatus string

		if err := rows.Scan(
			&rec.ID, &kType, &rec.KeyID, &kStatus, &rec.Version, &rec.Algorithm,
			&rec.Description, &rec.RotatedBy, &rec.GracePeriodExpiresAt, &rec.RevokedAt,
			&rec.RevocationReason, &rec.CreatedAt, &rec.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}

		rec.Type = domainKeyRotation.KeyType(kType)
		rec.Status = domainKeyRotation.KeyStatus(kStatus)
		records = append(records, &rec)
	}

	return records, total, nil
}

func (r *KeyRotationRepository) UpdateStatus(ctx context.Context, id string, status domainKeyRotation.KeyStatus, reason string) error {
	query := `
		UPDATE key_rotations
		SET status = $2,
		    revocation_reason = CASE WHEN $3 <> '' THEN $3 ELSE revocation_reason END,
		    revoked_at = CASE WHEN $2 = 'revoked' THEN NOW() ELSE revoked_at END,
		    updated_at = NOW()
		WHERE id = $1;
	`
	cmdTag, err := r.pool.Exec(ctx, query, id, string(status), reason)
	if err != nil {
		return fmt.Errorf("failed to update key status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return domainKeyRotation.ErrKeyNotFound
	}
	return nil
}
