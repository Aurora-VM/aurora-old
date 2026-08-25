package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	appBackup "github.com/aurora-vm/aurora/internal/app/backup"
	appReconcile "github.com/aurora-vm/aurora/internal/app/reconcile"
	appRecovery "github.com/aurora-vm/aurora/internal/app/recovery"
	domainBackup "github.com/aurora-vm/aurora/internal/domain/backup"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainIdentity "github.com/aurora-vm/aurora/internal/domain/identity"
	infraMemory "github.com/aurora-vm/aurora/internal/infra/memory"
	infraStorage "github.com/aurora-vm/aurora/internal/infra/storage"
	transportHTTP "github.com/aurora-vm/aurora/internal/transport/http"
	"github.com/go-chi/chi/v5"
)

type mockEventPub struct{}

func (m *mockEventPub) Publish(ctx context.Context, event *domainEvents.Event) error {
	return nil
}

func setupTestRouter(t *testing.T) (chi.Router, *infraMemory.MemoryStore, *domainIdentity.Subject, *domainIdentity.Subject) {
	t.Helper()
	memStore := infraMemory.NewMemoryStore()
	backupRepo := memStore.Backups()
	reconcileRepo := memStore.Reconcile()
	instRepo := memStore.Instances()
	nodeRepo := memStore.Nodes()
	jobRepo := memStore.Jobs()
	migRepo := memStore.Migrations()
	quotaRepo := memStore.Quotas()
	auditRepo := &testAuditRepo{}
	authorizer := &testAuthorizer{}

	memStorage := infraStorage.NewMemoryObjectStorage()
	encStorage, _ := infraStorage.NewEncryptedStorageWrapper(memStorage, []byte("super-secret-32-byte-key-aurora"))
	eventPub := &mockEventPub{}

	backupSvc := appBackup.NewService(backupRepo, encStorage, authorizer, auditRepo, eventPub)
	reconcileSvc := appReconcile.NewService(reconcileRepo, instRepo, nodeRepo, jobRepo, migRepo, quotaRepo, authorizer, auditRepo)
	coord := appRecovery.NewCoordinator(backupSvc, reconcileSvc, backupRepo, auditRepo, eventPub, authorizer)

	r := transportHTTP.NewRouter()

	adminSub := &domainIdentity.Subject{
		UserID:      "admin-01",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	customerSub := &domainIdentity.Subject{
		UserID:      "customer-01",
		Roles:       []string{"customer"},
		Permissions: []string{"backup:read", "backup:create", "backup:restore"},
	}

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			token := req.Header.Get("X-User-Role")
			sub := customerSub
			if token == "superadmin" {
				sub = adminSub
			}
			ctx := context.WithValue(req.Context(), transportHTTP.SubjectContextKey, sub)
			next.ServeHTTP(w, req.WithContext(ctx))
		})
	}

	backupHandler := transportHTTP.NewBackupHandler(backupSvc, coord, authorizer)
	backupHandler.RegisterRoutes(r, authMiddleware)

	reconcileHandler := transportHTTP.NewReconcileHandler(reconcileSvc, authorizer)
	reconcileHandler.RegisterRoutes(r, authMiddleware)

	return r, memStore, adminSub, customerSub
}

func TestHTTP_Backup_And_Recovery_Workflow(t *testing.T) {
	r, _, _, _ := setupTestRouter(t)

	// 1. Admin creates cluster backup
	createBody, _ := json.Marshal(appBackup.CreateBackupRequest{
		ResourceType: "cluster",
		Type:         domainBackup.TypeFull,
	})
	req, _ := http.NewRequest("POST", "/api/v1/admin/recovery/backups", bytes.NewReader(createBody))
	req.Header.Set("X-User-Role", "superadmin")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data domainBackup.Record `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	backupID := resp.Data.ID
	if backupID == "" {
		t.Fatalf("expected backup ID")
	}

	// 2. Admin triggers dry-run disaster recovery
	dryRunBody, _ := json.Marshal(map[string]interface{}{
		"backupId": backupID,
	})
	req, _ = http.NewRequest("POST", "/api/v1/admin/recovery/dry-run", bytes.NewReader(dryRunBody))
	req.Header.Set("X-User-Role", "superadmin")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for dry-run, got %d: %s", rr.Code, rr.Body.String())
	}

	// 3. Admin verifies backup integrity
	req, _ = http.NewRequest("POST", "/api/v1/admin/recovery/backups/"+backupID+"/verify", nil)
	req.Header.Set("X-User-Role", "superadmin")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for verify, got %d: %s", rr.Code, rr.Body.String())
	}

	// 4. Admin triggers full disaster recovery
	restoreBody, _ := json.Marshal(map[string]interface{}{
		"backupId":    backupID,
		"confirmedDr": true,
	})
	req, _ = http.NewRequest("POST", "/api/v1/admin/recovery/restore", bytes.NewReader(restoreBody))
	req.Header.Set("X-User-Role", "superadmin")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for live restore, got %d: %s", rr.Code, rr.Body.String())
	}

	// 5. Customer creates and lists tenant backup
	custCreateBody, _ := json.Marshal(appBackup.CreateBackupRequest{
		ResourceType: "instance",
		ResourceID:   "inst-customer-01",
		Type:         domainBackup.TypePointInTime,
	})
	req, _ = http.NewRequest("POST", "/api/v1/backups", bytes.NewReader(custCreateBody))
	req.Header.Set("X-User-Role", "customer")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for customer backup, got %d: %s", rr.Code, rr.Body.String())
	}

	req, _ = http.NewRequest("GET", "/api/v1/backups", nil)
	req.Header.Set("X-User-Role", "customer")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for customer backup list, got %d: %s", rr.Code, rr.Body.String())
	}
}
