package postgres

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UsageRepository implements billing.UsageRepository using PostgreSQL.
type UsageRepository struct {
	pool *pgxpool.Pool
}

func NewUsageRepository(pool *pgxpool.Pool) *UsageRepository {
	return &UsageRepository{pool: pool}
}

func (r *UsageRepository) RecordUsage(ctx context.Context, u *billing.UsageRecord) error {
	metaJSON, _ := json.Marshal(u.Metadata)
	now := time.Now().UTC()

	query := `
		INSERT INTO usage_records (
			id, user_id, resource_type, resource_id, metric, quantity, unit,
			period_start, period_end, idempotency_key, metadata, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7,
			$8, $9, NULLIF($10, ''), $11, $12
		)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, created_at;
	`
	err := r.pool.QueryRow(ctx, query,
		u.ID, u.TenantID, u.ResourceType, u.ResourceID, string(u.Metric), u.Quantity, u.Unit,
		u.PeriodStart, u.PeriodEnd, u.IdempotencyKey, metaJSON, now,
	).Scan(&u.ID, &u.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			// Idempotent duplicate record: no-op
			return nil
		}
		return err
	}
	return nil
}

func (r *UsageRepository) GetAggregate(ctx context.Context, tenantID string, start, end time.Time) (*billing.UsageAggregate, error) {
	query := `
		SELECT metric, COALESCE(SUM(quantity), 0)
		FROM usage_records
		WHERE user_id = $1 AND period_end >= $2 AND period_start <= $3
		GROUP BY metric;
	`
	rows, err := r.pool.Query(ctx, query, tenantID, start, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	agg := &billing.UsageAggregate{
		TenantID:    tenantID,
		PeriodStart: start,
		PeriodEnd:   end,
		Metrics:     make(map[billing.MetricType]float64),
	}

	for rows.Next() {
		var metricStr string
		var sumQty float64
		if err := rows.Scan(&metricStr, &sumQty); err != nil {
			return nil, err
		}
		agg.Metrics[billing.MetricType(metricStr)] = sumQty
	}
	return agg, rows.Err()
}

func (r *UsageRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*billing.UsageRecord, error) {
	query := `
		SELECT id, user_id, resource_type, resource_id, metric, quantity, unit,
			period_start, period_end, idempotency_key, metadata, created_at
		FROM usage_records
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3;
	`
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*billing.UsageRecord
	for rows.Next() {
		u, err := r.scanRecord(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, u)
	}
	return result, rows.Err()
}

func (r *UsageRepository) scanRecord(row pgx.Row) (*billing.UsageRecord, error) {
	var u billing.UsageRecord
	var metricStr string
	var metaBytes []byte
	var idemKey *string

	err := row.Scan(
		&u.ID, &u.TenantID, &u.ResourceType, &u.ResourceID, &metricStr, &u.Quantity, &u.Unit,
		&u.PeriodStart, &u.PeriodEnd, &idemKey, &metaBytes, &u.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	u.Metric = billing.MetricType(metricStr)
	if idemKey != nil {
		u.IdempotencyKey = *idemKey
	}
	if len(metaBytes) > 0 {
		_ = json.Unmarshal(metaBytes, &u.Metadata)
	}
	return &u, nil
}
