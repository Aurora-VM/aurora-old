package keyrotation

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	appBackup "github.com/aurora-vm/aurora/internal/app/backup"
	domainAudit "github.com/aurora-vm/aurora/internal/domain/audit"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainKeyRotation "github.com/aurora-vm/aurora/internal/domain/keyrotation"
	"github.com/google/uuid"
)

// Service manages cryptographic credentials and key rotation lifecycles.
type Service struct {
	repo           domainKeyRotation.Repository
	authorizer     domainIdentity.Authorizer
	auditRepo      appBackup.AuditRecorder
	eventPublisher appBackup.EventPublisher
	mu             sync.Mutex
}

func NewService(
	repo domainKeyRotation.Repository,
	authorizer domainIdentity.Authorizer,
	auditRepo appBackup.AuditRecorder,
	eventPublisher appBackup.EventPublisher,
) *Service {
	return &Service{
		repo:           repo,
		authorizer:     authorizer,
		auditRepo:      auditRepo,
		eventPublisher: eventPublisher,
	}
}

// RotateKeyRequest specifies rotation parameters.
type RotateKeyRequest struct {
	Type                domainKeyRotation.KeyType `json:"type"`
	Description         string                    `json:"description,omitempty"`
	GracePeriodDuration time.Duration             `json:"gracePeriodDuration,omitempty"` // e.g. 24h for dual verification
}

// RotateKey creates a new active cryptographic key version and transitions old key to a grace period.
func (s *Service) RotateKey(ctx context.Context, sub *domainIdentity.Subject, req RotateKeyRequest) (*domainKeyRotation.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		return nil, domainIdentity.ErrResourceForbidden
	}

	actorID := "system"
	if sub != nil {
		actorID = sub.UserID
	}

	if req.GracePeriodDuration <= 0 {
		req.GracePeriodDuration = 24 * time.Hour
	}

	// 1. Check existing active key
	prevActive, _ := s.repo.GetActive(ctx, req.Type)
	newVersion := 1
	algorithm := "HS256"
	switch req.Type {
	case domainKeyRotation.TypeJWTSigning:
		algorithm = "HS256"
	case domainKeyRotation.TypeWebhookSecret:
		algorithm = "HMAC-SHA256"
	case domainKeyRotation.TypeNodeMTLSCert:
		algorithm = "ECDSA-P256"
	case domainKeyRotation.TypeBackupEncryption:
		algorithm = "AES-GCM-256"
	case domainKeyRotation.TypeDBCredential:
		algorithm = "SCRAM-SHA-256"
	}

	if prevActive != nil {
		newVersion = prevActive.Version + 1
		graceExpiry := time.Now().UTC().Add(req.GracePeriodDuration)
		prevActive.Status = domainKeyRotation.StatusGracePeriod
		prevActive.GracePeriodExpiresAt = &graceExpiry
		_ = s.repo.Save(ctx, prevActive)
	}

	newKeyRecord := &domainKeyRotation.Record{
		ID:          uuid.NewString(),
		Type:        req.Type,
		KeyID:       fmt.Sprintf("%s-v%d-%s", req.Type, newVersion, uuid.NewString()[:8]),
		Status:      domainKeyRotation.StatusActive,
		Version:     newVersion,
		Algorithm:   algorithm,
		Description: req.Description,
		RotatedBy:   actorID,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := s.repo.Save(ctx, newKeyRecord); err != nil {
		return nil, fmt.Errorf("failed to persist key rotation record: %w", err)
	}

	// Audit Log
	if s.auditRepo != nil {
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "key.rotated",
			ResourceType: "security_key",
			ResourceID:   &newKeyRecord.KeyID,
			StatusCode:   200,
			Details: map[string]interface{}{
				"keyType":     req.Type,
				"version":     newVersion,
				"algorithm":   algorithm,
				"gracePeriod": req.GracePeriodDuration.String(),
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	// Event
	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			ID:           uuid.NewString(),
			TenantID:     "system",
			Type:         domainEvents.EventKeyRotated,
			ResourceType: "security_key",
			ResourceID:   newKeyRecord.KeyID,
			Timestamp:    time.Now().UTC(),
			Version:      "1.0",
		})
	}

	log.Printf("[INFO] Cryptographic key %s rotated to version %d (Key ID: %s)", req.Type, newVersion, newKeyRecord.KeyID)
	return newKeyRecord, nil
}

// RevokeKey immediately revokes a compromised cryptographic credential.
func (s *Service) RevokeKey(ctx context.Context, sub *domainIdentity.Subject, id, reason string) (*domainKeyRotation.Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		return nil, domainIdentity.ErrResourceForbidden
	}

	actorID := "system"
	if sub != nil {
		actorID = sub.UserID
	}

	rec, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if rec.Status == domainKeyRotation.StatusRevoked {
		return nil, domainKeyRotation.ErrKeyAlreadyRevoked
	}

	if reason == "" {
		reason = "Revoked by security administrator"
	}

	if err := s.repo.UpdateStatus(ctx, id, domainKeyRotation.StatusRevoked, reason); err != nil {
		return nil, err
	}

	rec.Status = domainKeyRotation.StatusRevoked
	rec.RevocationReason = reason

	// Audit log
	if s.auditRepo != nil {
		_ = s.auditRepo.Record(ctx, &domainAudit.AuditLog{
			ActorID:      &actorID,
			Action:       "key.revoked",
			ResourceType: "security_key",
			ResourceID:   &rec.KeyID,
			StatusCode:   200,
			Details: map[string]interface{}{
				"keyType": rec.Type,
				"reason":  reason,
			},
			CreatedAt: time.Now().UTC(),
		})
	}

	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			ID:           uuid.NewString(),
			TenantID:     "system",
			Type:         domainEvents.EventKeyRevoked,
			ResourceType: "security_key",
			ResourceID:   rec.KeyID,
			Timestamp:    time.Now().UTC(),
			Version:      "1.0",
		})
	}

	log.Printf("[WARN] Cryptographic key %s revoked (Key ID: %s, Reason: %s)", rec.Type, rec.KeyID, reason)
	return rec, nil
}

// ListKeyRotations queries rotation history.
func (s *Service) ListKeyRotations(ctx context.Context, sub *domainIdentity.Subject, keyType domainKeyRotation.KeyType, limit, offset int) ([]*domainKeyRotation.Record, int, error) {
	if s.authorizer != nil && sub != nil && !sub.IsSuperadmin() {
		return nil, 0, domainIdentity.ErrResourceForbidden
	}
	return s.repo.List(ctx, keyType, limit, offset)
}
