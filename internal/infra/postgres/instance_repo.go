package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InstanceRepository implements compute.InstanceRepository using PostgreSQL.
type InstanceRepository struct {
	pool *pgxpool.Pool
}

// NewInstanceRepository constructs a PostgreSQL InstanceRepository.
func NewInstanceRepository(pool *pgxpool.Pool) *InstanceRepository {
	return &InstanceRepository{pool: pool}
}

func (r *InstanceRepository) Create(ctx context.Context, inst *compute.Instance) error {
	configJSON, err := json.Marshal(inst.Config)
	if err != nil {
		configJSON = []byte("{}")
	}

	query := `
		INSERT INTO instances (
			id, user_id, node_id, name, type, status,
			cpu_cores, memory_bytes, storage_bytes, image,
			ipv4_address, ipv6_address, config, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15);
	`

	_, err = r.pool.Exec(
		ctx, query,
		inst.ID, inst.UserID, inst.NodeID, inst.Name, string(inst.Type), string(inst.Status),
		inst.CPUCores, inst.MemoryBytes, inst.StorageBytes, inst.Image,
		inst.IPv4Address, inst.IPv6Address, configJSON, inst.CreatedAt, inst.UpdatedAt,
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // Unique violation
			return compute.ErrInstanceAlreadyExists
		}
		return fmt.Errorf("failed to insert instance: %w", err)
	}

	return nil
}

func (r *InstanceRepository) GetByID(ctx context.Context, id string) (*compute.Instance, error) {
	query := `
		SELECT id, user_id, node_id, name, type, status,
		       cpu_cores, memory_bytes, storage_bytes, image,
		       COALESCE(ipv4_address, ''), COALESCE(ipv6_address, ''),
		       config, created_at, updated_at
		FROM instances
		WHERE id = $1;
	`

	var inst compute.Instance
	var typeStr, statusStr string
	var configJSON []byte

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&inst.ID, &inst.UserID, &inst.NodeID, &inst.Name, &typeStr, &statusStr,
		&inst.CPUCores, &inst.MemoryBytes, &inst.StorageBytes, &inst.Image,
		&inst.IPv4Address, &inst.IPv6Address, &configJSON, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, compute.ErrInstanceNotFound
		}
		return nil, fmt.Errorf("failed to query instance by ID: %w", err)
	}

	inst.Type = compute.Type(typeStr)
	inst.Status = compute.Status(statusStr)
	_ = json.Unmarshal(configJSON, &inst.Config)

	return &inst, nil
}

func (r *InstanceRepository) GetByName(ctx context.Context, name string) (*compute.Instance, error) {
	query := `
		SELECT id, user_id, node_id, name, type, status,
		       cpu_cores, memory_bytes, storage_bytes, image,
		       COALESCE(ipv4_address, ''), COALESCE(ipv6_address, ''),
		       config, created_at, updated_at
		FROM instances
		WHERE name = $1;
	`

	var inst compute.Instance
	var typeStr, statusStr string
	var configJSON []byte

	err := r.pool.QueryRow(ctx, query, name).Scan(
		&inst.ID, &inst.UserID, &inst.NodeID, &inst.Name, &typeStr, &statusStr,
		&inst.CPUCores, &inst.MemoryBytes, &inst.StorageBytes, &inst.Image,
		&inst.IPv4Address, &inst.IPv6Address, &configJSON, &inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, compute.ErrInstanceNotFound
		}
		return nil, fmt.Errorf("failed to query instance by name: %w", err)
	}

	inst.Type = compute.Type(typeStr)
	inst.Status = compute.Status(statusStr)
	_ = json.Unmarshal(configJSON, &inst.Config)

	return &inst, nil
}

func (r *InstanceRepository) ListByUser(ctx context.Context, userID string) ([]*compute.Instance, error) {
	query := `
		SELECT id, user_id, node_id, name, type, status,
		       cpu_cores, memory_bytes, storage_bytes, image,
		       COALESCE(ipv4_address, ''), COALESCE(ipv6_address, ''),
		       config, created_at, updated_at
		FROM instances
		WHERE user_id = $1
		ORDER BY created_at DESC;
	`
	return r.queryInstances(ctx, query, userID)
}

func (r *InstanceRepository) ListByNode(ctx context.Context, nodeID string) ([]*compute.Instance, error) {
	query := `
		SELECT id, user_id, node_id, name, type, status,
		       cpu_cores, memory_bytes, storage_bytes, image,
		       COALESCE(ipv4_address, ''), COALESCE(ipv6_address, ''),
		       config, created_at, updated_at
		FROM instances
		WHERE node_id = $1
		ORDER BY created_at DESC;
	`
	return r.queryInstances(ctx, query, nodeID)
}

func (r *InstanceRepository) ListAll(ctx context.Context) ([]*compute.Instance, error) {
	query := `
		SELECT id, user_id, node_id, name, type, status,
		       cpu_cores, memory_bytes, storage_bytes, image,
		       COALESCE(ipv4_address, ''), COALESCE(ipv6_address, ''),
		       config, created_at, updated_at
		FROM instances
		ORDER BY created_at DESC;
	`
	return r.queryInstances(ctx, query)
}

func (r *InstanceRepository) UpdateStatus(ctx context.Context, id string, status compute.Status, ipv4, ipv6 string) error {
	query := `
		UPDATE instances
		SET status = $2,
		    ipv4_address = CASE WHEN $3 <> '' THEN $3 ELSE ipv4_address END,
		    ipv6_address = CASE WHEN $4 <> '' THEN $4 ELSE ipv6_address END,
		    updated_at = $5
		WHERE id = $1;
	`

	cmdTag, err := r.pool.Exec(ctx, query, id, string(status), ipv4, ipv6, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update instance status: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return compute.ErrInstanceNotFound
	}
	return nil
}

func (r *InstanceRepository) UpdateSpec(ctx context.Context, id string, cpu int, memory, storage int64) error {
	query := `
		UPDATE instances
		SET cpu_cores = $2,
		    memory_bytes = $3,
		    storage_bytes = $4,
		    updated_at = $5
		WHERE id = $1;
	`

	cmdTag, err := r.pool.Exec(ctx, query, id, cpu, memory, storage, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update instance spec: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return compute.ErrInstanceNotFound
	}
	return nil
}

func (r *InstanceRepository) UpdateNodeID(ctx context.Context, id string, nodeID string) error {
	query := `
		UPDATE instances
		SET node_id = $2,
		    updated_at = $3
		WHERE id = $1;
	`

	cmdTag, err := r.pool.Exec(ctx, query, id, nodeID, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("failed to update instance node ID: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return compute.ErrInstanceNotFound
	}
	return nil
}

func (r *InstanceRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM instances WHERE id = $1;`
	cmdTag, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete instance: %w", err)
	}
	if cmdTag.RowsAffected() == 0 {
		return compute.ErrInstanceNotFound
	}
	return nil
}

func (r *InstanceRepository) queryInstances(ctx context.Context, query string, args ...interface{}) ([]*compute.Instance, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query instances: %w", err)
	}
	defer rows.Close()

	var instances []*compute.Instance
	for rows.Next() {
		var inst compute.Instance
		var typeStr, statusStr string
		var configJSON []byte

		if err := rows.Scan(
			&inst.ID, &inst.UserID, &inst.NodeID, &inst.Name, &typeStr, &statusStr,
			&inst.CPUCores, &inst.MemoryBytes, &inst.StorageBytes, &inst.Image,
			&inst.IPv4Address, &inst.IPv6Address, &configJSON, &inst.CreatedAt, &inst.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan instance row: %w", err)
		}

		inst.Type = compute.Type(typeStr)
		inst.Status = compute.Status(statusStr)
		_ = json.Unmarshal(configJSON, &inst.Config)

		instances = append(instances, &inst)
	}

	return instances, nil
}
