package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/audit"
	"github.com/aurora-vm/aurora/internal/app/authz"
	"github.com/aurora-vm/aurora/internal/app/events"
	appNotification "github.com/aurora-vm/aurora/internal/app/notification"
	appWebhook "github.com/aurora-vm/aurora/internal/app/webhook"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNotification "github.com/aurora-vm/aurora/internal/domain/notification"
	domainWebhook "github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/aurora-vm/aurora/internal/infra/email"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/webhooks"
	transportHTTP "github.com/aurora-vm/aurora/internal/transport/http"
)

type dummyTester struct{}

func (d *dummyTester) TestWebhook(ctx context.Context, endpoint *domainWebhook.WebhookEndpoint) (*domainWebhook.WebhookDelivery, error) {
	return &domainWebhook.WebhookDelivery{
		ID:         "del-test-01",
		WebhookID:  endpoint.ID,
		TenantID:   endpoint.TenantID,
		Status:     domainWebhook.DeliveryDelivered,
		HTTPStatus: 200,
	}, nil
}

func setupNotificationAndWebhookTest() (*memory.MemoryStore, *transportHTTP.NotificationHandler, *transportHTTP.WebhookHandler, *transportHTTP.EventHandler, *events.EventBus, *appNotification.Service, *appWebhook.Service) {
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	auditService := audit.NewService(memStore.Audit(), nil, nil, authorizer)

	eventBus := events.NewEventBus(memStore.Events(), 100, 2)
	emailProv := email.NewSimulatedEmailProvider()
	dispatcher := webhooks.NewDispatcher(memStore.Webhooks(), memStore.Deliveries())

	notifService := appNotification.NewService(memStore.Notifications(), memStore.Preferences(), emailProv, authorizer, auditService)
	webhookService := appWebhook.NewService(memStore.Webhooks(), memStore.Deliveries(), &dummyTester{}, authorizer, auditService)

	eventBus.Subscribe("*", notifService.HandleEvent)
	eventBus.Subscribe("*", dispatcher.DispatchEvent)

	notifHandler := transportHTTP.NewNotificationHandler(notifService, authorizer)
	webhookHandler := transportHTTP.NewWebhookHandler(webhookService, authorizer)
	eventHandler := transportHTTP.NewEventHandler(memStore.Events(), memStore.Deliveries(), authorizer)

	return memStore, notifHandler, webhookHandler, eventHandler, eventBus, notifService, webhookService
}

func TestNotification_LifecycleAndUnreadCount(t *testing.T) {
	memStore, notifHandler, _, _, eventBus, _, _ := setupNotificationAndWebhookTest()
	router := transportHTTP.NewRouter()

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sub := &identity.Subject{
				UserID:      "customer-tenant-01",
				Username:    "customer1",
				Roles:       []string{"customer"},
				Permissions: []string{"notification:read", "notification:manage"},
			}
			ctx := context.WithValue(r.Context(), transportHTTP.SubjectContextKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	notifHandler.RegisterRoutes(router, authMiddleware)

	// Publish domain events
	_ = eventBus.Publish(context.Background(), &domainEvents.Event{
		TenantID:     "customer-tenant-01",
		Type:         domainEvents.EventInstanceCreated,
		ResourceType: "instance",
		ResourceID:   "inst-01",
	})
	_ = eventBus.Publish(context.Background(), &domainEvents.Event{
		TenantID:     "customer-tenant-01",
		Type:         domainEvents.EventInvoiceCreated,
		ResourceType: "invoice",
		ResourceID:   "inv-01",
	})

	time.Sleep(50 * time.Millisecond)

	// 1. Check Unread Count
	req := httptest.NewRequest(http.MethodGet, "/api/v1/notifications/unread-count", nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}
	var countResp struct {
		Data struct {
			UnreadCount int64 `json:"unreadCount"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &countResp)
	if countResp.Data.UnreadCount != 2 {
		t.Errorf("expected 2 unread notifications, got %d", countResp.Data.UnreadCount)
	}

	// 2. List Notifications
	req = httptest.NewRequest(http.MethodGet, "/api/v1/notifications", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	var listResp struct {
		Data struct {
			Notifications []*domainNotification.Notification `json:"notifications"`
			Total         int64                              `json:"total"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &listResp)
	if listResp.Data.Total != 2 {
		t.Errorf("expected 2 notifications total, got %d", listResp.Data.Total)
	}

	// 3. Mark All Read
	req = httptest.NewRequest(http.MethodPost, "/api/v1/notifications/read-all", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on mark-all, got %d", rr.Code)
	}

	// 4. Verify unread count becomes 0
	count, _ := memStore.Notifications().GetUnreadCount(context.Background(), "customer-tenant-01")
	if count != 0 {
		t.Errorf("expected 0 unread notifications after read-all, got %d", count)
	}
}

func TestWebhook_CRUD_SecretRotation_And_SSRF(t *testing.T) {
	_, _, webhookHandler, _, _, _, _ := setupNotificationAndWebhookTest()
	router := transportHTTP.NewRouter()

	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sub := &identity.Subject{
				UserID:      "customer-tenant-01",
				Username:    "customer1",
				Roles:       []string{"customer"},
				Permissions: []string{"webhook:read", "webhook:create", "webhook:update", "webhook:delete", "webhook:rotate", "webhook:test"},
			}
			ctx := context.WithValue(r.Context(), transportHTTP.SubjectContextKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	webhookHandler.RegisterRoutes(router, authMiddleware)

	// 1. SSRF Attack Blocked
	ssrfPayload := map[string]interface{}{
		"name":       "SSRF Target",
		"url":        "http://127.0.0.1:8080/hook",
		"eventTypes": []string{"*"},
	}
	ssrfBytes, _ := json.Marshal(ssrfPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(ssrfBytes))
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected SSRF target to be blocked with 400 Bad Request, got %d", rr.Code)
	}

	// 2. Valid Webhook Creation (secret returned once)
	validPayload := map[string]interface{}{
		"name":        "Public API Endpoint",
		"url":         "https://api.github.com/webhook",
		"description": "Production event hook",
		"eventTypes":  []string{"instance.*", "billing.invoice.created"},
	}
	validBytes, _ := json.Marshal(validPayload)
	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(validBytes))
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rr.Code, rr.Body.String())
	}
	var createResp struct {
		Data struct {
			Endpoint *domainWebhook.WebhookEndpoint `json:"endpoint"`
			Secret   string                         `json:"secret"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &createResp)
	if createResp.Data.Secret == "" || !createResp.Data.Endpoint.Active {
		t.Fatalf("expected secret in creation response and active status")
	}
	webhookID := createResp.Data.Endpoint.ID

	// 3. List Webhooks (secret must NOT be present in JSON)
	req = httptest.NewRequest(http.MethodGet, "/api/v1/webhooks", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rr.Code)
	}
	if bytes.Contains(rr.Body.Bytes(), []byte(createResp.Data.Secret)) {
		t.Errorf("SECURITY FAULT: webhook secret leaked in list API response")
	}

	// 4. Rotate Secret
	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/rotate-secret", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on rotate-secret, got %d", rr.Code)
	}
	var rotResp struct {
		Data struct {
			Secret string `json:"secret"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &rotResp)
	if rotResp.Data.Secret == "" || rotResp.Data.Secret == createResp.Data.Secret {
		t.Errorf("expected fresh secret on rotation, got %s", rotResp.Data.Secret)
	}

	// 5. Test Webhook
	req = httptest.NewRequest(http.MethodPost, "/api/v1/webhooks/"+webhookID+"/test", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on test webhook, got %d", rr.Code)
	}
}
