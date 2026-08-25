package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MigrationRepository implements domainMigration.MigrationRepository using PostgreSQL.
type MigrationRepository struct {
	pool *pgxpool.Pool
}

// NewMigrationRepository constructs a PostgreSQL Migration Repository.
func NewMigrationRepository(pool *pgxpool.Pool) *MigrationRepository {
	return &MigrationRepository{pool: pool}
}

func (r *MigrationRepository) Create(ctx context.Context, m *domainMigration.Migration) error {
	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	preflightJSON, _ := json.Marshal(m.Preflight)

	query := `
	INSERT INTO workload_migrations (
		id, tenant_id, instance_id, source_node_id, dest_node_id,
		type, status, preflight_data, progress_percent, bytes_transferred, total_bytes,
		error, started_at, completed_at, created_at, updated_at
	) VALUES (
		$1, $2, $3, $4, $5,
		$6, $7, $8, $9, $10, $11,
		$12, $13, $14, $15, $16
	);
	`

	_, err := r.pool.Exec(ctx, query,
		m.ID, m.TenantID, m.InstanceID, m.SourceNodeID, m.DestNodeID,
		string(m.Type), string(m.Status), preflightJSON, m.ProgressPercent, m.BytesTransferred, m.TotalBytes,
		m.Error, m.StartedAt, m.CompletedAt, m.CreatedAt, m.UpdatedAt,
	)
	return err
}

func (r *MigrationRepository) GetByID(ctx context.Context, id string) (*domainMigration.Migration, error) {
	query := `
	SELECT id, tenant_id, instance_id, source_node_id, dest_node_id,
	       type, status, preflight_data, progress_percent, bytes_transferred, total_bytes,
	       error, started_at, completed_at, created_at, updated_at
	FROM workload_migrations WHERE id = $1;
	`
	return r.scanMigration(r.pool.QueryRow(ctx, query, id))
}

func (r *MigrationRepository) GetActiveForInstance(ctx context.Context, instanceID string) (*domainMigration.Migration, error) {
	query := `
	SELECT id, tenant_id, instance_id, source_node_id, dest_node_id,
	       type, status, preflight_data, progress_percent, bytes_transferred, total_bytes,
	       error, started_at, completed_at, created_at, updated_at
	FROM workload_migrations
	WHERE instance_id = $1 AND status NOT IN ('completed', 'failed', 'canceled', 'rolled_back')
	LIMIT 1;
	`
	m, err := r.scanMigration(r.pool.QueryRow(ctx, query, instanceID))
	if err != nil {
		if errors.Is(err, domainMigration.ErrMigrationNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return m, nil
}

func (r *MigrationRepository) List(ctx context.Context, filter domainMigration.MigrationFilter) ([]*domainMigration.Migration, int, error) {
	var whereClauses []string
	var args []interface{}
	idx := 1

	if filter.TenantID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("tenant_id = $%d", idx))
		args = append(args, filter.TenantID)
		idx++
	}
	if filter.InstanceID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("instance_id = $%d", idx))
		args = append(args, filter.InstanceID)
		idx++
	}
	if filter.SourceNodeID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("source_node_id = $%d", idx))
		args = append(args, filter.SourceNodeID)
		idx++
	}
	if filter.DestNodeID != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("dest_node_id = $%d", idx))
		args = append(args, filter.DestNodeID)
		idx++
	}
	if filter.Status != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("status = $%d", idx))
		args = append(args, string(filter.Status))
		idx++
	}

	whereSQL := ""
	if len(whereClauses) > 0 {
		whereSQL = "WHERE " + strings.Join(whereClauses, " AND ")
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM workload_migrations %s;", whereSQL)
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
	SELECT id, tenant_id, instance_id, source_node_id, dest_node_id,
	       type, status, preflight_data, progress_percent, bytes_transferred, total_bytes,
	       error, started_at, completed_at, created_at, updated_at
	FROM workload_migrations
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

	var list []*domainMigration.Migration
	for rows.Next() {
		m, err := r.scanMigration(rows)
		if err != nil {
			return nil, 0, err
		}
		list = append(list, m)
	}

	return list, total, nil
}

func (r *MigrationRepository) UpdateStatus(ctx context.Context, id string, status domainMigration.Status, progress int, errStr string) error {
	now := time.Now().UTC()
	query := `
	UPDATE workload_migrations
	SET status = $1,
	    progress_percent = $2,
	    error = $3,
	    started_at = CASE WHEN $1 = 'transferring' AND started_at IS NULL THEN $4 ELSE started_at END,
	    completed_at = CASE WHEN $1 IN ('completed', 'failed', 'canceled', 'rolled_back') AND completed_at IS NULL THEN $4 ELSE completed_at END,
	    updated_at = $4
	WHERE id = $5;
	`
	_, err := r.pool.Exec(ctx, query, string(status), progress, errStr, now, id)
	return err
}

func (r *MigrationRepository) UpdateProgress(ctx context.Context, id string, progress int, transferred, total int64) error {
	now := time.Now().UTC()
	query := `
	UPDATE workload_migrations
	SET progress_percent = $1,
	    bytes_transferred = $2,
	    total_bytes = $3,
	    updated_at = $4
	WHERE id = $5;
	`
	_, err := r.pool.Exec(ctx, query, progress, transferred, total, now, id)
	return err
}

func (r *MigrationRepository) scanMigration(row pgx.Row) (*domainMigration.Migration, error) {
	var m domainMigration.Migration
	var typeStr, statusStr string
	var preflightBytes []byte
	var errStr *string

	err := row.Scan(
		&m.ID, &m.TenantID, &m.InstanceID, &m.SourceNodeID, &m.DestNodeID,
		&typeStr, &statusStr, &preflightBytes, &m.ProgressPercent, &m.BytesTransferred, &m.TotalBytes,
		&errStr, &m.StartedAt, &m.CompletedAt, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, domainMigration.ErrMigrationNotFound
		}
		return nil, err
	}

	m.Type = domainMigration.Type(typeStr)
	m.Status = domainMigration.Status(statusStr)
	if errStr != nil {
		m.Error = *errStr
	}
	if len(preflightBytes) > 0 {
		_ = json.Unmarshal(preflightBytes, &m.Preflight)
	}

	return &m, nil
}
