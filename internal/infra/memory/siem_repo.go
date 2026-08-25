package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/audit"
	"github.com/google/uuid"
)

// MemorySIEMRepo implements audit.SIEMRepository in memory.
type MemorySIEMRepo struct {
	mu           sync.RWMutex
	destinations map[string]*audit.SIEMDestination
}

func NewMemorySIEMRepo() *MemorySIEMRepo {
	return &MemorySIEMRepo{
		destinations: make(map[string]*audit.SIEMDestination),
	}
}

func (r *MemorySIEMRepo) Create(ctx context.Context, dest *audit.SIEMDestination) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if dest.ID == "" {
		dest.ID = uuid.New().String()
	}
	now := time.Now().UTC()
	dest.CreatedAt = now
	dest.UpdatedAt = now

	cp := *dest
	r.destinations[dest.ID] = &cp
	return nil
}

func (r *MemorySIEMRepo) GetByID(ctx context.Context, id string) (*audit.SIEMDestination, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dest, ok := r.destinations[id]
	if !ok {
		return nil, audit.ErrSIEMDestinationNotFound
	}
	cp := *dest
	return &cp, nil
}

func (r *MemorySIEMRepo) List(ctx context.Context) ([]*audit.SIEMDestination, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*audit.SIEMDestination
	for _, d := range r.destinations {
		cp := *d
		result = append(result, &cp)
	}
	return result, nil
}

func (r *MemorySIEMRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.destinations[id]; !ok {
		return audit.ErrSIEMDestinationNotFound
	}
	delete(r.destinations, id)
	return nil
}
