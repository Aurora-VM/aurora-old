package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// EventRepository implements events.Repository using PostgreSQL.
type EventRepository struct {
	pool *pgxpool.Pool
}

func NewEventRepository(pool *pgxpool.Pool) *EventRepository {
	return &EventRepository{pool: pool}
}

func (r *EventRepository) Store(ctx context.Context, e *events.Event) error {
	payloadJSON, _ := json.Marshal(e.Payload)
	metaJSON, _ := json.Marshal(e.Metadata)
	if e.Timestamp.IsZero() {
		e.Timestamp = time.Now().UTC()
	}
	if e.Version == "" {
		e.Version = "1.0"
	}

	query := `
		INSERT INTO events (
			id, tenant_id, event_type, resource_type, resource_id, actor_id,
			payload, metadata, version, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6,
			$7, $8, $9, $10
		) RETURNING id, created_at;
	`
	return r.pool.QueryRow(ctx, query,
		e.ID, e.TenantID, string(e.Type), e.ResourceType, e.ResourceID, e.ActorID,
		payloadJSON, metaJSON, e.Version, e.Timestamp,
	).Scan(&e.ID, &e.Timestamp)
}

func (r *EventRepository) GetByID(ctx context.Context, id string) (*events.Event, error) {
	query := `
		SELECT id, tenant_id, event_type, resource_type, resource_id, actor_id,
			payload, metadata, version, created_at
		FROM events WHERE id = $1;
	`
	var e events.Event
	var eventTypeStr string
	var payloadBytes, metaBytes []byte

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&e.ID, &e.TenantID, &eventTypeStr, &e.ResourceType, &e.ResourceID, &e.ActorID,
		&payloadBytes, &metaBytes, &e.Version, &e.Timestamp,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, events.ErrEventNotFound
		}
		return nil, err
	}

	e.Type = events.EventType(eventTypeStr)
	_ = json.Unmarshal(payloadBytes, &e.Payload)
	_ = json.Unmarshal(metaBytes, &e.Metadata)
	return &e, nil
}

func (r *EventRepository) List(ctx context.Context, filter events.EventFilter) ([]*events.Event, int64, error) {
	baseQuery := `
		FROM events
		WHERE ($1 = '' OR tenant_id = $1)
		  AND ($2 = '' OR event_type = $2)
		  AND ($3 = '' OR resource_type = $3)
		  AND ($4 = '' OR resource_id = $4)
		  AND ($5 = '' OR actor_id = $5)
		  AND ($6::timestamptz IS NULL OR created_at >= $6)
		  AND ($7::timestamptz IS NULL OR created_at <= $7)
	`
	countQuery := `SELECT COUNT(*) ` + baseQuery
	var total int64
	err := r.pool.QueryRow(ctx, countQuery,
		filter.TenantID, string(filter.Type), filter.ResourceType, filter.ResourceID,
		filter.ActorID, filter.StartTime, filter.EndTime,
	).Scan(&total)
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
		SELECT id, tenant_id, event_type, resource_type, resource_id, actor_id,
			payload, metadata, version, created_at
		` + baseQuery + `
		ORDER BY created_at DESC
		LIMIT $8 OFFSET $9;
	`
	rows, err := r.pool.Query(ctx, selectQuery,
		filter.TenantID, string(filter.Type), filter.ResourceType, filter.ResourceID,
		filter.ActorID, filter.StartTime, filter.EndTime, limit, offset,
	)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var result []*events.Event
	for rows.Next() {
		var e events.Event
		var eventTypeStr string
		var payloadBytes, metaBytes []byte

		if err := rows.Scan(
			&e.ID, &e.TenantID, &eventTypeStr, &e.ResourceType, &e.ResourceID, &e.ActorID,
			&payloadBytes, &metaBytes, &e.Version, &e.Timestamp,
		); err != nil {
			return nil, 0, err
		}
		e.Type = events.EventType(eventTypeStr)
		_ = json.Unmarshal(payloadBytes, &e.Payload)
		_ = json.Unmarshal(metaBytes, &e.Metadata)
		result = append(result, &e)
	}

	return result, total, rows.Err()
}
