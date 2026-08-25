package postgres

import (
	"context"
	"errors"
	"math"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/monitoring"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// MetricsRepository implements monitoring.MetricsRepository with PostgreSQL.
type MetricsRepository struct {
	pool *pgxpool.Pool
}

func NewMetricsRepository(pool *pgxpool.Pool) *MetricsRepository {
	return &MetricsRepository{pool: pool}
}

func (r *MetricsRepository) InsertSamples(ctx context.Context, samples []*monitoring.MetricSample) error {
	if len(samples) == 0 {
		return nil
	}

	query := `
		INSERT INTO metric_samples (
			resource_type, resource_id, metric_name, value, timestamp
		) VALUES ($1, $2::uuid, $3, $4, $5)
		ON CONFLICT (resource_type, resource_id, metric_name, timestamp) DO NOTHING
	`

	batch := &pgx.Batch{}
	for _, s := range samples {
		if s == nil {
			continue
		}
		ts := s.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		batch.Queue(query, string(s.ResourceType), s.ResourceID, s.MetricName, s.Value, ts)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for i := 0; i < len(samples); i++ {
		_, err := br.Exec()
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *MetricsRepository) QueryRange(
	ctx context.Context,
	resType monitoring.ResourceType,
	resID, metricName string,
	from, to time.Time,
	stepSeconds int,
) (*monitoring.TimeSeries, error) {
	if stepSeconds <= 0 {
		stepSeconds = 10
	}

	query := `
		SELECT 
			to_timestamp(floor(extract(epoch from timestamp) / $5) * $5) AS bucket,
			AVG(value) AS avg_value
		FROM metric_samples
		WHERE resource_type = $1 AND resource_id = $2::uuid AND metric_name = $3
		  AND timestamp >= $4 AND timestamp <= $6
		GROUP BY bucket
		ORDER BY bucket ASC
	`

	rows, err := r.pool.Query(ctx, query, string(resType), resID, metricName, from, stepSeconds, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var points []monitoring.DataPoint
	for rows.Next() {
		var bucketTime time.Time
		var avgVal float64
		if err := rows.Scan(&bucketTime, &avgVal); err != nil {
			return nil, err
		}
		points = append(points, monitoring.DataPoint{
			Timestamp: bucketTime.UTC(),
			Value:     math.Round(avgVal*100) / 100,
		})
	}

	return &monitoring.TimeSeries{
		MetricName: metricName,
		Unit:       getPostgresMetricUnit(metricName),
		DataPoints: points,
	}, rows.Err()
}

func getPostgresMetricUnit(metricName string) string {
	switch metricName {
	case "cpu_percent":
		return "%"
	case "memory_used_bytes", "memory_total_bytes", "disk_used_bytes", "disk_total_bytes", "net_rx_bytes", "net_tx_bytes":
		return "bytes"
	case "load_1m", "load_5m", "load_15m":
		return "load"
	default:
		return "value"
	}
}

// AlertThresholdRepository implements monitoring.AlertThresholdRepository with PostgreSQL.
type AlertThresholdRepository struct {
	pool *pgxpool.Pool
}

func NewAlertThresholdRepository(pool *pgxpool.Pool) *AlertThresholdRepository {
	return &AlertThresholdRepository{pool: pool}
}

func (r *AlertThresholdRepository) Create(ctx context.Context, t *monitoring.AlertThreshold) error {
	query := `
		INSERT INTO alert_thresholds (
			id, user_id, resource_type, resource_id, metric_name, operator, threshold_value, duration_seconds, severity, enabled, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), NULLIF($2, '')::uuid, $3, $4::uuid, $5, $6, $7, $8, $9, $10, $11, $12
		) RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	return r.pool.QueryRow(ctx, query,
		t.ID, t.UserID, string(t.ResourceType), t.ResourceID, t.MetricName, string(t.Operator), t.ThresholdValue, t.DurationSeconds, string(t.Severity), t.Enabled, now, now,
	).Scan(&t.ID, &t.CreatedAt, &t.UpdatedAt)
}

func (r *AlertThresholdRepository) GetByID(ctx context.Context, id string) (*monitoring.AlertThreshold, error) {
	query := `
		SELECT id, COALESCE(user_id::text, ''), resource_type, resource_id::text, metric_name, operator, threshold_value, duration_seconds, severity, enabled, created_at, updated_at
		FROM alert_thresholds WHERE id = $1
	`
	return r.scanThreshold(r.pool.QueryRow(ctx, query, id))
}

func (r *AlertThresholdRepository) ListByResource(ctx context.Context, resType monitoring.ResourceType, resID string) ([]*monitoring.AlertThreshold, error) {
	query := `
		SELECT id, COALESCE(user_id::text, ''), resource_type, resource_id::text, metric_name, operator, threshold_value, duration_seconds, severity, enabled, created_at, updated_at
		FROM alert_thresholds WHERE resource_type = $1 AND ($2 = '' OR resource_id = $2::uuid)
		ORDER BY created_at DESC
	`
	return r.queryThresholds(ctx, query, string(resType), resID)
}

func (r *AlertThresholdRepository) ListAll(ctx context.Context) ([]*monitoring.AlertThreshold, error) {
	query := `
		SELECT id, COALESCE(user_id::text, ''), resource_type, resource_id::text, metric_name, operator, threshold_value, duration_seconds, severity, enabled, created_at, updated_at
		FROM alert_thresholds
		ORDER BY created_at DESC
	`
	return r.queryThresholds(ctx, query)
}

func (r *AlertThresholdRepository) queryThresholds(ctx context.Context, query string, args ...interface{}) ([]*monitoring.AlertThreshold, error) {
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*monitoring.AlertThreshold
	for rows.Next() {
		t, err := r.scanThreshold(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}

func (r *AlertThresholdRepository) Update(ctx context.Context, t *monitoring.AlertThreshold) error {
	query := `
		UPDATE alert_thresholds
		SET metric_name = $2, operator = $3, threshold_value = $4, duration_seconds = $5, severity = $6, enabled = $7, updated_at = $8
		WHERE id = $1
	`
	res, err := r.pool.Exec(ctx, query,
		t.ID, t.MetricName, string(t.Operator), t.ThresholdValue, t.DurationSeconds, string(t.Severity), t.Enabled, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return monitoring.ErrThresholdNotFound
	}
	return nil
}

func (r *AlertThresholdRepository) Delete(ctx context.Context, id string) error {
	res, err := r.pool.Exec(ctx, `DELETE FROM alert_thresholds WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return monitoring.ErrThresholdNotFound
	}
	return nil
}

func (r *AlertThresholdRepository) scanThreshold(row pgx.Row) (*monitoring.AlertThreshold, error) {
	var t monitoring.AlertThreshold
	var resTypeStr, opStr, sevStr string

	err := row.Scan(
		&t.ID, &t.UserID, &resTypeStr, &t.ResourceID, &t.MetricName, &opStr, &t.ThresholdValue, &t.DurationSeconds, &sevStr, &t.Enabled, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, monitoring.ErrThresholdNotFound
		}
		return nil, err
	}

	t.ResourceType = monitoring.ResourceType(resTypeStr)
	t.Operator = monitoring.ComparisonOperator(opStr)
	t.Severity = monitoring.AlertSeverity(sevStr)
	return &t, nil
}

// AlertEventRepository implements monitoring.AlertEventRepository with PostgreSQL.
type AlertEventRepository struct {
	pool *pgxpool.Pool
}

func NewAlertEventRepository(pool *pgxpool.Pool) *AlertEventRepository {
	return &AlertEventRepository{pool: pool}
}

func (r *AlertEventRepository) Create(ctx context.Context, e *monitoring.AlertEvent) error {
	query := `
		INSERT INTO alert_events (
			id, threshold_id, resource_type, resource_id, triggered_value, severity, message, state, triggered_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), NULLIF($2, '')::uuid, $3, $4::uuid, $5, $6, $7, $8, $9
		) RETURNING id, triggered_at
	`
	now := e.TriggeredAt
	if now.IsZero() {
		now = time.Now().UTC()
	}

	return r.pool.QueryRow(ctx, query,
		e.ID, e.ThresholdID, string(e.ResourceType), e.ResourceID, e.TriggeredValue, string(e.Severity), e.Message, string(e.State), now,
	).Scan(&e.ID, &e.TriggeredAt)
}

func (r *AlertEventRepository) GetByID(ctx context.Context, id string) (*monitoring.AlertEvent, error) {
	query := `
		SELECT id, COALESCE(threshold_id::text, ''), resource_type, resource_id::text, triggered_value, severity, message, state, triggered_at, resolved_at
		FROM alert_events WHERE id = $1
	`
	return r.scanEvent(r.pool.QueryRow(ctx, query, id))
}

func (r *AlertEventRepository) List(ctx context.Context, resType monitoring.ResourceType, resID string, state monitoring.AlertState) ([]*monitoring.AlertEvent, error) {
	query := `
		SELECT id, COALESCE(threshold_id::text, ''), resource_type, resource_id::text, triggered_value, severity, message, state, triggered_at, resolved_at
		FROM alert_events
		WHERE ($1 = '' OR resource_type = $1)
		  AND ($2 = '' OR resource_id = $2::uuid)
		  AND ($3 = '' OR state = $3)
		ORDER BY triggered_at DESC
	`
	rows, err := r.pool.Query(ctx, query, string(resType), resID, string(state))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*monitoring.AlertEvent
	for rows.Next() {
		e, err := r.scanEvent(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (r *AlertEventRepository) Update(ctx context.Context, e *monitoring.AlertEvent) error {
	query := `
		UPDATE alert_events
		SET state = $2, resolved_at = $3
		WHERE id = $1
	`
	res, err := r.pool.Exec(ctx, query, e.ID, string(e.State), e.ResolvedAt)
	if err != nil {
		return err
	}
	if res.RowsAffected() == 0 {
		return monitoring.ErrAlertEventNotFound
	}
	return nil
}

func (r *AlertEventRepository) scanEvent(row pgx.Row) (*monitoring.AlertEvent, error) {
	var e monitoring.AlertEvent
	var resTypeStr, sevStr, stateStr string

	err := row.Scan(
		&e.ID, &e.ThresholdID, &resTypeStr, &e.ResourceID, &e.TriggeredValue, &sevStr, &e.Message, &stateStr, &e.TriggeredAt, &e.ResolvedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, monitoring.ErrAlertEventNotFound
		}
		return nil, err
	}

	e.ResourceType = monitoring.ResourceType(resTypeStr)
	e.Severity = monitoring.AlertSeverity(sevStr)
	e.State = monitoring.AlertState(stateStr)
	return &e, nil
}
