package backup_test

import (
	"context"
	"testing"

	appAuthz "github.com/aurora-vm/aurora/internal/app/authz"
	appBackup "github.com/aurora-vm/aurora/internal/app/backup"
	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	infraMemory "github.com/aurora-vm/aurora/internal/infra/memory"
	infraStorage "github.com/aurora-vm/aurora/internal/infra/storage"
)

type mockEventPublisher struct{}

func (m *mockEventPublisher) Publish(ctx context.Context, event *domainEvents.Event) error {
	return nil
}

func setupBackupService(t *testing.T) (*appBackup.Service, *infraMemory.MemoryBackupRepo, *infraStorage.MemoryObjectStorage) {
	t.Helper()
	repo := infraMemory.NewMemoryBackupRepo()
	memStorage := infraStorage.NewMemoryObjectStorage()
	encStorage, err := infraStorage.NewEncryptedStorageWrapper(memStorage, []byte("super-secret-32-byte-key-aurora"))
	if err != nil {
		t.Fatalf("failed to create encrypted storage: %v", err)
	}

	memStore := infraMemory.NewMemoryStore()
	authorizer := appAuthz.NewAuthorizer(memStore.Roles())
	auditRepo := memStore.Audit()
	eventPub := &mockEventPublisher{}

	svc := appBackup.NewService(repo, encStorage, authorizer, auditRepo, eventPub)
	return svc, repo, memStorage
}

func TestBackup_Create_Verify_And_EncryptedAtRest(t *testing.T) {
	svc, _, _ := setupBackupService(t)
	ctx := context.Background()

	adminSub := &domainIdentity.Subject{
		UserID: "superadmin-01",
		Roles:  []string{"superadmin"},
		Permissions: []string{"*"},
	}

	// 1. Create a cluster database backup
	rec, err := svc.CreateBackup(ctx, adminSub, appBackup.CreateBackupRequest{
		TenantID:     "system",
		ResourceType: "database",
		Type:         domainBackup.TypeFull,
		Metadata:     map[string]interface{}{"description": "Cluster database pre-upgrade backup"},
	})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	if rec.ID == "" || rec.ChecksumSHA256 == "" {
		t.Fatalf("expected valid backup record, got %+v", rec)
	}
	if rec.Status != domainBackup.StatusVerified {
		t.Errorf("expected verified status, got %s", rec.Status)
	}
	if !rec.IsProtectedPoint {
		t.Errorf("first verified backup should be protected recovery point")
	}

	// 2. Download and verify payload
	data, downloadedRec, err := svc.DownloadBackup(ctx, adminSub, rec.ID)
	if err != nil {
		t.Fatalf("DownloadBackup failed: %v", err)
	}
	if len(data) == 0 || downloadedRec.ID != rec.ID {
		t.Errorf("invalid downloaded payload")
	}

	// 3. Verify integrity
	if err := svc.VerifyBackup(ctx, adminSub, rec.ID); err != nil {
		t.Errorf("VerifyBackup failed: %v", err)
	}
}

func TestBackup_CorruptedArtifact_Rejection(t *testing.T) {
	svc, _, rawStorage := setupBackupService(t)
	ctx := context.Background()

	adminSub := &domainIdentity.Subject{
		UserID: "superadmin-01",
		Roles:  []string{"superadmin"},
		Permissions: []string{"*"},
	}

	rec, err := svc.CreateBackup(ctx, adminSub, appBackup.CreateBackupRequest{
		TenantID:     "system",
		ResourceType: "cluster",
		Type:         domainBackup.TypeFull,
	})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Artificially corrupt the raw stored ciphertext bytes
	rawStorage.CorruptArtifactForTesting(rec.StorageLocation)

	// Verification should now fail and reject the backup
	err = svc.VerifyBackup(ctx, adminSub, rec.ID)
	if err == nil {
		t.Fatalf("expected error on corrupted artifact verification, got nil")
	}

	// State should be failed
	updated, _ := svc.GetBackup(ctx, adminSub, rec.ID)
	if updated.Status != domainBackup.StatusFailed {
		t.Errorf("expected status failed, got %s", updated.Status)
	}
}

func TestBackup_CannotDeleteLastGoodBackupProtection(t *testing.T) {
	svc, repo, _ := setupBackupService(t)
	ctx := context.Background()

	adminSub := &domainIdentity.Subject{
		UserID: "superadmin-01",
		Roles:  []string{"superadmin"},
		Permissions: []string{"*"},
	}

	// Create sole verified backup
	rec, err := svc.CreateBackup(ctx, adminSub, appBackup.CreateBackupRequest{
		TenantID:     "system",
		ResourceType: "database",
		Type:         domainBackup.TypeFull,
	})
	if err != nil {
		t.Fatalf("CreateBackup failed: %v", err)
	}

	// Attempting to delete the only verified recovery point must fail
	err = svc.DeleteBackup(ctx, adminSub, rec.ID)
	if err != domainBackup.ErrCannotDeleteLastGoodBackup {
		t.Fatalf("expected ErrCannotDeleteLastGoodBackup, got %v", err)
	}

	// Create a second verified backup
	rec2, err := svc.CreateBackup(ctx, adminSub, appBackup.CreateBackupRequest{
		TenantID:     "system",
		ResourceType: "database",
		Type:         domainBackup.TypeFull,
	})
	if err != nil || rec2 == nil {
		t.Fatalf("CreateBackup 2 failed: %v", err)
	}

	// Unset protected point on rec to allow deletion
	_ = repo.SetProtectedPoint(ctx, rec.ID, false)

	// Deleting rec should now succeed because rec2 exists
	if err := svc.DeleteBackup(ctx, adminSub, rec.ID); err != nil {
		t.Errorf("expected successful deletion of non-final backup, got %v", err)
	}
}

func TestBackup_TenantIsolation(t *testing.T) {
	svc, _, _ := setupBackupService(t)
	ctx := context.Background()

	tenantASub := &domainIdentity.Subject{
		UserID: "tenant-aaa",
		Roles:  []string{"customer"},
		Permissions: []string{"backup:read", "backup:create", "backup:restore"},
	}
	tenantBSub := &domainIdentity.Subject{
		UserID: "tenant-bbb",
		Roles:  []string{"customer"},
		Permissions: []string{"backup:read", "backup:create", "backup:restore"},
	}

	// Tenant A creates backup
	recA, err := svc.CreateBackup(ctx, tenantASub, appBackup.CreateBackupRequest{
		ResourceType: "instance",
		ResourceID:   "inst-tenant-a",
		Type:         domainBackup.TypePointInTime,
	})
	if err != nil {
		t.Fatalf("Tenant A backup failed: %v", err)
	}

	// Tenant B attempts to read Tenant A's backup -> should be forbidden or not found
	_, err = svc.GetBackup(ctx, tenantBSub, recA.ID)
	if err == nil {
		t.Errorf("expected tenant isolation authorization error for Tenant B, got nil")
	}

	// Tenant B lists backups -> Tenant A's backup must not appear
	bList, total, err := svc.ListBackups(ctx, tenantBSub, domainBackup.Filter{Limit: 50})
	if err != nil {
		t.Fatalf("ListBackups for Tenant B failed: %v", err)
	}
	if total != 0 || len(bList) != 0 {
		t.Errorf("expected 0 backups for Tenant B, got %d", total)
	}
}
