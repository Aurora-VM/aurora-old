package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/compute"
)

// MemoryInstanceStore holds in-memory state for instances.
type MemoryInstanceStore struct {
	mu        sync.RWMutex
	instances map[string]*compute.Instance // key: ID
	byName    map[string]*compute.Instance // key: Name
}

// NewMemoryInstanceStore initializes an in-memory instance store.
func NewMemoryInstanceStore() *MemoryInstanceStore {
	return &MemoryInstanceStore{
		instances: make(map[string]*compute.Instance),
		byName:    make(map[string]*compute.Instance),
	}
}

func (s *MemoryInstanceStore) Instances() *MemoryInstanceRepo {
	return &MemoryInstanceRepo{s: s}
}

type MemoryInstanceRepo struct {
	s *MemoryInstanceStore
}

func (r *MemoryInstanceRepo) Create(ctx context.Context, inst *compute.Instance) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, exists := r.s.byName[inst.Name]; exists {
		return compute.ErrInstanceAlreadyExists
	}

	copy := *inst
	r.s.instances[inst.ID] = &copy
	r.s.byName[inst.Name] = &copy
	return nil
}

func (r *MemoryInstanceRepo) GetByID(ctx context.Context, id string) (*compute.Instance, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	inst, exists := r.s.instances[id]
	if !exists {
		return nil, compute.ErrInstanceNotFound
	}
	copy := *inst
	return &copy, nil
}

func (r *MemoryInstanceRepo) GetByName(ctx context.Context, name string) (*compute.Instance, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	inst, exists := r.s.byName[name]
	if !exists {
		return nil, compute.ErrInstanceNotFound
	}
	copy := *inst
	return &copy, nil
}

func (r *MemoryInstanceRepo) ListByUser(ctx context.Context, userID string) ([]*compute.Instance, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var list []*compute.Instance
	for _, inst := range r.s.instances {
		if inst.UserID == userID {
			copy := *inst
			list = append(list, &copy)
		}
	}
	return list, nil
}

func (r *MemoryInstanceRepo) ListByNode(ctx context.Context, nodeID string) ([]*compute.Instance, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var list []*compute.Instance
	for _, inst := range r.s.instances {
		if inst.NodeID == nodeID {
			copy := *inst
			list = append(list, &copy)
		}
	}
	return list, nil
}

func (r *MemoryInstanceRepo) ListAll(ctx context.Context) ([]*compute.Instance, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var list []*compute.Instance
	for _, inst := range r.s.instances {
		copy := *inst
		list = append(list, &copy)
	}
	return list, nil
}

func (r *MemoryInstanceRepo) UpdateStatus(ctx context.Context, id string, status compute.Status, ipv4, ipv6 string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	inst, exists := r.s.instances[id]
	if !exists {
		return compute.ErrInstanceNotFound
	}

	inst.Status = status
	if ipv4 != "" {
		inst.IPv4Address = ipv4
	}
	if ipv6 != "" {
		inst.IPv6Address = ipv6
	}
	inst.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryInstanceRepo) UpdateSpec(ctx context.Context, id string, cpu int, memory, storage int64) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	inst, exists := r.s.instances[id]
	if !exists {
		return compute.ErrInstanceNotFound
	}

	inst.CPUCores = cpu
	inst.MemoryBytes = memory
	inst.StorageBytes = storage
	inst.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryInstanceRepo) UpdateNodeID(ctx context.Context, id string, nodeID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	inst, exists := r.s.instances[id]
	if !exists {
		return compute.ErrInstanceNotFound
	}

	inst.NodeID = nodeID
	inst.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryInstanceRepo) Delete(ctx context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	inst, exists := r.s.instances[id]
	if !exists {
		return compute.ErrInstanceNotFound
	}

	delete(r.s.byName, inst.Name)
	delete(r.s.instances, id)
	return nil
}
