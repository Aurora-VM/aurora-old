package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/storage"
	"github.com/google/uuid"
)

// StoragePoolRepository is an in-memory implementation of storage.StoragePoolRepository.
type StoragePoolRepository struct {
	mu    sync.RWMutex
	pools map[string]*storage.StoragePool // id -> pool
}

func NewStoragePoolRepository() *StoragePoolRepository {
	return &StoragePoolRepository{
		pools: make(map[string]*storage.StoragePool),
	}
}

func (r *StoragePoolRepository) Create(ctx context.Context, pool *storage.StoragePool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, p := range r.pools {
		if p.NodeID == pool.NodeID && p.Name == pool.Name {
			return storage.ErrStoragePoolAlreadyExists
		}
	}

	if pool.ID == "" {
		pool.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	pool.CreatedAt = now
	pool.UpdatedAt = now

	cp := *pool
	r.pools[pool.ID] = &cp
	return nil
}

func (r *StoragePoolRepository) GetByID(ctx context.Context, id string) (*storage.StoragePool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, ok := r.pools[id]
	if !ok {
		return nil, storage.ErrStoragePoolNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *StoragePoolRepository) GetByNodeAndName(ctx context.Context, nodeID, name string) (*storage.StoragePool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.pools {
		if p.NodeID == nodeID && p.Name == name {
			cp := *p
			return &cp, nil
		}
	}
	return nil, storage.ErrStoragePoolNotFound
}

func (r *StoragePoolRepository) List(ctx context.Context, nodeID string) ([]*storage.StoragePool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.StoragePool
	for _, p := range r.pools {
		if nodeID == "" || p.NodeID == nodeID {
			cp := *p
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *StoragePoolRepository) Update(ctx context.Context, pool *storage.StoragePool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.pools[pool.ID]; !ok {
		return storage.ErrStoragePoolNotFound
	}

	pool.UpdatedAt = time.Now().UTC()
	cp := *pool
	r.pools[pool.ID] = &cp
	return nil
}

func (r *StoragePoolRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.pools[id]; !ok {
		return storage.ErrStoragePoolNotFound
	}
	delete(r.pools, id)
	return nil
}

// VolumeRepository is an in-memory implementation of storage.VolumeRepository.
type VolumeRepository struct {
	mu      sync.RWMutex
	volumes map[string]*storage.Volume // id -> volume
}

func NewVolumeRepository() *VolumeRepository {
	return &VolumeRepository{
		volumes: make(map[string]*storage.Volume),
	}
}

func (r *VolumeRepository) Create(ctx context.Context, vol *storage.Volume) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, v := range r.volumes {
		if v.PoolID == vol.PoolID && v.Name == vol.Name {
			return storage.ErrVolumeAlreadyExists
		}
	}

	if vol.ID == "" {
		vol.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	vol.CreatedAt = now
	vol.UpdatedAt = now

	cp := *vol
	r.volumes[vol.ID] = &cp
	return nil
}

func (r *VolumeRepository) GetByID(ctx context.Context, id string) (*storage.Volume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	v, ok := r.volumes[id]
	if !ok {
		return nil, storage.ErrVolumeNotFound
	}
	cp := *v
	return &cp, nil
}

func (r *VolumeRepository) GetByPoolAndName(ctx context.Context, poolID, name string) (*storage.Volume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, v := range r.volumes {
		if v.PoolID == poolID && v.Name == name {
			cp := *v
			return &cp, nil
		}
	}
	return nil, storage.ErrVolumeNotFound
}

func (r *VolumeRepository) ListByUser(ctx context.Context, userID string) ([]*storage.Volume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.Volume
	for _, v := range r.volumes {
		if userID == "" || v.UserID == userID {
			cp := *v
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *VolumeRepository) ListByPool(ctx context.Context, poolID string) ([]*storage.Volume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.Volume
	for _, v := range r.volumes {
		if v.PoolID == poolID {
			cp := *v
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *VolumeRepository) ListByInstance(ctx context.Context, instanceID string) ([]*storage.Volume, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.Volume
	for _, v := range r.volumes {
		if v.InstanceID != nil && *v.InstanceID == instanceID {
			cp := *v
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *VolumeRepository) Update(ctx context.Context, vol *storage.Volume) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.volumes[vol.ID]; !ok {
		return storage.ErrVolumeNotFound
	}

	vol.UpdatedAt = time.Now().UTC()
	cp := *vol
	r.volumes[vol.ID] = &cp
	return nil
}

func (r *VolumeRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.volumes[id]; !ok {
		return storage.ErrVolumeNotFound
	}
	delete(r.volumes, id)
	return nil
}

// VolumeSnapshotRepository is an in-memory implementation of storage.VolumeSnapshotRepository.
type VolumeSnapshotRepository struct {
	mu        sync.RWMutex
	snapshots map[string]*storage.VolumeSnapshot // id -> snapshot
}

func NewVolumeSnapshotRepository() *VolumeSnapshotRepository {
	return &VolumeSnapshotRepository{
		snapshots: make(map[string]*storage.VolumeSnapshot),
	}
}

func (r *VolumeSnapshotRepository) Create(ctx context.Context, snap *storage.VolumeSnapshot) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, s := range r.snapshots {
		if s.VolumeID == snap.VolumeID && s.Name == snap.Name {
			return storage.ErrVolumeSnapshotAlreadyExists
		}
	}

	if snap.ID == "" {
		snap.ID = uuid.New().String()
	}
	snap.CreatedAt = time.Now().UTC()

	cp := *snap
	r.snapshots[snap.ID] = &cp
	return nil
}

func (r *VolumeSnapshotRepository) GetByID(ctx context.Context, id string) (*storage.VolumeSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, ok := r.snapshots[id]
	if !ok {
		return nil, storage.ErrVolumeSnapshotNotFound
	}
	cp := *s
	return &cp, nil
}

func (r *VolumeSnapshotRepository) GetByVolumeAndName(ctx context.Context, volumeID, name string) (*storage.VolumeSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, s := range r.snapshots {
		if s.VolumeID == volumeID && s.Name == name {
			cp := *s
			return &cp, nil
		}
	}
	return nil, storage.ErrVolumeSnapshotNotFound
}

func (r *VolumeSnapshotRepository) ListByVolume(ctx context.Context, volumeID string) ([]*storage.VolumeSnapshot, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*storage.VolumeSnapshot
	for _, s := range r.snapshots {
		if s.VolumeID == volumeID {
			cp := *s
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *VolumeSnapshotRepository) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.snapshots[id]; !ok {
		return storage.ErrVolumeSnapshotNotFound
	}
	delete(r.snapshots, id)
	return nil
}
