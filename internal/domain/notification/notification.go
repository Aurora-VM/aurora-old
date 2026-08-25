package notification

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotificationNotFound = errors.New("notification not found")
	ErrInvalidPreference    = errors.New("invalid notification preference")
)

type Severity string

const (
	SeverityInfo     Severity = "info"
	SeveritySuccess  Severity = "success"
	SeverityWarning  Severity = "warning"
	SeverityError    Severity = "error"
	SeverityCritical Severity = "critical"
)

// Notification represents a user-facing in-app message.
type Notification struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenantId"`
	UserID       string     `json:"userId"`
	Type         string     `json:"type"` // e.g. "instance.created", "billing.invoice.created"
	Title        string     `json:"title"`
	Body         string     `json:"body"`
	Severity     Severity   `json:"severity"`
	ResourceType string     `json:"resourceType,omitempty"`
	ResourceID   string     `json:"resourceId,omitempty"`
	ReadAt       *time.Time `json:"readAt,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
}

func (n *Notification) IsRead() bool {
	return n.ReadAt != nil
}

// NotificationPreference controls which channels a user receives for an event type.
type NotificationPreference struct {
	UserID         string `json:"userId"`
	EventType      string `json:"eventType"`
	InAppEnabled   bool   `json:"inAppEnabled"`
	EmailEnabled   bool   `json:"emailEnabled"`
	WebhookEnabled bool   `json:"webhookEnabled"`
}

// Filter specifies querying options for notifications.
type Filter struct {
	TenantID   string    `json:"tenantId,omitempty"`
	UserID     string    `json:"userId,omitempty"`
	UnreadOnly bool      `json:"unreadOnly,omitempty"`
	Severity   *Severity `json:"severity,omitempty"`
	Limit      int       `json:"limit"`
	Offset     int       `json:"offset"`
}

// NotificationRepository defines persistence for user notifications.
type NotificationRepository interface {
	Create(ctx context.Context, n *Notification) error
	GetByID(ctx context.Context, id string) (*Notification, error)
	List(ctx context.Context, filter Filter) ([]*Notification, int64, error)
	MarkAsRead(ctx context.Context, id string, userID string) error
	MarkAllAsRead(ctx context.Context, userID string) (int64, error)
	GetUnreadCount(ctx context.Context, userID string) (int64, error)
}

// PreferenceRepository defines persistence for user notification preferences.
type PreferenceRepository interface {
	GetPreferences(ctx context.Context, userID string) ([]*NotificationPreference, error)
	GetPreference(ctx context.Context, userID string, eventType string) (*NotificationPreference, error)
	SetPreference(ctx context.Context, pref *NotificationPreference) error
}
