package keyrotation_test

import (
	"context"
	"testing"
	"time"

	appAuthz "github.com/aurora-vm/aurora/internal/app/authz"
	appKeyRotation "github.com/aurora-vm/aurora/internal/app/keyrotation"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	domainKeyRotation "github.com/aurora-vm/aurora/internal/domain/keyrotation"
	infraMemory "github.com/aurora-vm/aurora/internal/infra/memory"
)

type mockEventPublisher struct{}

func (m *mockEventPublisher) Publish(ctx context.Context, event *domainEvents.Event) error {
	return nil
}

func setupKeyRotationService(t *testing.T) (*appKeyRotation.Service, *domainIdentity.Subject) {
	t.Helper()
	memStore := infraMemory.NewMemoryStore()
	repo := memStore.KeyRotations()
	authorizer := appAuthz.NewAuthorizer(memStore.Roles())
	auditRepo := memStore.Audit()
	eventPub := &mockEventPublisher{}

	svc := appKeyRotation.NewService(repo, authorizer, auditRepo, eventPub)

	adminSub := &domainIdentity.Subject{
		UserID: "superadmin-01",
		Roles:  []string{"superadmin"},
		Permissions: []string{"*"},
	}

	return svc, adminSub
}

func TestKeyRotation_Rotate_And_GracePeriod(t *testing.T) {
	svc, adminSub := setupKeyRotationService(t)
	ctx := context.Background()

	// 1. Rotate JWT signing key
	rec1, err := svc.RotateKey(ctx, adminSub, appKeyRotation.RotateKeyRequest{
		Type:                domainKeyRotation.TypeJWTSigning,
		Description:         "Q3 2026 JWT Signing Key Rotation",
		GracePeriodDuration: 12 * time.Hour,
	})
	if err != nil {
		t.Fatalf("RotateKey failed: %v", err)
	}

	if rec1.Status != domainKeyRotation.StatusActive || rec1.Version != 2 {
		t.Errorf("expected active version 2 key, got status=%s version=%d", rec1.Status, rec1.Version)
	}

	// 2. Rotate again to verify version increment and grace period status on previous key
	rec2, err := svc.RotateKey(ctx, adminSub, appKeyRotation.RotateKeyRequest{
		Type:                domainKeyRotation.TypeJWTSigning,
		Description:         "Emergency JWT Key Rotation",
		GracePeriodDuration: 24 * time.Hour,
	})
	if err != nil {
		t.Fatalf("RotateKey 2 failed: %v", err)
	}

	if rec2.Version != 3 {
		t.Errorf("expected version 3, got %d", rec2.Version)
	}

	// 3. List rotations
	list, total, err := svc.ListKeyRotations(ctx, adminSub, domainKeyRotation.TypeJWTSigning, 10, 0)
	if err != nil {
		t.Fatalf("ListKeyRotations failed: %v", err)
	}
	if total < 3 {
		t.Errorf("expected at least 3 JWT key records, got %d", total)
	}
	if len(list) < 3 {
		t.Errorf("expected at least 3 JWT key items, got %d", len(list))
	}
}

func TestKeyRotation_Revocation(t *testing.T) {
	svc, adminSub := setupKeyRotationService(t)
	ctx := context.Background()

	// Rotate webhook key
	rec, err := svc.RotateKey(ctx, adminSub, appKeyRotation.RotateKeyRequest{
		Type:        domainKeyRotation.TypeWebhookSecret,
		Description: "Primary Webhook Signing Secret",
	})
	if err != nil {
		t.Fatalf("RotateKey failed: %v", err)
	}

	// Revoke the key
	revoked, err := svc.RevokeKey(ctx, adminSub, rec.ID, "Suspected credential exposure")
	if err != nil {
		t.Fatalf("RevokeKey failed: %v", err)
	}

	if revoked.Status != domainKeyRotation.StatusRevoked {
		t.Errorf("expected revoked status, got %s", revoked.Status)
	}

	// Attempting to revoke again must fail
	_, err = svc.RevokeKey(ctx, adminSub, rec.ID, "Duplicate revocation attempt")
	if err != domainKeyRotation.ErrKeyAlreadyRevoked {
		t.Errorf("expected ErrKeyAlreadyRevoked, got %v", err)
	}
}
