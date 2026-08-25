package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	domainKeyRotation "github.com/aurora-vm/aurora/internal/domain/keyrotation"
	"github.com/google/uuid"
)

// MemoryKeyRotationRepo implements domainKeyRotation.Repository in memory.
type MemoryKeyRotationRepo struct {
	mu      sync.RWMutex
	records map[string]*domainKeyRotation.Record
}

func NewMemoryKeyRotationRepo() *MemoryKeyRotationRepo {
	r := &MemoryKeyRotationRepo{
		records: make(map[string]*domainKeyRotation.Record),
	}
	r.seedActiveKeys()
	return r
}

func (r *MemoryKeyRotationRepo) seedActiveKeys() {
	now := time.Now().UTC()
	defaults := []*domainKeyRotation.Record{
		{
			ID:          "key-jwt-active",
			Type:        domainKeyRotation.TypeJWTSigning,
			KeyID:       "jwt-key-2026-v1",
			Status:      domainKeyRotation.StatusActive,
			Version:     1,
			Algorithm:   "HS256",
			Description: "Primary JWT HMAC-SHA256 Signing Key",
			RotatedBy:   "system:bootstrap",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "key-backup-active",
			Type:        domainKeyRotation.TypeBackupEncryption,
			KeyID:       "backup-aes-2026-v1",
			Status:      domainKeyRotation.StatusActive,
			Version:     1,
			Algorithm:   "AES-GCM-256",
			Description: "Envelope Encryption Key for Disaster Recovery Backups",
			RotatedBy:   "system:bootstrap",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
	for _, rec := range defaults {
		r.records[rec.ID] = rec
	}
}

func (r *MemoryKeyRotationRepo) Save(ctx context.Context, rec *domainKeyRotation.Record) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rec.ID == "" {
		rec.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = now
	}
	rec.UpdatedAt = now

	cp := *rec
	r.records[rec.ID] = &cp
	return nil
}

func (r *MemoryKeyRotationRepo) GetByID(ctx context.Context, id string) (*domainKeyRotation.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, exists := r.records[id]
	if !exists {
		return nil, domainKeyRotation.ErrKeyNotFound
	}
	cp := *rec
	return &cp, nil
}

func (r *MemoryKeyRotationRepo) GetActive(ctx context.Context, keyType domainKeyRotation.KeyType) (*domainKeyRotation.Record, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var latest *domainKeyRotation.Record
	for _, rec := range r.records {
		if rec.Type == keyType && (rec.Status == domainKeyRotation.StatusActive || rec.Status == domainKeyRotation.StatusGracePeriod) {
			if latest == nil || rec.Version > latest.Version {
				cp := *rec
				latest = &cp
			}
		}
	}
	if latest == nil {
		return nil, domainKeyRotation.ErrKeyNotFound
	}
	return latest, nil
}

func (r *MemoryKeyRotationRepo) List(ctx context.Context, keyType domainKeyRotation.KeyType, limit, offset int) ([]*domainKeyRotation.Record, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var list []*domainKeyRotation.Record
	for _, rec := range r.records {
		if keyType != "" && rec.Type != keyType {
			continue
		}
		cp := *rec
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

func (r *MemoryKeyRotationRepo) UpdateStatus(ctx context.Context, id string, status domainKeyRotation.KeyStatus, reason string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	rec, exists := r.records[id]
	if !exists {
		return domainKeyRotation.ErrKeyNotFound
	}

	rec.Status = status
	rec.UpdatedAt = time.Now().UTC()
	if status == domainKeyRotation.StatusRevoked {
		now := time.Now().UTC()
		rec.RevokedAt = &now
		rec.RevocationReason = reason
	}
	return nil
}
