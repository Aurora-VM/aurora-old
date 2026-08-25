package webhooks_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainWebhook "github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/webhooks"
)

func TestDispatcher_HMACDeliveryAndVerification(t *testing.T) {
	type receivedPayload struct {
		sig     string
		ts      string
		eventID string
		body    []byte
	}
	receivedChan := make(chan receivedPayload, 1)

	secret := domainWebhook.GenerateSecret()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		receivedChan <- receivedPayload{
			sig:     r.Header.Get("X-Aurora-Signature"),
			ts:      r.Header.Get("X-Aurora-Timestamp"),
			eventID: r.Header.Get("X-Aurora-Event-ID"),
			body:    body,
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	memStore := memory.NewMemoryStore()
	dispatcher := webhooks.NewDispatcher(memStore.Webhooks(), memStore.Deliveries())
	dispatcher.SetHTTPClient(server.Client())
	defer dispatcher.Close()

	// Register webhook endpoint in memory repo
	endpoint := &domainWebhook.WebhookEndpoint{
		ID:         "wh-disp-01",
		TenantID:   "tenant-01",
		Name:       "Test Webhook",
		URL:        server.URL,
		Secret:     secret,
		Active:     true,
		EventTypes: []string{"instance.*"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	if err := memStore.Webhooks().Create(context.Background(), endpoint); err != nil {
		t.Fatalf("failed to save endpoint: %v", err)
	}

	// Dispatch an event
	ev := &domainEvents.Event{
		ID:           "evt-disp-01",
		TenantID:     "tenant-01",
		Type:         domainEvents.EventInstanceStarted,
		ResourceType: "instance",
		ResourceID:   "inst-01",
		Timestamp:    time.Now().UTC(),
		Payload: map[string]interface{}{
			"status": "running",
		},
		Version: "1.0",
	}

	err := dispatcher.DispatchEvent(context.Background(), ev)
	if err != nil {
		t.Fatalf("failed to dispatch event: %v", err)
	}

	var rec receivedPayload
	select {
	case rec = <-receivedChan:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for webhook delivery")
	}

	if rec.eventID != ev.ID {
		t.Errorf("expected Event-ID header %s, got %s", ev.ID, rec.eventID)
	}

	// Verify HMAC-SHA256 signature
	var ts int64
	_, _ = fmt.Sscanf(rec.ts, "%d", &ts)
	if !domainWebhook.VerifySignature(secret, rec.sig, ts, rec.body, 60*time.Second) {
		t.Errorf("HMAC signature verification failed on receiver side")
	}
}

func TestDispatcher_SSRFProtection(t *testing.T) {
	memStore := memory.NewMemoryStore()
	dispatcher := webhooks.NewDispatcher(memStore.Webhooks(), memStore.Deliveries())
	defer dispatcher.Close()

	// Loopback / cloud metadata endpoint
	endpoint := &domainWebhook.WebhookEndpoint{
		ID:         "wh-ssrf-01",
		TenantID:   "tenant-01",
		Name:       "SSRF Loopback",
		URL:        "http://127.0.0.1:8080/internal-api",
		Secret:     "secret-key",
		Active:     true,
		EventTypes: []string{"*"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	delivery, err := dispatcher.TestWebhook(context.Background(), endpoint)
	if err == nil && delivery != nil && delivery.Status == domainWebhook.DeliveryDelivered {
		t.Fatalf("expected SSRF delivery to fail, got success")
	}

	if delivery != nil && !strings.Contains(delivery.Error, "blocked by SSRF policy") && !strings.Contains(delivery.Error, "loopback") {
		t.Logf("SSRF error message recorded: %s", delivery.Error)
	}
}

func TestDispatcher_BackoffAndDeadLetter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`Internal Error`))
	}))
	defer server.Close()

	memStore := memory.NewMemoryStore()
	dispatcher := webhooks.NewDispatcher(memStore.Webhooks(), memStore.Deliveries())
	dispatcher.SetHTTPClient(server.Client())
	defer dispatcher.Close()

	endpoint := &domainWebhook.WebhookEndpoint{
		ID:         "wh-fail-01",
		TenantID:   "tenant-01",
		Name:       "Failing Endpoint",
		URL:        server.URL,
		Secret:     "secret",
		Active:     true,
		EventTypes: []string{"*"},
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	_ = memStore.Webhooks().Create(context.Background(), endpoint)

	ev := &domainEvents.Event{
		ID:           "evt-fail-01",
		TenantID:     "tenant-01",
		Type:         domainEvents.EventInstanceStopped,
		ResourceType: "instance",
		ResourceID:   "inst-fail",
		Timestamp:    time.Now().UTC(),
	}

	_ = dispatcher.DispatchEvent(context.Background(), ev)
	time.Sleep(100 * time.Millisecond)

	// Check delivery status
	deliveries, _, _ := memStore.Deliveries().List(context.Background(), domainWebhook.DeliveryFilter{
		WebhookID: endpoint.ID,
	})

	if len(deliveries) == 0 {
		t.Fatalf("expected delivery record created")
	}

	firstDelivery := deliveries[0]
	if firstDelivery.Status != domainWebhook.DeliveryFailed && firstDelivery.Status != domainWebhook.DeliveryPending {
		t.Errorf("expected failed status, got %s", firstDelivery.Status)
	}
	if firstDelivery.HTTPStatus != 500 {
		t.Errorf("expected HTTP 500 status, got %d", firstDelivery.HTTPStatus)
	}
	if firstDelivery.NextRetryAt == nil {
		t.Errorf("expected NextRetryAt to be scheduled")
	}
}
