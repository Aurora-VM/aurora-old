package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/ipam"
	"github.com/google/uuid"
)

type MemoryIPPoolRepo struct {
	mu    sync.RWMutex
	pools map[string]*ipam.IPPool
}

func NewMemoryIPPoolRepo() *MemoryIPPoolRepo {
	return &MemoryIPPoolRepo{
		pools: make(map[string]*ipam.IPPool),
	}
}

func (r *MemoryIPPoolRepo) Create(ctx context.Context, pool *ipam.IPPool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.pools {
		if existing.CIDR == pool.CIDR {
			return ipam.ErrIPPoolAlreadyExists
		}
	}

	if pool.ID == "" {
		pool.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	pool.CreatedAt = now
	pool.UpdatedAt = now

	cp := *pool
	r.pools[pool.ID] = &cp
	return nil
}

func (r *MemoryIPPoolRepo) GetByID(ctx context.Context, id string) (*ipam.IPPool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	pool, exists := r.pools[id]
	if !exists {
		return nil, ipam.ErrIPPoolNotFound
	}
	cp := *pool
	return &cp, nil
}

func (r *MemoryIPPoolRepo) GetByCIDR(ctx context.Context, cidr string) (*ipam.IPPool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, p := range r.pools {
		if p.CIDR == cidr {
			cp := *p
			return &cp, nil
		}
	}
	return nil, ipam.ErrIPPoolNotFound
}

func (r *MemoryIPPoolRepo) List(ctx context.Context, locationID string) ([]*ipam.IPPool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*ipam.IPPool
	for _, p := range r.pools {
		if locationID == "" || p.LocationID == locationID {
			cp := *p
			results = append(results, &cp)
		}
	}
	return results, nil
}

func (r *MemoryIPPoolRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.pools[id]; !exists {
		return ipam.ErrIPPoolNotFound
	}
	delete(r.pools, id)
	return nil
}

// ---------------- MEMORY IP ALLOCATION REPO ----------------

type MemoryIPAllocationRepo struct {
	mu          sync.RWMutex
	allocations map[string]*ipam.IPAllocation
}

func NewMemoryIPAllocationRepo() *MemoryIPAllocationRepo {
	return &MemoryIPAllocationRepo{
		allocations: make(map[string]*ipam.IPAllocation),
	}
}

func (r *MemoryIPAllocationRepo) Create(ctx context.Context, alloc *ipam.IPAllocation) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, existing := range r.allocations {
		if existing.IPAddress == alloc.IPAddress && existing.ReleasedAt == nil {
			return ipam.ErrIPAlreadyAllocated
		}
	}

	if alloc.ID == "" {
		alloc.ID = uuid.NewString()
	}
	alloc.AllocatedAt = time.Now().UTC()
	alloc.ReleasedAt = nil

	cp := *alloc
	r.allocations[alloc.ID] = &cp
	return nil
}

func (r *MemoryIPAllocationRepo) GetByID(ctx context.Context, id string) (*ipam.IPAllocation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	alloc, exists := r.allocations[id]
	if !exists {
		return nil, ipam.ErrIPAllocationNotFound
	}
	cp := *alloc
	return &cp, nil
}

func (r *MemoryIPAllocationRepo) GetByIP(ctx context.Context, ip string) (*ipam.IPAllocation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, alloc := range r.allocations {
		if alloc.IPAddress == ip && alloc.ReleasedAt == nil {
			cp := *alloc
			return &cp, nil
		}
	}
	return nil, ipam.ErrIPAllocationNotFound
}

func (r *MemoryIPAllocationRepo) ListByPoolID(ctx context.Context, poolID string) ([]*ipam.IPAllocation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*ipam.IPAllocation
	for _, a := range r.allocations {
		if a.PoolID == poolID && a.ReleasedAt == nil {
			cp := *a
			results = append(results, &cp)
		}
	}
	return results, nil
}

func (r *MemoryIPAllocationRepo) ListByInstanceID(ctx context.Context, instanceID string) ([]*ipam.IPAllocation, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*ipam.IPAllocation
	for _, a := range r.allocations {
		if a.InstanceID != nil && *a.InstanceID == instanceID && a.ReleasedAt == nil {
			cp := *a
			results = append(results, &cp)
		}
	}
	return results, nil
}

func (r *MemoryIPAllocationRepo) Release(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	alloc, exists := r.allocations[id]
	if !exists {
		return ipam.ErrIPAllocationNotFound
	}
	now := time.Now().UTC()
	alloc.ReleasedAt = &now
	return nil
}

func (r *MemoryIPAllocationRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.allocations[id]; !exists {
		return ipam.ErrIPAllocationNotFound
	}
	delete(r.allocations, id)
	return nil
}
