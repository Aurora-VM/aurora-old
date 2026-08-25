package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SIEMRepository implements audit.SIEMRepository with PostgreSQL.
type SIEMRepository struct {
	pool *pgxpool.Pool
}

func NewSIEMRepository(pool *pgxpool.Pool) *SIEMRepository {
	return &SIEMRepository{pool: pool}
}

func (r *SIEMRepository) Create(ctx context.Context, d *audit.SIEMDestination) error {
	query := `
		INSERT INTO siem_destinations (
			id, name, type, target, auth_token, format, enabled, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9
		) RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	return r.pool.QueryRow(ctx, query,
		d.ID, d.Name, string(d.Type), d.Target, d.AuthToken, string(d.Format), d.Enabled, now, now,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
}

func (r *SIEMRepository) GetByID(ctx context.Context, id string) (*audit.SIEMDestination, error) {
	query := `
		SELECT id, name, type, target, auth_token, format, enabled, created_at, updated_at
		FROM siem_destinations WHERE id = $1
	`
	var d audit.SIEMDestination
	var typeStr, formatStr string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.Name, &typeStr, &d.Target, &d.AuthToken, &formatStr, &d.Enabled, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, audit.ErrSIEMDestinationNotFound
		}
		return nil, err
	}

	d.Type = audit.SIEMType(typeStr)
	d.Format = audit.SIEMFormat(formatStr)
	return &d, nil
}

func (r *SIEMRepository) List(ctx context.Context) ([]*audit.SIEMDestination, error) {
	query := `
		SELECT id, name, type, target, auth_token, format, enabled, created_at, updated_at
		FROM siem_destinations
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*audit.SIEMDestination
	for rows.Next() {
		var d audit.SIEMDestination
		var typeStr, formatStr string
		if err := rows.Scan(
			&d.ID, &d.Name, &typeStr, &d.Target, &d.AuthToken, &formatStr, &d.Enabled, &d.CreatedAt, &d.UpdatedAt,
		); err != nil {
			return nil, err
		}
		d.Type = audit.SIEMType(typeStr)
		d.Format = audit.SIEMFormat(formatStr)
		result = append(result, &d)
	}

	return result, rows.Err()
}

func (r *SIEMRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM siem_destinations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return audit.ErrSIEMDestinationNotFound
	}
	return nil
}
