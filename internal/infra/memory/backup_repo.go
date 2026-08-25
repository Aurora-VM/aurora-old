package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	"github.com/google/uuid"
)

// MemoryBackupRepo implements domainBackup.Repository in memory.
type MemoryBackupRepo struct {
	mu           sync.RWMutex
	backups      map[string]*domainBackup.Record
	policies     map[string]*domainBackup.Policy
	restorePlans map[string]*domainBackup.RestorePlan
}

func NewMemoryBackupRepo() *MemoryBackupRepo {
	r := &MemoryBackupRepo{
		backups:      make(map[string]*domainBackup.Record),
		policies:     make(map[string]*domainBackup.Policy),
		restorePlans: make(map[string]*domainBackup.RestorePlan),
	}
	r.seedDefaultPolicy()
	return r
}

func (r *MemoryBackupRepo) seedDefaultPolicy() {
	now := time.Now().UTC()
	defaultPolicy := &domainBackup.Policy{
		ID:            "policy-default-daily",
		Name:          "Default Daily PostgreSQL & Cluster DR Policy",
		ScheduleCron:  "0 2 * * *",
		RetentionDays: 30,
		MaxBackups:    14,
		StorageTarget: "s3",
		Encrypt:       true,
		Enabled:       true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
	r.policies[defaultPolicy.ID] = defaultPolicy
}

func (r *MemoryBackupRepo) Create(ctx context.Context, b *domainBackup.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if b.ID == "" {
		b.ID = uuid.NewString()
	}
	if b.CreatedAt.IsZero() {
		b.CreatedAt = time.Now().UTC()
	}

	cp := *b
	r.backups[b.ID] = &cp
	return nil
}

func (r *MemoryBackupRepo) GetByID(ctx context.Context, id string) (*domainBackup.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	b, exists := r.backups[id]
	if !exists {
		return nil, domainBackup.ErrBackupNotFound
	}
	cp := *b
	return &cp, nil
}

func (r *MemoryBackupRepo) List(ctx context.Context, filter domainBackup.Filter) ([]*domainBackup.Record, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domainBackup.Record
	for _, b := range r.backups {
		if filter.TenantID != "" && filter.TenantID != "system" && b.TenantID != filter.TenantID {
			continue
		}
		if filter.ResourceType != "" && b.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && b.ResourceID != filter.ResourceID {
			continue
		}
		if filter.Type != "" && b.Type != filter.Type {
			continue
		}
		if filter.Status != "" && b.Status != filter.Status {
			continue
		}
		cp := *b
		result = append(result, &cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	total := len(result)
	start := filter.Offset
	if start > total {
		start = total
	}
	end := start + filter.Limit
	if filter.Limit <= 0 || end > total {
		end = total
	}

	return result[start:end], total, nil
}

func (r *MemoryBackupRepo) UpdateStatus(ctx context.Context, id string, status domainBackup.Status, checksum string, size int64, errStr string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, exists := r.backups[id]
	if !exists {
		return domainBackup.ErrBackupNotFound
	}

	b.Status = status
	if checksum != "" {
		b.ChecksumSHA256 = checksum
	}
	if size > 0 {
		b.SizeBytes = size
	}
	if errStr != "" {
		b.ErrorMessage = errStr
	}
	now := time.Now().UTC()
	if status == domainBackup.StatusVerified {
		b.VerifiedAt = &now
		b.CompletedAt = &now
	}
	return nil
}

func (r *MemoryBackupRepo) SetProtectedPoint(ctx context.Context, id string, protected bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, exists := r.backups[id]
	if !exists {
		return domainBackup.ErrBackupNotFound
	}
	b.IsProtectedPoint = protected
	return nil
}

func (r *MemoryBackupRepo) GetLatestVerified(ctx context.Context, tenantID, resourceType string) (*domainBackup.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *domainBackup.Record
	for _, b := range r.backups {
		if tenantID != "" && b.TenantID != tenantID {
			continue
		}
		if resourceType != "" && b.ResourceType != resourceType {
			continue
		}
		if b.Status != domainBackup.StatusVerified {
			continue
		}
		if latest == nil || b.CreatedAt.After(latest.CreatedAt) {
			cp := *b
			latest = &cp
		}
	}
	if latest == nil {
		return nil, domainBackup.ErrBackupNotFound
	}
	return latest, nil
}

func (r *MemoryBackupRepo) CountVerified(ctx context.Context) (int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, b := range r.backups {
		if b.Status == domainBackup.StatusVerified {
			count++
		}
	}
	return count, nil
}

func (r *MemoryBackupRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	b, exists := r.backups[id]
	if !exists {
		return domainBackup.ErrBackupNotFound
	}

	// Check if this is the final verified backup
	if b.IsProtectedPoint {
		return domainBackup.ErrCannotDeleteLastGoodBackup
	}

	verifiedCount := 0
	for _, item := range r.backups {
		if item.Status == domainBackup.StatusVerified && item.ID != id {
			verifiedCount++
		}
	}
	if b.Status == domainBackup.StatusVerified && verifiedCount == 0 {
		return domainBackup.ErrCannotDeleteLastGoodBackup
	}

	delete(r.backups, id)
	return nil
}

// Policy CRUD
func (r *MemoryBackupRepo) CreatePolicy(ctx context.Context, p *domainBackup.Policy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if p.ID == "" {
		p.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	p.CreatedAt = now
	p.UpdatedAt = now

	cp := *p
	r.policies[p.ID] = &cp
	return nil
}

func (r *MemoryBackupRepo) GetPolicy(ctx context.Context, id string) (*domainBackup.Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.policies[id]
	if !exists {
		return nil, domainBackup.ErrPolicyNotFound
	}
	cp := *p
	return &cp, nil
}

func (r *MemoryBackupRepo) ListPolicies(ctx context.Context) ([]*domainBackup.Policy, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*domainBackup.Policy
	for _, p := range r.policies {
		cp := *p
		list = append(list, &cp)
	}
	return list, nil
}

func (r *MemoryBackupRepo) UpdatePolicy(ctx context.Context, p *domainBackup.Policy) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.policies[p.ID]
	if !exists {
		return domainBackup.ErrPolicyNotFound
	}

	p.CreatedAt = existing.CreatedAt
	p.UpdatedAt = time.Now().UTC()

	cp := *p
	r.policies[p.ID] = &cp
	return nil
}

func (r *MemoryBackupRepo) DeletePolicy(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.policies, id)
	return nil
}

// Restore Plan CRUD
func (r *MemoryBackupRepo) SaveRestorePlan(ctx context.Context, plan *domainBackup.RestorePlan) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if plan.ID == "" {
		plan.ID = uuid.NewString()
	}
	if plan.CreatedAt.IsZero() {
		plan.CreatedAt = time.Now().UTC()
	}

	cp := *plan
	r.restorePlans[plan.ID] = &cp
	return nil
}

func (r *MemoryBackupRepo) GetRestorePlan(ctx context.Context, id string) (*domainBackup.RestorePlan, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	p, exists := r.restorePlans[id]
	if !exists {
		return nil, domainBackup.ErrRestoreFailed
	}
	cp := *p
	return &cp, nil
}

func (r *MemoryBackupRepo) ListRestorePlans(ctx context.Context, limit, offset int) ([]*domainBackup.RestorePlan, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*domainBackup.RestorePlan
	for _, p := range r.restorePlans {
		cp := *p
		result = append(result, &cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.After(result[j].CreatedAt)
	})

	total := len(result)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if limit <= 0 || end > total {
		end = total
	}

	return result[start:end], total, nil
}
