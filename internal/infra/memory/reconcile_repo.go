package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	domainReconcile "github.com/aurora-vm/aurora/internal/domain/reconcile"
	"github.com/google/uuid"
)

// MemoryReconcileRepo implements domainReconcile.Repository in memory.
type MemoryReconcileRepo struct {
	mu      sync.RWMutex
	reports map[string]*domainReconcile.Report
}

func NewMemoryReconcileRepo() *MemoryReconcileRepo {
	return &MemoryReconcileRepo{
		reports: make(map[string]*domainReconcile.Report),
	}
}

func (r *MemoryReconcileRepo) SaveReport(ctx context.Context, rep *domainReconcile.Report) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rep.ID == "" {
		rep.ID = uuid.NewString()
	}
	if rep.CreatedAt.IsZero() {
		rep.CreatedAt = time.Now().UTC()
	}

	cp := *rep
	r.reports[rep.ID] = &cp
	return nil
}

func (r *MemoryReconcileRepo) GetLatestReport(ctx context.Context) (*domainReconcile.Report, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *domainReconcile.Report
	for _, rep := range r.reports {
		if latest == nil || rep.CreatedAt.After(latest.CreatedAt) {
			cp := *rep
			latest = &cp
		}
	}
	return latest, nil
}

func (r *MemoryReconcileRepo) ListReports(ctx context.Context, limit, offset int) ([]*domainReconcile.Report, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*domainReconcile.Report
	for _, rep := range r.reports {
		cp := *rep
		list = append(list, &cp)
	}

	sort.Slice(list, func(i, j int) bool {
		return list[i].CreatedAt.After(list[j].CreatedAt)
	})

	total := len(list)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if limit <= 0 || end > total {
		end = total
	}

	return list[start:end], total, nil
}
