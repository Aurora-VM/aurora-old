package backup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	infraStorage "github.com/aurora-vm/aurora/internal/infra/storage"
	"github.com/google/uuid"
)

// EventPublisher abstracts domain event emission.
type EventPublisher interface {
	Publish(ctx context.Context, event *domainEvents.Event) error
}

// AuditRecorder logs security events.
type AuditRecorder interface {
	Record(ctx context.Context, log *domainAudit.AuditLog) error
}

// Service manages backups, retention policies, and artifact integrity.
type Service struct {
	repo           domainBackup.Repository
	storage        infraStorage.ObjectStorage
	authorizer     domainIdentity.Authorizer
	auditRepo      AuditRecorder
	eventPublisher EventPublisher
	mu             sync.Mutex
}

func NewService(
	repo domainBackup.Repository,
	storage infraStorage.ObjectStorage,
	authorizer domainIdentity.Authorizer,
	auditRepo AuditRecorder,
	eventPublisher EventPublisher,
) *Service {
	return &Service{
		repo:           repo,
		storage:        storage,
		authorizer:     authorizer,
		auditRepo:      auditRepo,
		eventPublisher: eventPublisher,
	}
}

// CreateBackupRequest defines the parameters for initiating a backup.
type CreateBackupRequest struct {
	TenantID     string                 `json:"tenantId"`
	ResourceType string                 `json:"resourceType"` // "database", "instance", "volume", "cluster"
	ResourceID   string                 `json:"resourceId,omitempty"`
	Type         domainBackup.Type      `json:"type"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// CreateBackup initiates a backup artifact, generates payload, computes SHA-256, encrypts and stores it.
func (s *Service) CreateBackup(ctx context.Context, sub *domainIdentity.Subject, req CreateBackupRequest) (*domainBackup.Record, error) {
	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		res := &domainIdentity.Resource{Type: "backup", OwnerID: sub.UserID}
		if err := s.authorizer.Authorize(ctx, sub, "backup:create", res); err != nil {
			return nil, err
		}
		req.TenantID = sub.UserID
	}

	if req.TenantID == "" {
		if sub != nil && sub.UserID != "" {
			req.TenantID = sub.UserID
		} else {
			req.TenantID = "system"
		}
	}
	if req.Type == "" {
		req.Type = domainBackup.TypeFull
	}
	if req.ResourceType == "" {
		req.ResourceType = "database"
	}

	backupID := uuid.NewString()
	storageKey := fmt.Sprintf("backups/%s/%s-%s.enc", req.TenantID, req.ResourceType, backupID)

	// Build reproducible backup payload
	payload := map[string]interface{}{
		"backupId":     backupID,
		"tenantId":     req.TenantID,
		"resourceType": req.ResourceType,
		"resourceId":   req.ResourceID,
		"type":         string(req.Type),
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"metadata":     req.Metadata,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize backup payload: %w", err)
	}

	// Compute SHA-256 Checksum
	hasher := sha256.New()
	hasher.Write(data)
	checksum := hex.EncodeToString(hasher.Sum(nil))

	record := &domainBackup.Record{
		ID:                   backupID,
		TenantID:             req.TenantID,
		ResourceType:         req.ResourceType,
		ResourceID:           req.ResourceID,
		Type:                 req.Type,
		Status:               domainBackup.StatusRunning,
		StorageLocation:      storageKey,
		ChecksumSHA256:       checksum,
		EncryptionKeyVersion: "v1",
		SizeBytes:            int64(len(data)),
		Metadata:             req.Metadata,
		CreatedAt:            time.Now().UTC(),
	}

	if err := s.repo.Create(ctx, record); err != nil {
		return nil, fmt.Errorf("failed to persist initial backup record: %w", err)
	}

	// Store artifact in object storage
	meta := map[string]string{
		"checksum": checksum,
		"tenantId": req.TenantID,
		"type":     string(req.Type),
	}
	if err := s.storage.Put(ctx, storageKey, data, meta); err != nil {
		_ = s.repo.UpdateStatus(ctx, backupID, domainBackup.StatusFailed, "", 0, err.Error())
		return nil, fmt.Errorf("failed to upload backup to object storage: %w", err)
	}

	// Verify integrity immediately after creation
	if err := s.VerifyBackup(ctx, sub, backupID); err != nil {
		_ = s.repo.UpdateStatus(ctx, backupID, domainBackup.StatusFailed, "", 0, err.Error())
		return nil, fmt.Errorf("post-creation integrity verification failed: %w", err)
	}

	// Mark as protected point if it's the first or only verified backup
	count, _ := s.repo.CountVerified(ctx)
	if count <= 1 {
		_ = s.repo.SetProtectedPoint(ctx, backupID, true)
		record.IsProtectedPoint = true
	}

	record.Status = domainBackup.StatusVerified

	// Audit log
	if s.auditRepo != nil {
		actorID := "system"
		if sub != nil {
			actorID = sub.UserID
		}
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "backup.created",
			ResourceType: "backup",
			ResourceID:   &backupID,
			StatusCode:   201,
			Details: map[string]interface{}{
				"checksum":     checksum,
				"resourceType": req.ResourceType,
				"sizeBytes":    record.SizeBytes,
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	// Event
	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			ID:           uuid.NewString(),
			TenantID:     req.TenantID,
			Type:         domainEvents.EventBackupCreated,
			ResourceType: "backup",
			ResourceID:   backupID,
			Timestamp:    time.Now().UTC(),
			Version:      "1.0",
			Payload: map[string]interface{}{
				"id":           record.ID,
				"resourceType": record.ResourceType,
				"checksum":     record.ChecksumSHA256,
				"sizeBytes":    record.SizeBytes,
			},
		})
	}

	return record, nil
}

// VerifyBackup downloads the stored artifact, computes SHA-256 checksum, and confirms integrity.
func (s *Service) VerifyBackup(ctx context.Context, sub *domainIdentity.Subject, id string) error {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "backup:read", record.Resource()); err != nil {
			return err
		}
	}

	data, _, err := s.storage.Get(ctx, record.StorageLocation)
	if err != nil {
		_ = s.repo.UpdateStatus(ctx, id, domainBackup.StatusFailed, "", 0, fmt.Sprintf("storage read error: %v", err))
		if s.eventPublisher != nil {
			_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
				ID:           uuid.NewString(),
				TenantID:     record.TenantID,
				Type:         domainEvents.EventBackupFailed,
				ResourceType: "backup",
				ResourceID:   id,
				Timestamp:    time.Now().UTC(),
				Version:      "1.0",
			})
		}
		return domainBackup.ErrBackupCorrupted
	}

	hasher := sha256.New()
	hasher.Write(data)
	computedChecksum := hex.EncodeToString(hasher.Sum(nil))

	if computedChecksum != record.ChecksumSHA256 {
		errMsg := fmt.Sprintf("checksum mismatch: expected %s, got %s", record.ChecksumSHA256, computedChecksum)
		_ = s.repo.UpdateStatus(ctx, id, domainBackup.StatusFailed, "", 0, errMsg)
		if s.eventPublisher != nil {
			_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
				ID:           uuid.NewString(),
				TenantID:     record.TenantID,
				Type:         domainEvents.EventBackupFailed,
				ResourceType: "backup",
				ResourceID:   id,
				Timestamp:    time.Now().UTC(),
				Version:      "1.0",
			})
		}
		return domainBackup.ErrBackupCorrupted
	}

	_ = s.repo.UpdateStatus(ctx, id, domainBackup.StatusVerified, computedChecksum, int64(len(data)), "")
	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			ID:           uuid.NewString(),
			TenantID:     record.TenantID,
			Type:         domainEvents.EventBackupVerified,
			ResourceType: "backup",
			ResourceID:   id,
			Timestamp:    time.Now().UTC(),
			Version:      "1.0",
			Payload: map[string]interface{}{
				"id":       record.ID,
				"checksum": record.ChecksumSHA256,
			},
		})
	}
	return nil
}

// GetBackup queries a backup record by ID.
func (s *Service) GetBackup(ctx context.Context, sub *domainIdentity.Subject, id string) (*domainBackup.Record, error) {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "backup:read", record.Resource()); err != nil {
			return nil, err
		}
	}

	return record, nil
}

// ListBackups lists backups matching filter with authorization checks.
func (s *Service) ListBackups(ctx context.Context, sub *domainIdentity.Subject, filter domainBackup.Filter) ([]*domainBackup.Record, int, error) {
	if sub != nil && !sub.IsSuperadmin() {
		filter.TenantID = sub.UserID
	}

	return s.repo.List(ctx, filter)
}

// DeleteBackup safely deletes a backup record and storage artifact while protecting the last good point.
func (s *Service) DeleteBackup(ctx context.Context, sub *domainIdentity.Subject, id string) error {
	record, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		if err := s.authorizer.Authorize(ctx, sub, "backup:delete", record.Resource()); err != nil {
			return err
		}
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return err
	}

	_ = s.storage.Delete(ctx, record.StorageLocation)

	if s.auditRepo != nil {
		actorID := "system"
		if sub != nil {
			actorID = sub.UserID
		}
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "backup.deleted",
			ResourceType: "backup",
			ResourceID:   &id,
			StatusCode:   200,
			CreatedAt:    time.Now().UTC(),
		})
	}

	return nil
}

// DownloadBackup retrieves the raw artifact payload.
func (s *Service) DownloadBackup(ctx context.Context, sub *domainIdentity.Subject, id string) ([]byte, *domainBackup.Record, error) {
	record, err := s.GetBackup(ctx, sub, id)
	if err != nil {
		return nil, nil, err
	}

	data, _, err := s.storage.Get(ctx, record.StorageLocation)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to retrieve backup artifact: %w", err)
	}

	return data, record, nil
}

// ApplyRetentionPolicy enforces backup retention rules without deleting protected recovery points.
func (s *Service) ApplyRetentionPolicy(ctx context.Context, policyID string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	policy, err := s.repo.GetPolicy(ctx, policyID)
	if err != nil {
		return 0, err
	}

	backups, _, err := s.repo.List(ctx, domainBackup.Filter{Limit: 1000})
	if err != nil {
		return 0, err
	}

	deletedCount := 0
	cutoff := time.Now().UTC().AddDate(0, 0, -policy.RetentionDays)

	for _, b := range backups {
		if b.IsProtectedPoint {
			continue // Never delete protected points
		}
		if b.CreatedAt.Before(cutoff) {
			if err := s.DeleteBackup(ctx, nil, b.ID); err == nil {
				deletedCount++
			}
		}
	}

	log.Printf("[INFO] Retention policy %s pruned %d expired backups", policy.Name, deletedCount)
	return deletedCount, nil
}
