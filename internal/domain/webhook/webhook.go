package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

var (
	ErrWebhookNotFound      = errors.New("webhook endpoint not found")
	ErrInvalidWebhookURL    = errors.New("invalid webhook URL")
	ErrSSRFBlocked          = errors.New("webhook URL targets prohibited local or private network address")
	ErrDeliveryNotFound     = errors.New("webhook delivery record not found")
	ErrInvalidSignature     = errors.New("invalid webhook signature")
	ErrReplayAttackDetected = errors.New("webhook timestamp is outside valid replay window")
)

type DeliveryStatus string

const (
	DeliveryPending    DeliveryStatus = "pending"
	DeliveryDelivered  DeliveryStatus = "delivered"
	DeliveryFailed     DeliveryStatus = "failed"
	DeliveryDeadLetter DeliveryStatus = "dead_letter"
)

// WebhookEndpoint represents a customer-registered destination for HTTP event webhooks.
type WebhookEndpoint struct {
	ID             string     `json:"id"`
	TenantID       string     `json:"tenantId"`
	Name           string     `json:"name"`
	URL            string     `json:"url"`
	Description    string     `json:"description,omitempty"`
	Secret         string     `json:"-"` // Never serialized to JSON in list/get
	Active         bool       `json:"active"`
	EventTypes     []string   `json:"eventTypes"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	LastDeliveryAt *time.Time `json:"lastDeliveryAt,omitempty"`
	LastStatus     string     `json:"lastStatus,omitempty"`
	FailureCount   int        `json:"failureCount"`
}

// GenerateSecret generates a cryptographically secure 256-bit webhook signing secret.
func GenerateSecret() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return fmt.Sprintf("whsec_%s", hex.EncodeToString(b))
}

// RotateSecret replaces the endpoint signing secret with a fresh key and updates timestamp.
func (w *WebhookEndpoint) RotateSecret() string {
	newSecret := GenerateSecret()
	w.Secret = newSecret
	w.UpdatedAt = time.Now().UTC()
	return newSecret
}

// SubscribesTo checks whether the webhook is configured to receive a specific event type.
func (w *WebhookEndpoint) SubscribesTo(eventType string) bool {
	if !w.Active {
		return false
	}
	for _, et := range w.EventTypes {
		if et == "*" || et == eventType {
			return true
		}
		// Prefix wildcard e.g. "instance.*"
		if strings.HasSuffix(et, ".*") {
			prefix := strings.TrimSuffix(et, ".*")
			if strings.HasPrefix(eventType, prefix+".") {
				return true
			}
		}
	}
	return false
}

// SignPayload generates an HMAC-SHA256 digest of `${timestamp}.${payload}`.
func SignPayload(secret string, timestamp int64, payload []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	signedContent := fmt.Sprintf("%d.%s", timestamp, string(payload))
	mac.Write([]byte(signedContent))
	return hex.EncodeToString(mac.Sum(nil))
}

// VerifySignature validates that an incoming webhook signature matches the payload using constant-time comparison.
func VerifySignature(secret string, headerSignature string, timestamp int64, payload []byte, tolerance time.Duration) bool {
	// 1. Check replay window tolerance
	eventTime := time.Unix(timestamp, 0)
	now := time.Now().UTC()
	if math.Abs(now.Sub(eventTime).Seconds()) > tolerance.Seconds() {
		return false
	}

	expectedSig := SignPayload(secret, timestamp, payload)

	// Clean header if it has prefix "sha256="
	actualSig := headerSignature
	if strings.HasPrefix(actualSig, "sha256=") {
		actualSig = strings.TrimPrefix(actualSig, "sha256=")
	}

	return subtle.ConstantTimeCompare([]byte(expectedSig), []byte(actualSig)) == 1
}

// WebhookDelivery records the status and telemetry of an individual HTTP delivery attempt.
type WebhookDelivery struct {
	ID             string         `json:"id"`
	EventID        string         `json:"eventId"`
	WebhookID      string         `json:"webhookId"`
	TenantID       string         `json:"tenantId"`
	EventType      string         `json:"eventType"`
	Attempt        int            `json:"attempt"`
	Status         DeliveryStatus `json:"status"`
	HTTPStatus     int            `json:"httpStatus"`
	ResponseTimeMs int64          `json:"responseTimeMs"`
	Error          string         `json:"error,omitempty"`
	NextRetryAt    *time.Time     `json:"nextRetryAt,omitempty"`
	DeliveredAt    *time.Time     `json:"deliveredAt,omitempty"`
	CreatedAt      time.Time      `json:"createdAt"`
}

// WebhookFilter specifies filtering criteria for webhook endpoints.
type WebhookFilter struct {
	TenantID string `json:"tenantId,omitempty"`
	Active   *bool  `json:"active,omitempty"`
	Limit    int    `json:"limit"`
	Offset   int    `json:"offset"`
}

// DeliveryFilter specifies filtering criteria for webhook delivery audit logs.
type DeliveryFilter struct {
	TenantID  string          `json:"tenantId,omitempty"`
	WebhookID string          `json:"webhookId,omitempty"`
	EventID   string          `json:"eventId,omitempty"`
	Status    *DeliveryStatus `json:"status,omitempty"`
	Limit     int             `json:"limit"`
	Offset    int             `json:"offset"`
}

// WebhookRepository defines persistence operations for webhook endpoints.
type WebhookRepository interface {
	Create(ctx context.Context, endpoint *WebhookEndpoint) error
	GetByID(ctx context.Context, id string) (*WebhookEndpoint, error)
	List(ctx context.Context, filter WebhookFilter) ([]*WebhookEndpoint, int64, error)
	ListSubscribed(ctx context.Context, eventType string) ([]*WebhookEndpoint, error)
	Update(ctx context.Context, endpoint *WebhookEndpoint) error
	Delete(ctx context.Context, id string) error
	UpdateDeliveryStats(ctx context.Context, id string, lastStatus string, failureIncrement bool) error
}

// DeliveryRepository defines persistence operations for webhook delivery attempts.
type DeliveryRepository interface {
	Create(ctx context.Context, delivery *WebhookDelivery) error
	GetByID(ctx context.Context, id string) (*WebhookDelivery, error)
	List(ctx context.Context, filter DeliveryFilter) ([]*WebhookDelivery, int64, error)
	ListPendingRetries(ctx context.Context, before time.Time, limit int) ([]*WebhookDelivery, error)
	Update(ctx context.Context, delivery *WebhookDelivery) error
}
