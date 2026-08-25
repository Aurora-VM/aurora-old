package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/storage"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StoragePoolRepository implements storage.StoragePoolRepository with PostgreSQL.
type StoragePoolRepository struct {
	pool *pgxpool.Pool
}

func NewStoragePoolRepository(pool *pgxpool.Pool) *StoragePoolRepository {
	return &StoragePoolRepository{pool: pool}
}

func (r *StoragePoolRepository) Create(ctx context.Context, p *storage.StoragePool) error {
	query := `
		INSERT INTO storage_pools (
			id, node_id, name, driver, total_space_bytes, used_space_bytes, status, config, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	cfgJSON, err := json.Marshal(p.Config)
	if err != nil {
		cfgJSON = []byte("{}")
	}

	err = r.pool.QueryRow(ctx, query,
		p.ID, p.NodeID, p.Name, string(p.Driver), p.TotalSpaceBytes, p.UsedSpaceBytes, string(p.Status), cfgJSON, now, now,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrStoragePoolAlreadyExists
		}
		return err
	}
	return nil
}

func (r *StoragePoolRepository) GetByID(ctx context.Context, id string) (*storage.StoragePool, error) {
	query := `
		SELECT id, node_id, name, driver, total_space_bytes, used_space_bytes, status, config, created_at, updated_at
		FROM storage_pools WHERE id = $1
	`
	return r.scanPool(r.pool.QueryRow(ctx, query, id))
}

func (r *StoragePoolRepository) GetByNodeAndName(ctx context.Context, nodeID, name string) (*storage.StoragePool, error) {
	query := `
		SELECT id, node_id, name, driver, total_space_bytes, used_space_bytes, status, config, created_at, updated_at
		FROM storage_pools WHERE node_id = $1 AND name = $2
	`
	return r.scanPool(r.pool.QueryRow(ctx, query, nodeID, name))
}

func (r *StoragePoolRepository) List(ctx context.Context, nodeID string) ([]*storage.StoragePool, error) {
	query := `
		SELECT id, node_id, name, driver, total_space_bytes, used_space_bytes, status, config, created_at, updated_at
		FROM storage_pools WHERE ($1 = '' OR node_id = $1::uuid)
		ORDER BY name ASC
	`
	rows, err := r.pool.Query(ctx, query, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*storage.StoragePool
	for rows.Next() {
		p, err := r.scanPool(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	return result, rows.Err()
}

func (r *StoragePoolRepository) Update(ctx context.Context, p *storage.StoragePool) error {
	query := `
		UPDATE storage_pools
		SET name = $2, driver = $3, total_space_bytes = $4, used_space_bytes = $5, status = $6, config = $7, updated_at = $8
		WHERE id = $1
	`
	cfgJSON, _ := json.Marshal(p.Config)
	res, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, string(p.Driver), p.TotalSpaceBytes, p.UsedSpaceBytes, string(p.Status), cfgJSON, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return storage.ErrStoragePoolNotFound
	}
	return nil
}

func (r *StoragePoolRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM storage_pools WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return storage.ErrStoragePoolNotFound
	}
	return nil
}

func (r *StoragePoolRepository) scanPool(row pgx.Row) (*storage.StoragePool, error) {
	var p storage.StoragePool
	var driverStr, statusStr string
	var cfgBytes []byte

	err := row.Scan(
		&p.ID, &p.NodeID, &p.Name, &driverStr, &p.TotalSpaceBytes, &p.UsedSpaceBytes, &statusStr, &cfgBytes, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrStoragePoolNotFound
		}
		return nil, err
	}

	p.Driver = storage.DriverType(driverStr)
	p.Status = storage.PoolStatus(statusStr)
	if len(cfgBytes) > 0 {
		_ = json.Unmarshal(cfgBytes, &p.Config)
	}
	return &p, nil
}

// VolumeRepository implements storage.VolumeRepository with PostgreSQL.
type VolumeRepository struct {
	pool *pgxpool.Pool
}

func NewVolumeRepository(pool *pgxpool.Pool) *VolumeRepository {
	return &VolumeRepository{pool: pool}
}

func (r *VolumeRepository) Create(ctx context.Context, v *storage.Volume) error {
	query := `
		INSERT INTO volumes (
			id, user_id, pool_id, instance_id, name, size_bytes, content_type, mount_path, read_only, status, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
		) RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	err := r.pool.QueryRow(ctx, query,
		v.ID, v.UserID, v.PoolID, v.InstanceID, v.Name, v.SizeBytes, string(v.ContentType), v.MountPath, v.ReadOnly, string(v.Status), now, now,
	).Scan(&v.ID, &v.CreatedAt, &v.UpdatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrVolumeAlreadyExists
		}
		return err
	}
	return nil
}

func (r *VolumeRepository) GetByID(ctx context.Context, id string) (*storage.Volume, error) {
	query := `
		SELECT id, user_id, pool_id, instance_id, name, size_bytes, content_type, mount_path, read_only, status, created_at, updated_at
		FROM volumes WHERE id = $1
	`
	return r.scanVolume(r.pool.QueryRow(ctx, query, id))
}

func (r *VolumeRepository) GetByPoolAndName(ctx context.Context, poolID, name string) (*storage.Volume, error) {
	query := `
		SELECT id, user_id, pool_id, instance_id, name, size_bytes, content_type, mount_path, read_only, status, created_at, updated_at
		FROM volumes WHERE pool_id = $1 AND name = $2
	`
	return r.scanVolume(r.pool.QueryRow(ctx, query, poolID, name))
}

func (r *VolumeRepository) ListByUser(ctx context.Context, userID string) ([]*storage.Volume, error) {
	query := `
		SELECT id, user_id, pool_id, instance_id, name, size_bytes, content_type, mount_path, read_only, status, created_at, updated_at
		FROM volumes WHERE ($1 = '' OR user_id = $1::uuid)
		ORDER BY created_at DESC
	`
	return r.queryVolumes(ctx, query, userID)
}

func (r *VolumeRepository) ListByPool(ctx context.Context, poolID string) ([]*storage.Volume, error) {
	query := `
		SELECT id, user_id, pool_id, instance_id, name, size_bytes, content_type, mount_path, read_only, status, created_at, updated_at
		FROM volumes WHERE pool_id = $1
		ORDER BY created_at DESC
	`
	return r.queryVolumes(ctx, query, poolID)
}

func (r *VolumeRepository) ListByInstance(ctx context.Context, instanceID string) ([]*storage.Volume, error) {
	query := `
		SELECT id, user_id, pool_id, instance_id, name, size_bytes, content_type, mount_path, read_only, status, created_at, updated_at
		FROM volumes WHERE instance_id = $1
		ORDER BY created_at DESC
	`
	return r.queryVolumes(ctx, query, instanceID)
}

func (r *VolumeRepository) queryVolumes(ctx context.Context, query string, args ...interface{}) ([]*storage.Volume, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*storage.Volume
	for rows.Next() {
		v, err := r.scanVolume(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}

func (r *VolumeRepository) Update(ctx context.Context, v *storage.Volume) error {
	query := `
		UPDATE volumes
		SET instance_id = $2, name = $3, size_bytes = $4, content_type = $5, mount_path = $6, read_only = $7, status = $8, updated_at = $9
		WHERE id = $1
	`
	res, err := r.pool.Exec(ctx, query,
		v.ID, v.InstanceID, v.Name, v.SizeBytes, string(v.ContentType), v.MountPath, v.ReadOnly, string(v.Status), time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return storage.ErrVolumeNotFound
	}
	return nil
}

func (r *VolumeRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM volumes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return storage.ErrVolumeNotFound
	}
	return nil
}

func (r *VolumeRepository) scanVolume(row pgx.Row) (*storage.Volume, error) {
	var v storage.Volume
	var ctStr, statusStr string

	err := row.Scan(
		&v.ID, &v.UserID, &v.PoolID, &v.InstanceID, &v.Name, &v.SizeBytes, &ctStr, &v.MountPath, &v.ReadOnly, &statusStr, &v.CreatedAt, &v.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrVolumeNotFound
		}
		return nil, err
	}

	v.ContentType = storage.VolumeContentType(ctStr)
	v.Status = storage.VolumeStatus(statusStr)
	return &v, nil
}

// VolumeSnapshotRepository implements storage.VolumeSnapshotRepository with PostgreSQL.
type VolumeSnapshotRepository struct {
	pool *pgxpool.Pool
}

func NewVolumeSnapshotRepository(pool *pgxpool.Pool) *VolumeSnapshotRepository {
	return &VolumeSnapshotRepository{pool: pool}
}

func (r *VolumeSnapshotRepository) Create(ctx context.Context, s *storage.VolumeSnapshot) error {
	query := `
		INSERT INTO volume_snapshots (
			id, volume_id, name, size_bytes, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5
		) RETURNING id, created_at
	`
	now := time.Now().UTC()
	err := r.pool.QueryRow(ctx, query,
		s.ID, s.VolumeID, s.Name, s.SizeBytes, now,
	).Scan(&s.ID, &s.CreatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return storage.ErrVolumeSnapshotAlreadyExists
		}
		return err
	}
	return nil
}

func (r *VolumeSnapshotRepository) GetByID(ctx context.Context, id string) (*storage.VolumeSnapshot, error) {
	query := `
		SELECT id, volume_id, name, size_bytes, created_at
		FROM volume_snapshots WHERE id = $1
	`
	return r.scanSnapshot(r.pool.QueryRow(ctx, query, id))
}

func (r *VolumeSnapshotRepository) GetByVolumeAndName(ctx context.Context, volumeID, name string) (*storage.VolumeSnapshot, error) {
	query := `
		SELECT id, volume_id, name, size_bytes, created_at
		FROM volume_snapshots WHERE volume_id = $1 AND name = $2
	`
	return r.scanSnapshot(r.pool.QueryRow(ctx, query, volumeID, name))
}

func (r *VolumeSnapshotRepository) ListByVolume(ctx context.Context, volumeID string) ([]*storage.VolumeSnapshot, error) {
	query := `
		SELECT id, volume_id, name, size_bytes, created_at
		FROM volume_snapshots WHERE volume_id = $1
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, volumeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*storage.VolumeSnapshot
	for rows.Next() {
		s, err := r.scanSnapshot(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *VolumeSnapshotRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM volume_snapshots WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return storage.ErrVolumeSnapshotNotFound
	}
	return nil
}

func (r *VolumeSnapshotRepository) scanSnapshot(row pgx.Row) (*storage.VolumeSnapshot, error) {
	var s storage.VolumeSnapshot
	err := row.Scan(&s.ID, &s.VolumeID, &s.Name, &s.SizeBytes, &s.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, storage.ErrVolumeSnapshotNotFound
		}
		return nil, err
	}
	return &s, nil
}
