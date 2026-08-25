package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/notification"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// NotificationRepository implements notification.NotificationRepository using PostgreSQL.
type NotificationRepository struct {
	pool *pgxpool.Pool
}

func NewNotificationRepository(pool *pgxpool.Pool) *NotificationRepository {
	return &NotificationRepository{pool: pool}
}

func (r *NotificationRepository) Create(ctx context.Context, n *notification.Notification) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO notifications (
			id, tenant_id, user_id, event_type, title, body, severity,
			resource_type, resource_id, read_at, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11
		) RETURNING id, created_at;
	`
	return r.pool.QueryRow(ctx, query,
		n.ID, n.TenantID, n.UserID, n.Type, n.Title, n.Body, string(n.Severity),
		n.ResourceType, n.ResourceID, n.ReadAt, now,
	).Scan(&n.ID, &n.CreatedAt)
}

func (r *NotificationRepository) GetByID(ctx context.Context, id string) (*notification.Notification, error) {
	query := `
		SELECT id, tenant_id, user_id, event_type, title, body, severity,
			resource_type, resource_id, read_at, created_at
		FROM notifications WHERE id = $1;
	`
	var n notification.Notification
	var sevStr string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&n.ID, &n.TenantID, &n.UserID, &n.Type, &n.Title, &n.Body, &sevStr,
		&n.ResourceType, &n.ResourceID, &n.ReadAt, &n.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, notification.ErrNotificationNotFound
		}
		return nil, err
	}
	n.Severity = notification.Severity(sevStr)
	return &n, nil
}

func (r *NotificationRepository) List(ctx context.Context, filter notification.Filter) ([]*notification.Notification, int64, error) {
	var sevStr string
	if filter.Severity != nil {
		sevStr = string(*filter.Severity)
	}

	baseQuery := `
		FROM notifications
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR user_id = $2)
		  AND ($3::boolean = false OR read_at IS NULL)
		  AND ($4 = '' OR severity = $4)
	`
	countQuery := `SELECT COUNT(*) ` + baseQuery
	var total int64
	err := r.pool.QueryRow(ctx, countQuery, filter.TenantID, filter.UserID, filter.UnreadOnly, sevStr).Scan(&total)
	if err != nil {
		return nil, 0, err
	}

	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := filter.Offset
	if offset < 0 {
		offset = 0
	}

	selectQuery := `
		SELECT id, tenant_id, user_id, event_type, title, body, severity,
			resource_type, resource_id, read_at, created_at
		` + baseQuery + `
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $6;
	`
	rows, err := r.pool.Query(ctx, selectQuery, filter.TenantID, filter.UserID, filter.UnreadOnly, sevStr, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*notification.Notification
	for rows.Next() {
		var n notification.Notification
		var sStr string
		if err := rows.Scan(
			&n.ID, &n.TenantID, &n.UserID, &n.Type, &n.Title, &n.Body, &sStr,
			&n.ResourceType, &n.ResourceID, &n.ReadAt, &n.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		n.Severity = notification.Severity(sStr)
		result = append(result, &n)
	}

	return result, total, rows.Err()
}

func (r *NotificationRepository) MarkAsRead(ctx context.Context, id string, userID string) error {
	now := time.Now().UTC()
	query := `
		UPDATE notifications SET read_at = $3
		WHERE id = $1 AND user_id = $2 AND read_at IS NULL;
	`
	cmd, err := r.pool.Exec(ctx, query, id, userID, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return notification.ErrNotificationNotFound
	}
	return nil
}

func (r *NotificationRepository) MarkAllAsRead(ctx context.Context, userID string) (int64, error) {
	now := time.Now().UTC()
	query := `
		UPDATE notifications SET read_at = $2
		WHERE user_id = $1 AND read_at IS NULL;
	`
	cmd, err := r.pool.Exec(ctx, query, userID, now)
	if err != nil {
		return 0, err
	}
	return cmd.RowsAffected(), nil
}

func (r *NotificationRepository) GetUnreadCount(ctx context.Context, userID string) (int64, error) {
	query := `SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND read_at IS NULL;`
	var count int64
	err := r.pool.QueryRow(ctx, query, userID).Scan(&count)
	return count, err
}

// PreferenceRepository implements notification.PreferenceRepository using PostgreSQL.
type PreferenceRepository struct {
	pool *pgxpool.Pool
}

func NewPreferenceRepository(pool *pgxpool.Pool) *PreferenceRepository {
	return &PreferenceRepository{pool: pool}
}

func (r *PreferenceRepository) GetPreferences(ctx context.Context, userID string) ([]*notification.NotificationPreference, error) {
	query := `
		SELECT user_id, event_type, in_app_enabled, email_enabled, webhook_enabled
		FROM notification_preferences WHERE user_id = $1;
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*notification.NotificationPreference
	for rows.Next() {
		var p notification.NotificationPreference
		if err := rows.Scan(&p.UserID, &p.EventType, &p.InAppEnabled, &p.EmailEnabled, &p.WebhookEnabled); err != nil {
			return nil, err
		}
		result = append(result, &p)
	}
	return result, rows.Err()
}

func (r *PreferenceRepository) GetPreference(ctx context.Context, userID string, eventType string) (*notification.NotificationPreference, error) {
	query := `
		SELECT user_id, event_type, in_app_enabled, email_enabled, webhook_enabled
		FROM notification_preferences WHERE user_id = $1 AND event_type = $2;
	`
	var p notification.NotificationPreference
	err := r.pool.QueryRow(ctx, query, userID, eventType).Scan(
		&p.UserID, &p.EventType, &p.InAppEnabled, &p.EmailEnabled, &p.WebhookEnabled,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return &notification.NotificationPreference{
				UserID:         userID,
				EventType:      eventType,
				InAppEnabled:   true,
				EmailEnabled:   true,
				WebhookEnabled: true,
			}, nil
		}
		return nil, err
	}
	return &p, nil
}

func (r *PreferenceRepository) SetPreference(ctx context.Context, pref *notification.NotificationPreference) error {
	query := `
		INSERT INTO notification_preferences (user_id, event_type, in_app_enabled, email_enabled, webhook_enabled)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id, event_type) DO UPDATE SET
			in_app_enabled = EXCLUDED.in_app_enabled,
			email_enabled = EXCLUDED.email_enabled,
			webhook_enabled = EXCLUDED.webhook_enabled;
	`
	_, err := r.pool.Exec(ctx, query, pref.UserID, pref.EventType, pref.InAppEnabled, pref.EmailEnabled, pref.WebhookEnabled)
	return err
}
