package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// WebhookRepository implements webhook.WebhookRepository using PostgreSQL.
type WebhookRepository struct {
	pool *pgxpool.Pool
}

func NewWebhookRepository(pool *pgxpool.Pool) *WebhookRepository {
	return &WebhookRepository{pool: pool}
}

func (r *WebhookRepository) Create(ctx context.Context, ep *webhook.WebhookEndpoint) error {
	typesJSON, _ := json.Marshal(ep.EventTypes)
	now := time.Now().UTC()

	query := `
		INSERT INTO webhook_endpoints (
			id, tenant_id, name, url, description, secret, active,
			event_types, failure_count, last_status, last_delivery_at,
			created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13
		) RETURNING id, created_at, updated_at;
	`
	return r.pool.QueryRow(ctx, query,
		ep.ID, ep.TenantID, ep.Name, ep.URL, ep.Description, ep.Secret, ep.Active,
		typesJSON, ep.FailureCount, ep.LastStatus, ep.LastDeliveryAt, now, now,
	).Scan(&ep.ID, &ep.CreatedAt, &ep.UpdatedAt)
}

func (r *WebhookRepository) GetByID(ctx context.Context, id string) (*webhook.WebhookEndpoint, error) {
	query := `
		SELECT id, tenant_id, name, url, description, secret, active,
			event_types, failure_count, last_status, last_delivery_at,
			created_at, updated_at
		FROM webhook_endpoints WHERE id = $1;
	`
	var ep webhook.WebhookEndpoint
	var typesBytes []byte

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&ep.ID, &ep.TenantID, &ep.Name, &ep.URL, &ep.Description, &ep.Secret, &ep.Active,
		&typesBytes, &ep.FailureCount, &ep.LastStatus, &ep.LastDeliveryAt,
		&ep.CreatedAt, &ep.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, webhook.ErrWebhookNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal(typesBytes, &ep.EventTypes)
	return &ep, nil
}

func (r *WebhookRepository) List(ctx context.Context, filter webhook.WebhookFilter) ([]*webhook.WebhookEndpoint, int64, error) {
	baseQuery := `
		FROM webhook_endpoints
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2::boolean IS NULL OR active = $2)
	`
	countQuery := `SELECT COUNT(*) ` + baseQuery
	var total int64
	err := r.pool.QueryRow(ctx, countQuery, filter.TenantID, filter.Active).Scan(&total)
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
		SELECT id, tenant_id, name, url, description, secret, active,
			event_types, failure_count, last_status, last_delivery_at,
			created_at, updated_at
		` + baseQuery + `
		ORDER BY created_at DESC
		LIMIT $3 OFFSET $4;
	`
	rows, err := r.pool.Query(ctx, selectQuery, filter.TenantID, filter.Active, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*webhook.WebhookEndpoint
	for rows.Next() {
		var ep webhook.WebhookEndpoint
		var typesBytes []byte
		if err := rows.Scan(
			&ep.ID, &ep.TenantID, &ep.Name, &ep.URL, &ep.Description, &ep.Secret, &ep.Active,
			&typesBytes, &ep.FailureCount, &ep.LastStatus, &ep.LastDeliveryAt,
			&ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(typesBytes, &ep.EventTypes)
		result = append(result, &ep)
	}

	return result, total, rows.Err()
}

func (r *WebhookRepository) ListSubscribed(ctx context.Context, eventType string) ([]*webhook.WebhookEndpoint, error) {
	query := `
		SELECT id, tenant_id, name, url, description, secret, active,
			event_types, failure_count, last_status, last_delivery_at,
			created_at, updated_at
		FROM webhook_endpoints
		WHERE active = true;
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*webhook.WebhookEndpoint
	for rows.Next() {
		var ep webhook.WebhookEndpoint
		var typesBytes []byte
		if err := rows.Scan(
			&ep.ID, &ep.TenantID, &ep.Name, &ep.URL, &ep.Description, &ep.Secret, &ep.Active,
			&typesBytes, &ep.FailureCount, &ep.LastStatus, &ep.LastDeliveryAt,
			&ep.CreatedAt, &ep.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(typesBytes, &ep.EventTypes)
		if ep.SubscribesTo(eventType) {
			result = append(result, &ep)
		}
	}
	return result, rows.Err()
}

func (r *WebhookRepository) Update(ctx context.Context, ep *webhook.WebhookEndpoint) error {
	typesJSON, _ := json.Marshal(ep.EventTypes)
	now := time.Now().UTC()

	query := `
		UPDATE webhook_endpoints SET
			name = $2, url = $3, description = $4, secret = $5,
			active = $6, event_types = $7, updated_at = $8
		WHERE id = $1;
	`
	cmd, err := r.pool.Exec(ctx, query,
		ep.ID, ep.Name, ep.URL, ep.Description, ep.Secret,
		ep.Active, typesJSON, now,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return webhook.ErrWebhookNotFound
	}
	ep.UpdatedAt = now
	return nil
}

func (r *WebhookRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM webhook_endpoints WHERE id = $1;`
	cmd, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return webhook.ErrWebhookNotFound
	}
	return nil
}

func (r *WebhookRepository) UpdateDeliveryStats(ctx context.Context, id string, lastStatus string, failureIncrement bool) error {
	now := time.Now().UTC()
	query := `
		UPDATE webhook_endpoints SET
			last_status = $2,
			last_delivery_at = $3,
			failure_count = CASE WHEN $4::boolean THEN failure_count + 1 ELSE 0 END,
			updated_at = $3
		WHERE id = $1;
	`
	_, err := r.pool.Exec(ctx, query, id, lastStatus, now, failureIncrement)
	return err
}

// DeliveryRepository implements webhook.DeliveryRepository using PostgreSQL.
type DeliveryRepository struct {
	pool *pgxpool.Pool
}

func NewDeliveryRepository(pool *pgxpool.Pool) *DeliveryRepository {
	return &DeliveryRepository{pool: pool}
}

func (r *DeliveryRepository) Create(ctx context.Context, d *webhook.WebhookDelivery) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO webhook_deliveries (
			id, event_id, webhook_id, tenant_id, event_type, attempt,
			status, http_status, response_time_ms, error_message,
			next_retry_at, delivered_at, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6,
			$7, $8, $9, $10, $11, $12, $13
		) RETURNING id, created_at;
	`
	return r.pool.QueryRow(ctx, query,
		d.ID, d.EventID, d.WebhookID, d.TenantID, d.EventType, d.Attempt,
		string(d.Status), d.HTTPStatus, d.ResponseTimeMs, d.Error,
		d.NextRetryAt, d.DeliveredAt, now,
	).Scan(&d.ID, &d.CreatedAt)
}

func (r *DeliveryRepository) GetByID(ctx context.Context, id string) (*webhook.WebhookDelivery, error) {
	query := `
		SELECT id, event_id, webhook_id, tenant_id, event_type, attempt,
			status, http_status, response_time_ms, error_message,
			next_retry_at, delivered_at, created_at
		FROM webhook_deliveries WHERE id = $1;
	`
	var d webhook.WebhookDelivery
	var statusStr string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&d.ID, &d.EventID, &d.WebhookID, &d.TenantID, &d.EventType, &d.Attempt,
		&statusStr, &d.HTTPStatus, &d.ResponseTimeMs, &d.Error,
		&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, webhook.ErrDeliveryNotFound
		}
		return nil, err
	}
	d.Status = webhook.DeliveryStatus(statusStr)
	return &d, nil
}

func (r *DeliveryRepository) List(ctx context.Context, filter webhook.DeliveryFilter) ([]*webhook.WebhookDelivery, int64, error) {
	var statusStr string
	if filter.Status != nil {
		statusStr = string(*filter.Status)
	}

	baseQuery := `
		FROM webhook_deliveries
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR webhook_id = $2)
		  AND ($3 = '' OR event_id = $3)
		  AND ($4 = '' OR status = $4)
	`
	countQuery := `SELECT COUNT(*) ` + baseQuery
	var total int64
	err := r.pool.QueryRow(ctx, countQuery, filter.TenantID, filter.WebhookID, filter.EventID, statusStr).Scan(&total)
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
		SELECT id, event_id, webhook_id, tenant_id, event_type, attempt,
			status, http_status, response_time_ms, error_message,
			next_retry_at, delivered_at, created_at
		` + baseQuery + `
		ORDER BY created_at DESC
		LIMIT $5 OFFSET $6;
	`
	rows, err := r.pool.Query(ctx, selectQuery, filter.TenantID, filter.WebhookID, filter.EventID, statusStr, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*webhook.WebhookDelivery
	for rows.Next() {
		var d webhook.WebhookDelivery
		var sStr string
		if err := rows.Scan(
			&d.ID, &d.EventID, &d.WebhookID, &d.TenantID, &d.EventType, &d.Attempt,
			&sStr, &d.HTTPStatus, &d.ResponseTimeMs, &d.Error,
			&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		d.Status = webhook.DeliveryStatus(sStr)
		result = append(result, &d)
	}

	return result, total, rows.Err()
}

func (r *DeliveryRepository) ListPendingRetries(ctx context.Context, before time.Time, limit int) ([]*webhook.WebhookDelivery, error) {
	if limit <= 0 {
		limit = 50
	}
	query := `
		SELECT id, event_id, webhook_id, tenant_id, event_type, attempt,
			status, http_status, response_time_ms, error_message,
			next_retry_at, delivered_at, created_at
		FROM webhook_deliveries
		WHERE status = 'pending' AND next_retry_at <= $1
		ORDER BY next_retry_at ASC
		LIMIT $2;
	`
	rows, err := r.pool.Query(ctx, query, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*webhook.WebhookDelivery
	for rows.Next() {
		var d webhook.WebhookDelivery
		var sStr string
		if err := rows.Scan(
			&d.ID, &d.EventID, &d.WebhookID, &d.TenantID, &d.EventType, &d.Attempt,
			&sStr, &d.HTTPStatus, &d.ResponseTimeMs, &d.Error,
			&d.NextRetryAt, &d.DeliveredAt, &d.CreatedAt,
		); err != nil {
			return nil, err
		}
		d.Status = webhook.DeliveryStatus(sStr)
		result = append(result, &d)
	}

	return result, rows.Err()
}

func (r *DeliveryRepository) Update(ctx context.Context, d *webhook.WebhookDelivery) error {
	query := `
		UPDATE webhook_deliveries SET
			attempt = $2, status = $3, http_status = $4,
			response_time_ms = $5, error_message = $6,
			next_retry_at = $7, delivered_at = $8
		WHERE id = $1;
	`
	cmd, err := r.pool.Exec(ctx, query,
		d.ID, d.Attempt, string(d.Status), d.HTTPStatus,
		d.ResponseTimeMs, d.Error, d.NextRetryAt, d.DeliveredAt,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return webhook.ErrDeliveryNotFound
	}
	return nil
}
