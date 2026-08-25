package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	domainMigration "github.com/aurora-vm/aurora/internal/domain/migration"
	"github.com/google/uuid"
)

// MemoryMigrationRepo implements domainMigration.MigrationRepository in memory.
type MemoryMigrationRepo struct {
	mu         sync.RWMutex
	migrations map[string]*domainMigration.Migration
}

// NewMemoryMigrationRepo creates an in-memory migration repository.
func NewMemoryMigrationRepo() *MemoryMigrationRepo {
	return &MemoryMigrationRepo{
		migrations: make(map[string]*domainMigration.Migration),
	}
}

func (r *MemoryMigrationRepo) Create(ctx context.Context, m *domainMigration.Migration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if m.ID == "" {
		m.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if m.CreatedAt.IsZero() {
		m.CreatedAt = now
	}
	m.UpdatedAt = now

	copy := *m
	r.migrations[m.ID] = &copy
	return nil
}

func (r *MemoryMigrationRepo) GetByID(ctx context.Context, id string) (*domainMigration.Migration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	m, exists := r.migrations[id]
	if !exists {
		return nil, domainMigration.ErrMigrationNotFound
	}
	copy := *m
	return &copy, nil
}

func (r *MemoryMigrationRepo) GetActiveForInstance(ctx context.Context, instanceID string) (*domainMigration.Migration, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, m := range r.migrations {
		if m.InstanceID == instanceID && !m.Status.IsTerminal() {
			copy := *m
			return &copy, nil
		}
	}
	return nil, nil
}

func (r *MemoryMigrationRepo) List(ctx context.Context, filter domainMigration.MigrationFilter) ([]*domainMigration.Migration, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*domainMigration.Migration
	for _, m := range r.migrations {
		if filter.TenantID != "" && m.TenantID != filter.TenantID {
			continue
		}
		if filter.InstanceID != "" && m.InstanceID != filter.InstanceID {
			continue
		}
		if filter.SourceNodeID != "" && m.SourceNodeID != filter.SourceNodeID {
			continue
		}
		if filter.DestNodeID != "" && m.DestNodeID != filter.DestNodeID {
			continue
		}
		if filter.Status != "" && m.Status != filter.Status {
			continue
		}

		copy := *m
		matched = append(matched, &copy)
	}

	sort.Slice(matched, func(i, j int) bool {
		return matched[i].CreatedAt.After(matched[j].CreatedAt)
	})

	total := len(matched)
	if filter.Offset >= total {
		return []*domainMigration.Migration{}, total, nil
	}

	end := filter.Offset + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}

	return matched[filter.Offset:end], total, nil
}

func (r *MemoryMigrationRepo) UpdateStatus(ctx context.Context, id string, status domainMigration.Status, progress int, errStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, exists := r.migrations[id]
	if !exists {
		return domainMigration.ErrMigrationNotFound
	}

	now := time.Now().UTC()
	m.Status = status
	m.ProgressPercent = progress
	m.Error = errStr
	m.UpdatedAt = now

	if status == domainMigration.StatusTransferring && m.StartedAt == nil {
		m.StartedAt = &now
	}
	if status.IsTerminal() && m.CompletedAt == nil {
		m.CompletedAt = &now
	}

	return nil
}

func (r *MemoryMigrationRepo) UpdateProgress(ctx context.Context, id string, progress int, transferred, total int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	m, exists := r.migrations[id]
	if !exists {
		return domainMigration.ErrMigrationNotFound
	}

	m.ProgressPercent = progress
	m.BytesTransferred = transferred
	m.TotalBytes = total
	m.UpdatedAt = time.Now().UTC()
	return nil
}
