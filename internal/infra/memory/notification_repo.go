package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/notification"
	"github.com/google/uuid"
)

// MemoryNotificationRepo implements notification.NotificationRepository in-memory.
type MemoryNotificationRepo struct {
	mu            sync.RWMutex
	notifications map[string]*notification.Notification
	order         []string
}

func NewMemoryNotificationRepo() *MemoryNotificationRepo {
	return &MemoryNotificationRepo{
		notifications: make(map[string]*notification.Notification),
		order:         make([]string, 0),
	}
}

func (r *MemoryNotificationRepo) Create(ctx context.Context, n *notification.Notification) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if n.ID == "" {
		n.ID = uuid.NewString()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	cp := *n
	r.notifications[n.ID] = &cp
	r.order = append(r.order, n.ID)
	return nil
}

func (r *MemoryNotificationRepo) GetByID(ctx context.Context, id string) (*notification.Notification, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	n, exists := r.notifications[id]
	if !exists {
		return nil, notification.ErrNotificationNotFound
	}
	cp := *n
	return &cp, nil
}

func (r *MemoryNotificationRepo) List(ctx context.Context, filter notification.Filter) ([]*notification.Notification, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*notification.Notification
	// Reverse order (newest first)
	for i := len(r.order) - 1; i >= 0; i-- {
		id := r.order[i]
		n := r.notifications[id]

		if filter.TenantID != "" && n.TenantID != filter.TenantID {
			continue
		}
		if filter.UserID != "" && n.UserID != filter.UserID {
			continue
		}
		if filter.UnreadOnly && n.ReadAt != nil {
			continue
		}
		if filter.Severity != nil && n.Severity != *filter.Severity {
			continue
		}

		cp := *n
		matched = append(matched, &cp)
	}

	total := int64(len(matched))
	if filter.Offset >= len(matched) {
		return []*notification.Notification{}, total, nil
	}

	end := len(matched)
	if filter.Limit > 0 && filter.Offset+filter.Limit < end {
		end = filter.Offset + filter.Limit
	}

	return matched[filter.Offset:end], total, nil
}

func (r *MemoryNotificationRepo) MarkAsRead(ctx context.Context, id string, userID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	n, exists := r.notifications[id]
	if !exists {
		return notification.ErrNotificationNotFound
	}
	if n.UserID != userID {
		return notification.ErrNotificationNotFound
	}

	now := time.Now().UTC()
	n.ReadAt = &now
	return nil
}

func (r *MemoryNotificationRepo) MarkAllAsRead(ctx context.Context, userID string) (int64, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	var count int64
	for _, n := range r.notifications {
		if n.UserID == userID && n.ReadAt == nil {
			n.ReadAt = &now
			count++
		}
	}
	return count, nil
}

func (r *MemoryNotificationRepo) GetUnreadCount(ctx context.Context, userID string) (int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var count int64
	for _, n := range r.notifications {
		if n.UserID == userID && n.ReadAt == nil {
			count++
		}
	}
	return count, nil
}

// MemoryPreferenceRepo implements notification.PreferenceRepository in-memory.
type MemoryPreferenceRepo struct {
	mu          sync.RWMutex
	preferences map[string]map[string]*notification.NotificationPreference // userID -> eventType -> pref
}

func NewMemoryPreferenceRepo() *MemoryPreferenceRepo {
	return &MemoryPreferenceRepo{
		preferences: make(map[string]map[string]*notification.NotificationPreference),
	}
}

func (r *MemoryPreferenceRepo) GetPreferences(ctx context.Context, userID string) ([]*notification.NotificationPreference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*notification.NotificationPreference
	if userPrefs, exists := r.preferences[userID]; exists {
		for _, p := range userPrefs {
			cp := *p
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *MemoryPreferenceRepo) GetPreference(ctx context.Context, userID string, eventType string) (*notification.NotificationPreference, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if userPrefs, exists := r.preferences[userID]; exists {
		if p, ok := userPrefs[eventType]; ok {
			cp := *p
			return &cp, nil
		}
	}
	// Default preference (all channels enabled)
	return &notification.NotificationPreference{
		UserID:         userID,
		EventType:      eventType,
		InAppEnabled:   true,
		EmailEnabled:   true,
		WebhookEnabled: true,
	}, nil
}

func (r *MemoryPreferenceRepo) SetPreference(ctx context.Context, pref *notification.NotificationPreference) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.preferences[pref.UserID]; !exists {
		r.preferences[pref.UserID] = make(map[string]*notification.NotificationPreference)
	}

	cp := *pref
	r.preferences[pref.UserID][pref.EventType] = &cp
	return nil
}
