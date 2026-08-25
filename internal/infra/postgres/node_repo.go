package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NodeRepository implements node.NodeRepository using PostgreSQL.
type NodeRepository struct {
	pool *pgxpool.Pool
}

// NewNodeRepository creates a new PostgreSQL node repository.
func NewNodeRepository(pool *pgxpool.Pool) *NodeRepository {
	return &NodeRepository{pool: pool}
}

func (r *NodeRepository) Create(ctx context.Context, n *node.Node) error {
	capsJSON, _ := json.Marshal(n.Capabilities)
	query := `
	INSERT INTO nodes (id, location_id, name, fqdn, status, cert_fingerprint, cpu_cores, memory_bytes, storage_bytes, capabilities, maintenance_mode, created_at, updated_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13);
	`
	_, err := r.pool.Exec(ctx, query,
		n.ID, n.LocationID, n.Name, n.FQDN, string(n.Status), n.CertFingerprint, n.CPUCores, n.MemoryBytes, n.StorageBytes,
		capsJSON, n.MaintenanceMode, n.CreatedAt, n.UpdatedAt,
	)
	return err
}

func (r *NodeRepository) GetByID(ctx context.Context, id string) (*node.Node, error) {
	query := `
	SELECT id, location_id, name, fqdn, status, cert_fingerprint, cpu_cores, memory_bytes, storage_bytes, cpu_overcommit_ratio, memory_overcommit_ratio, capabilities, maintenance_mode, last_heartbeat_at, created_at, updated_at
	FROM nodes WHERE id = $1;
	`
	return r.scanNode(r.pool.QueryRow(ctx, query, id))
}

func (r *NodeRepository) GetByFQDN(ctx context.Context, fqdn string) (*node.Node, error) {
	query := `
	SELECT id, location_id, name, fqdn, status, cert_fingerprint, cpu_cores, memory_bytes, storage_bytes, cpu_overcommit_ratio, memory_overcommit_ratio, capabilities, maintenance_mode, last_heartbeat_at, created_at, updated_at
	FROM nodes WHERE fqdn = $1;
	`
	return r.scanNode(r.pool.QueryRow(ctx, query, fqdn))
}

func (r *NodeRepository) GetByCertFingerprint(ctx context.Context, fingerprint string) (*node.Node, error) {
	query := `
	SELECT id, location_id, name, fqdn, status, cert_fingerprint, cpu_cores, memory_bytes, storage_bytes, cpu_overcommit_ratio, memory_overcommit_ratio, capabilities, maintenance_mode, last_heartbeat_at, created_at, updated_at
	FROM nodes WHERE cert_fingerprint = $1;
	`
	return r.scanNode(r.pool.QueryRow(ctx, query, fingerprint))
}

func (r *NodeRepository) UpdateStatus(ctx context.Context, id string, status node.Status) error {
	query := `UPDATE nodes SET status = $1, updated_at = $2 WHERE id = $3;`
	_, err := r.pool.Exec(ctx, query, string(status), time.Now().UTC(), id)
	return err
}

func (r *NodeRepository) UpdateHealthState(ctx context.Context, id string, status node.Status, reason string) error {
	query := `UPDATE nodes SET status = $1, unhealthy_reason = $2, last_state_change_at = $3, updated_at = $3 WHERE id = $4;`
	_, err := r.pool.Exec(ctx, query, string(status), reason, time.Now().UTC(), id)
	return err
}

func (r *NodeRepository) UpdateDrainMode(ctx context.Context, id string, drainMode bool) error {
	query := `UPDATE nodes SET drain_mode = $1, updated_at = $2 WHERE id = $3;`
	_, err := r.pool.Exec(ctx, query, drainMode, time.Now().UTC(), id)
	return err
}

func (r *NodeRepository) UpdateHeartbeat(ctx context.Context, id string, lastSeen time.Time, caps map[string]interface{}) error {
	capsJSON, _ := json.Marshal(caps)
	query := `UPDATE nodes SET last_heartbeat_at = $1, capabilities = $2, updated_at = $3 WHERE id = $4;`
	_, err := r.pool.Exec(ctx, query, lastSeen, capsJSON, time.Now().UTC(), id)
	return err
}

func (r *NodeRepository) UpdateMaintenanceMode(ctx context.Context, id string, inMaintenance bool) error {
	status := node.StatusOnline
	if inMaintenance {
		status = node.StatusMaintenance
	}
	query := `UPDATE nodes SET maintenance_mode = $1, status = $2, updated_at = $3 WHERE id = $4;`
	_, err := r.pool.Exec(ctx, query, inMaintenance, string(status), time.Now().UTC(), id)
	return err
}

func (r *NodeRepository) Revoke(ctx context.Context, id string) error {
	query := `UPDATE nodes SET status = $1, updated_at = $2 WHERE id = $3;`
	_, err := r.pool.Exec(ctx, query, string(node.StatusRevoked), time.Now().UTC(), id)
	return err
}

func (r *NodeRepository) List(ctx context.Context) ([]*node.Node, error) {
	query := `
	SELECT id, location_id, name, fqdn, status, cert_fingerprint, cpu_cores, memory_bytes, storage_bytes, cpu_overcommit_ratio, memory_overcommit_ratio, capabilities, maintenance_mode, last_heartbeat_at, created_at, updated_at
	FROM nodes ORDER BY name;
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var nodes []*node.Node
	for rows.Next() {
		n, err := r.scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (r *NodeRepository) scanNode(row pgx.Row) (*node.Node, error) {
	var n node.Node
	var statusStr string
	var certFP *string
	var capsJSON []byte

	err := row.Scan(
		&n.ID, &n.LocationID, &n.Name, &n.FQDN, &statusStr, &certFP,
		&n.CPUCores, &n.MemoryBytes, &n.StorageBytes, &n.CPUOvercommitRatio, &n.MemoryOvercommitRatio,
		&capsJSON, &n.MaintenanceMode, &n.LastHeartbeatAt, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, node.ErrNodeNotFound
		}
		return nil, err
	}

	n.Status = node.Status(statusStr)
	if certFP != nil {
		n.CertFingerprint = *certFP
	}
	if len(capsJSON) > 0 {
		_ = json.Unmarshal(capsJSON, &n.Capabilities)
	}
	return &n, nil
}

// ---------------- ENROLLMENT REPOSITORY ----------------

// EnrollmentRepository implements node.EnrollmentRepository using PostgreSQL.
type EnrollmentRepository struct {
	pool *pgxpool.Pool
}

// NewEnrollmentRepository creates a new PostgreSQL enrollment repository.
func NewEnrollmentRepository(pool *pgxpool.Pool) *EnrollmentRepository {
	return &EnrollmentRepository{pool: pool}
}

func (r *EnrollmentRepository) Create(ctx context.Context, secret *node.EnrollmentSecret) error {
	query := `
	INSERT INTO node_enrollment_secrets (id, location_id, secret_hash, node_name_pattern, expires_at, created_by, created_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7);
	`
	_, err := r.pool.Exec(ctx, query,
		secret.ID, secret.LocationID, secret.SecretHash, secret.NodeNamePattern, secret.ExpiresAt, secret.CreatedBy, secret.CreatedAt,
	)
	return err
}

func (r *EnrollmentRepository) GetBySecretHash(ctx context.Context, hash string) (*node.EnrollmentSecret, error) {
	query := `
	SELECT id, location_id, secret_hash, node_name_pattern, expires_at, used_at, used_by_node_id, created_by, created_at
	FROM node_enrollment_secrets WHERE secret_hash = $1;
	`
	var s node.EnrollmentSecret
	var namePattern *string
	err := r.pool.QueryRow(ctx, query, hash).Scan(
		&s.ID, &s.LocationID, &s.SecretHash, &namePattern, &s.ExpiresAt, &s.UsedAt, &s.UsedByNodeID, &s.CreatedBy, &s.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, node.ErrEnrollmentTokenInvalid
		}
		return nil, err
	}
	if namePattern != nil {
		s.NodeNamePattern = *namePattern
	}
	return &s, nil
}

func (r *EnrollmentRepository) MarkUsed(ctx context.Context, id, nodeID string) error {
	now := time.Now().UTC()
	query := `UPDATE node_enrollment_secrets SET used_at = $1, used_by_node_id = $2 WHERE id = $3 AND used_at IS NULL;`
	tag, err := r.pool.Exec(ctx, query, now, nodeID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return node.ErrEnrollmentTokenUsed
	}
	return nil
}

func (r *EnrollmentRepository) ListActive(ctx context.Context) ([]*node.EnrollmentSecret, error) {
	query := `
	SELECT id, location_id, secret_hash, node_name_pattern, expires_at, used_at, used_by_node_id, created_by, created_at
	FROM node_enrollment_secrets
	WHERE used_at IS NULL AND expires_at > NOW()
	ORDER BY created_at DESC;
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var secrets []*node.EnrollmentSecret
	for rows.Next() {
		var s node.EnrollmentSecret
		var namePattern *string
		if err := rows.Scan(&s.ID, &s.LocationID, &s.SecretHash, &namePattern, &s.ExpiresAt, &s.UsedAt, &s.UsedByNodeID, &s.CreatedBy, &s.CreatedAt); err != nil {
			return nil, err
		}
		if namePattern != nil {
			s.NodeNamePattern = *namePattern
		}
		secrets = append(secrets, &s)
	}
	return secrets, nil
}
