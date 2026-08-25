package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"sync"
	"time"

	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	domainWebhook "github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/aurora-vm/aurora/internal/infra/ssrf"
	"github.com/google/uuid"
)

const MaxDeliveryAttempts = 6

// Retry intervals for attempts 2..6 (attempt 1 is immediate)
var retryIntervals = []time.Duration{
	0,                   // Attempt 1 (immediate)
	5 * time.Second,     // Attempt 2
	30 * time.Second,    // Attempt 3
	5 * time.Minute,     // Attempt 4
	30 * time.Minute,    // Attempt 5
	2 * time.Hour,       // Attempt 6
}

// Dispatcher manages HTTP deliveries, signatures, and retries for webhook endpoints.
type Dispatcher struct {
	webhookRepo  domainWebhook.WebhookRepository
	deliveryRepo domainWebhook.DeliveryRepository
	httpClient   *http.Client
	skipSSRF     bool
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
}

// NewDispatcher constructs a Webhook Dispatcher with SSRF-safe HTTP transport.
func NewDispatcher(webhookRepo domainWebhook.WebhookRepository, deliveryRepo domainWebhook.DeliveryRepository) *Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())

	d := &Dispatcher{
		webhookRepo:  webhookRepo,
		deliveryRepo: deliveryRepo,
		httpClient:   ssrf.NewSafeHTTPClient(15 * time.Second),
		skipSSRF:     false,
		ctx:          ctx,
		cancel:       cancel,
	}

	// Start background retry worker
	d.wg.Add(1)
	go d.retryWorker()

	return d
}

// SetHTTPClient overrides the internal HTTP client (used for integration tests).
func (d *Dispatcher) SetHTTPClient(client *http.Client) {
	d.httpClient = client
	d.skipSSRF = true
}

// DispatchEvent routes a domain event to all subscribed webhook endpoints of the tenant.
func (d *Dispatcher) DispatchEvent(ctx context.Context, event *domainEvents.Event) error {
	endpoints, err := d.webhookRepo.ListSubscribed(ctx, string(event.Type))
	if err != nil {
		return err
	}

	for _, ep := range endpoints {
		// Strict tenant isolation: only deliver if tenant matches or endpoint is global
		if ep.TenantID != event.TenantID {
			continue
		}

		go func(endpoint *domainWebhook.WebhookEndpoint) {
			_, _ = d.deliver(context.Background(), endpoint, event, 1)
		}(ep)
	}

	return nil
}

// TestWebhook sends a test ping event to verify connectivity and signature verification.
func (d *Dispatcher) TestWebhook(ctx context.Context, endpoint *domainWebhook.WebhookEndpoint) (*domainWebhook.WebhookDelivery, error) {
	testEvent := &domainEvents.Event{
		ID:           uuid.NewString(),
		TenantID:     endpoint.TenantID,
		Type:         "webhook.test",
		ResourceType: "webhook",
		ResourceID:   endpoint.ID,
		ActorID:      "user",
		Timestamp:    time.Now().UTC(),
		Payload: map[string]interface{}{
			"message": "Project Aurora Webhook Verification Ping",
			"url":     endpoint.URL,
		},
		Version: "1.0",
	}

	return d.deliver(ctx, endpoint, testEvent, 1)
}

func (d *Dispatcher) deliver(ctx context.Context, endpoint *domainWebhook.WebhookEndpoint, event *domainEvents.Event, attempt int) (*domainWebhook.WebhookDelivery, error) {
	now := time.Now().UTC()
	delivery := &domainWebhook.WebhookDelivery{
		ID:        uuid.NewString(),
		EventID:   event.ID,
		WebhookID: endpoint.ID,
		TenantID:  endpoint.TenantID,
		EventType: string(event.Type),
		Attempt:   attempt,
		Status:    domainWebhook.DeliveryPending,
		CreatedAt: now,
	}

	// 1. SSRF URL Check
	if !d.skipSSRF {
		if err := ssrf.ValidateURL(endpoint.URL); err != nil {
			delivery.Status = domainWebhook.DeliveryDeadLetter
			delivery.Error = fmt.Sprintf("SSRF blocked: %v", err)
			_ = d.deliveryRepo.Create(ctx, delivery)
			_ = d.webhookRepo.UpdateDeliveryStats(ctx, endpoint.ID, "blocked", true)
			return delivery, err
		}
	}

	// 2. Prepare JSON Payload
	bodyJSON, err := json.Marshal(event)
	if err != nil {
		delivery.Status = domainWebhook.DeliveryDeadLetter
		delivery.Error = fmt.Sprintf("serialization error: %v", err)
		_ = d.deliveryRepo.Create(ctx, delivery)
		return delivery, err
	}

	// 3. Compute HMAC-SHA256 Signature
	timestamp := time.Now().UTC().Unix()
	signature := domainWebhook.SignPayload(endpoint.Secret, timestamp, bodyJSON)

	// 4. Construct HTTP Request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.URL, bytes.NewReader(bodyJSON))
	if err != nil {
		delivery.Status = domainWebhook.DeliveryDeadLetter
		delivery.Error = fmt.Sprintf("invalid request: %v", err)
		_ = d.deliveryRepo.Create(ctx, delivery)
		return delivery, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "Aurora-Webhook-Dispatcher/1.0")
	req.Header.Set("X-Aurora-Event-ID", event.ID)
	req.Header.Set("X-Aurora-Event-Type", string(event.Type))
	req.Header.Set("X-Aurora-Timestamp", fmt.Sprintf("%d", timestamp))
	req.Header.Set("X-Aurora-Signature", fmt.Sprintf("sha256=%s", signature))

	// 5. Execute HTTP Delivery
	start := time.Now()
	resp, reqErr := d.httpClient.Do(req)
	delivery.ResponseTimeMs = time.Since(start).Milliseconds()

	if reqErr != nil {
		delivery.Error = reqErr.Error()
		d.handleFailure(ctx, endpoint, delivery, 0)
		_ = d.deliveryRepo.Create(ctx, delivery)
		return delivery, reqErr
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)

	delivery.HTTPStatus = resp.StatusCode

	// Success on 2xx
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		deliveredAt := time.Now().UTC()
		delivery.Status = domainWebhook.DeliveryDelivered
		delivery.DeliveredAt = &deliveredAt
		_ = d.deliveryRepo.Create(ctx, delivery)
		_ = d.webhookRepo.UpdateDeliveryStats(ctx, endpoint.ID, "success", false)
		return delivery, nil
	}

	// Non-2xx response
	delivery.Error = fmt.Sprintf("endpoint returned HTTP %d", resp.StatusCode)
	d.handleFailure(ctx, endpoint, delivery, resp.StatusCode)
	_ = d.deliveryRepo.Create(ctx, delivery)
	return delivery, fmt.Errorf("webhook delivery failed with status %d", resp.StatusCode)
}

func (d *Dispatcher) handleFailure(ctx context.Context, endpoint *domainWebhook.WebhookEndpoint, delivery *domainWebhook.WebhookDelivery, statusCode int) {
	_ = d.webhookRepo.UpdateDeliveryStats(ctx, endpoint.ID, "failure", true)

	// Don't retry non-retryable 4xx errors (except 408 Request Timeout and 429 Too Many Requests)
	if statusCode >= 400 && statusCode < 500 && statusCode != 408 && statusCode != 429 {
		delivery.Status = domainWebhook.DeliveryDeadLetter
		return
	}

	// If reached max attempts, mark dead-letter
	if delivery.Attempt >= MaxDeliveryAttempts {
		delivery.Status = domainWebhook.DeliveryDeadLetter
		return
	}

	// Calculate exponential backoff interval + jitter
	baseInterval := retryIntervals[delivery.Attempt]
	jitter := time.Duration(rand.Int63n(int64(baseInterval / 5))) // 20% jitter
	nextRetry := time.Now().UTC().Add(baseInterval + jitter)

	delivery.Status = domainWebhook.DeliveryPending
	delivery.NextRetryAt = &nextRetry
}

func (d *Dispatcher) retryWorker() {
	defer d.wg.Done()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.processPendingRetries()
		}
	}
}

func (d *Dispatcher) processPendingRetries() {
	now := time.Now().UTC()
	retries, err := d.deliveryRepo.ListPendingRetries(d.ctx, now, 20)
	if err != nil || len(retries) == 0 {
		return
	}

	for _, delivery := range retries {
		endpoint, err := d.webhookRepo.GetByID(d.ctx, delivery.WebhookID)
		if err != nil || !endpoint.Active {
			delivery.Status = domainWebhook.DeliveryDeadLetter
			delivery.Error = "webhook endpoint no longer active"
			_ = d.deliveryRepo.Update(d.ctx, delivery)
			continue
		}

		event := &domainEvents.Event{
			ID:        delivery.EventID,
			TenantID:  delivery.TenantID,
			Type:      domainEvents.EventType(delivery.EventType),
			Timestamp: delivery.CreatedAt,
			Version:   "1.0",
		}

		go func(ep *domainWebhook.WebhookEndpoint, del *domainWebhook.WebhookDelivery, ev *domainEvents.Event) {
			newDel, _ := d.deliver(d.ctx, ep, ev, del.Attempt+1)
			del.Attempt = newDel.Attempt
			del.Status = newDel.Status
			del.HTTPStatus = newDel.HTTPStatus
			del.ResponseTimeMs = newDel.ResponseTimeMs
			del.Error = newDel.Error
			del.NextRetryAt = newDel.NextRetryAt
			del.DeliveredAt = newDel.DeliveredAt
			_ = d.deliveryRepo.Update(d.ctx, del)
		}(endpoint, delivery, event)
	}
}

// Close terminates the retry worker cleanly.
func (d *Dispatcher) Close() {
	d.cancel()
	d.wg.Wait()
}
