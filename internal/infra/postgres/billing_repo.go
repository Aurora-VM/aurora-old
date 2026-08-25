package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// --- PlanRepository Implementation ---

type PlanRepository struct {
	pool *pgxpool.Pool
}

func NewPlanRepository(pool *pgxpool.Pool) *PlanRepository {
	return &PlanRepository{pool: pool}
}

func (r *PlanRepository) Create(ctx context.Context, p *billing.Plan) error {
	featJSON, _ := json.Marshal(p.Features)
	now := time.Now().UTC()

	query := `
		INSERT INTO billing_plans (
			id, name, slug, description, currency, monthly_price_minor, yearly_price_minor,
			included_vcpu, included_memory_mb, included_storage_mb, included_ipv4,
			included_snapshots, included_backups, included_bandwidth_gb,
			max_instances, max_vcpu, max_memory_mb, max_storage_mb,
			features, active, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
		) RETURNING id, created_at, updated_at;
	`
	err := r.pool.QueryRow(ctx, query,
		p.ID, p.Name, p.Slug, p.Description, p.Currency, p.MonthlyPriceMinor, p.YearlyPriceMinor,
		p.IncludedVCPU, p.IncludedMemoryMB, p.IncludedStorageMB, p.IncludedIPv4,
		p.IncludedSnapshots, p.IncludedBackups, p.IncludedBandwidthGB,
		p.MaximumInstances, p.MaximumVCPU, p.MaximumMemoryMB, p.MaximumStorageMB,
		featJSON, p.Active, now, now,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "idx_billing_plans_slug") || strings.Contains(err.Error(), "billing_plans_slug_key") {
			return billing.ErrPlanSlugExists
		}
		return err
	}
	return nil
}

func (r *PlanRepository) GetByID(ctx context.Context, id string) (*billing.Plan, error) {
	query := `
		SELECT id, name, slug, description, currency, monthly_price_minor, yearly_price_minor,
			included_vcpu, included_memory_mb, included_storage_mb, included_ipv4,
			included_snapshots, included_backups, included_bandwidth_gb,
			max_instances, max_vcpu, max_memory_mb, max_storage_mb,
			features, active, created_at, updated_at
		FROM billing_plans WHERE id = $1;
	`
	return r.scanPlan(r.pool.QueryRow(ctx, query, id))
}

func (r *PlanRepository) GetBySlug(ctx context.Context, slug string) (*billing.Plan, error) {
	query := `
		SELECT id, name, slug, description, currency, monthly_price_minor, yearly_price_minor,
			included_vcpu, included_memory_mb, included_storage_mb, included_ipv4,
			included_snapshots, included_backups, included_bandwidth_gb,
			max_instances, max_vcpu, max_memory_mb, max_storage_mb,
			features, active, created_at, updated_at
		FROM billing_plans WHERE slug = $1;
	`
	return r.scanPlan(r.pool.QueryRow(ctx, query, slug))
}

func (r *PlanRepository) List(ctx context.Context, activeOnly bool) ([]*billing.Plan, error) {
	query := `
		SELECT id, name, slug, description, currency, monthly_price_minor, yearly_price_minor,
			included_vcpu, included_memory_mb, included_storage_mb, included_ipv4,
			included_snapshots, included_backups, included_bandwidth_gb,
			max_instances, max_vcpu, max_memory_mb, max_storage_mb,
			features, active, created_at, updated_at
		FROM billing_plans
		WHERE ($1::boolean = false OR active = true)
		ORDER BY monthly_price_minor ASC;
	`
	rows, err := r.pool.Query(ctx, query, activeOnly)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*billing.Plan
	for rows.Next() {
		p, err := r.scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, p)
	}
	return plans, rows.Err()
}

func (r *PlanRepository) Update(ctx context.Context, p *billing.Plan) error {
	featJSON, _ := json.Marshal(p.Features)
	now := time.Now().UTC()

	query := `
		UPDATE billing_plans SET
			name = $2, slug = $3, description = $4, currency = $5,
			monthly_price_minor = $6, yearly_price_minor = $7,
			included_vcpu = $8, included_memory_mb = $9, included_storage_mb = $10, included_ipv4 = $11,
			included_snapshots = $12, included_backups = $13, included_bandwidth_gb = $14,
			max_instances = $15, max_vcpu = $16, max_memory_mb = $17, max_storage_mb = $18,
			features = $19, active = $20, updated_at = $21
		WHERE id = $1;
	`
	cmd, err := r.pool.Exec(ctx, query,
		p.ID, p.Name, p.Slug, p.Description, p.Currency,
		p.MonthlyPriceMinor, p.YearlyPriceMinor,
		p.IncludedVCPU, p.IncludedMemoryMB, p.IncludedStorageMB, p.IncludedIPv4,
		p.IncludedSnapshots, p.IncludedBackups, p.IncludedBandwidthGB,
		p.MaximumInstances, p.MaximumVCPU, p.MaximumMemoryMB, p.MaximumStorageMB,
		featJSON, p.Active, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "idx_billing_plans_slug") || strings.Contains(err.Error(), "billing_plans_slug_key") {
			return billing.ErrPlanSlugExists
		}
		return err
	}
	if cmd.RowsAffected() == 0 {
		return billing.ErrPlanNotFound
	}
	p.UpdatedAt = now
	return nil
}

func (r *PlanRepository) Delete(ctx context.Context, id string) error {
	query := `DELETE FROM billing_plans WHERE id = $1;`
	cmd, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return billing.ErrPlanNotFound
	}
	return nil
}

func (r *PlanRepository) scanPlan(row pgx.Row) (*billing.Plan, error) {
	var p billing.Plan
	var featBytes []byte

	err := row.Scan(
		&p.ID, &p.Name, &p.Slug, &p.Description, &p.Currency,
		&p.MonthlyPriceMinor, &p.YearlyPriceMinor,
		&p.IncludedVCPU, &p.IncludedMemoryMB, &p.IncludedStorageMB, &p.IncludedIPv4,
		&p.IncludedSnapshots, &p.IncludedBackups, &p.IncludedBandwidthGB,
		&p.MaximumInstances, &p.MaximumVCPU, &p.MaximumMemoryMB, &p.MaximumStorageMB,
		&featBytes, &p.Active, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrPlanNotFound
		}
		return nil, err
	}
	if len(featBytes) > 0 {
		_ = json.Unmarshal(featBytes, &p.Features)
	}
	if p.Features == nil {
		p.Features = make(map[string]bool)
	}
	return &p, nil
}

// --- SubscriptionRepository Implementation ---

type SubscriptionRepository struct {
	pool *pgxpool.Pool
}

func NewSubscriptionRepository(pool *pgxpool.Pool) *SubscriptionRepository {
	return &SubscriptionRepository{pool: pool}
}

func (r *SubscriptionRepository) Create(ctx context.Context, s *billing.Subscription) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO subscriptions (
			id, user_id, plan_id, status, billing_cycle, current_period_start, current_period_end, cancel_at_period_end, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING id, created_at, updated_at;
	`
	return r.pool.QueryRow(ctx, query,
		s.ID, s.TenantID, s.PlanID, string(s.Status), string(s.BillingCycle),
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.CancelAtPeriodEnd, now, now,
	).Scan(&s.ID, &s.CreatedAt, &s.UpdatedAt)
}

func (r *SubscriptionRepository) GetByID(ctx context.Context, id string) (*billing.Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, billing_cycle, current_period_start, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM subscriptions WHERE id = $1;
	`
	return r.scanSubscription(r.pool.QueryRow(ctx, query, id))
}

func (r *SubscriptionRepository) GetByTenantID(ctx context.Context, tenantID string) (*billing.Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, billing_cycle, current_period_start, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM subscriptions WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1;
	`
	return r.scanSubscription(r.pool.QueryRow(ctx, query, tenantID))
}

func (r *SubscriptionRepository) List(ctx context.Context) ([]*billing.Subscription, error) {
	query := `
		SELECT id, user_id, plan_id, status, billing_cycle, current_period_start, current_period_end, cancel_at_period_end, created_at, updated_at
		FROM subscriptions ORDER BY created_at DESC;
	`
	rows, err := r.pool.Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*billing.Subscription
	for rows.Next() {
		s, err := r.scanSubscription(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	return result, rows.Err()
}

func (r *SubscriptionRepository) Update(ctx context.Context, s *billing.Subscription) error {
	now := time.Now().UTC()
	query := `
		UPDATE subscriptions SET
			plan_id = $2, status = $3, billing_cycle = $4,
			current_period_start = $5, current_period_end = $6, cancel_at_period_end = $7,
			updated_at = $8
		WHERE id = $1;
	`
	cmd, err := r.pool.Exec(ctx, query,
		s.ID, s.PlanID, string(s.Status), string(s.BillingCycle),
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.CancelAtPeriodEnd, now,
	)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return billing.ErrSubscriptionNotFound
	}
	s.UpdatedAt = now
	return nil
}

func (r *SubscriptionRepository) Cancel(ctx context.Context, id string) error {
	now := time.Now().UTC()
	query := `UPDATE subscriptions SET status = 'canceled', updated_at = $2 WHERE id = $1;`
	cmd, err := r.pool.Exec(ctx, query, id, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return billing.ErrSubscriptionNotFound
	}
	return nil
}

func (r *SubscriptionRepository) scanSubscription(row pgx.Row) (*billing.Subscription, error) {
	var s billing.Subscription
	var statusStr, cycleStr string

	err := row.Scan(
		&s.ID, &s.TenantID, &s.PlanID, &statusStr, &cycleStr,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd, &s.CancelAtPeriodEnd,
		&s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrSubscriptionNotFound
		}
		return nil, err
	}
	s.Status = billing.SubscriptionStatus(statusStr)
	s.BillingCycle = billing.BillingCycle(cycleStr)
	return &s, nil
}

// --- QuotaRepository Implementation ---

type QuotaRepository struct {
	pool *pgxpool.Pool
}

func NewQuotaRepository(pool *pgxpool.Pool) *QuotaRepository {
	return &QuotaRepository{pool: pool}
}

func (r *QuotaRepository) GetQuota(ctx context.Context, tenantID string, metric billing.MetricType) (*billing.Quota, error) {
	query := `
		SELECT user_id, metric, quota_limit, current_usage, reset_period, updated_at
		FROM quotas WHERE user_id = $1 AND metric = $2;
	`
	var q billing.Quota
	var metricStr string
	err := r.pool.QueryRow(ctx, query, tenantID, string(metric)).Scan(
		&q.TenantID, &metricStr, &q.Limit, &q.CurrentUsage, &q.ResetPeriod, &q.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrQuotaNotFound
		}
		return nil, err
	}
	q.Metric = billing.MetricType(metricStr)
	return &q, nil
}

func (r *QuotaRepository) ListQuotas(ctx context.Context, tenantID string) (billing.QuotaSet, error) {
	query := `
		SELECT user_id, metric, quota_limit, current_usage, reset_period, updated_at
		FROM quotas WHERE user_id = $1;
	`
	rows, err := r.pool.Query(ctx, query, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	set := make(billing.QuotaSet)
	for rows.Next() {
		var q billing.Quota
		var metricStr string
		if err := rows.Scan(&q.TenantID, &metricStr, &q.Limit, &q.CurrentUsage, &q.ResetPeriod, &q.UpdatedAt); err != nil {
			return nil, err
		}
		q.Metric = billing.MetricType(metricStr)
		set[q.Metric] = &q
	}
	return set, rows.Err()
}

func (r *QuotaRepository) SetQuota(ctx context.Context, quota *billing.Quota) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO quotas (user_id, metric, quota_limit, current_usage, reset_period, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (user_id, metric) DO UPDATE SET
			quota_limit = EXCLUDED.quota_limit,
			current_usage = EXCLUDED.current_usage,
			reset_period = EXCLUDED.reset_period,
			updated_at = EXCLUDED.updated_at;
	`
	_, err := r.pool.Exec(ctx, query,
		quota.TenantID, string(quota.Metric), quota.Limit, quota.CurrentUsage, quota.ResetPeriod, now,
	)
	quota.UpdatedAt = now
	return err
}

func (r *QuotaRepository) ReserveQuota(ctx context.Context, tenantID string, metric billing.MetricType, delta int64, limit int64) error {
	now := time.Now().UTC()
	query := `
		INSERT INTO quotas (user_id, metric, quota_limit, current_usage, reset_period, updated_at)
		VALUES ($1, $2, $3, $4, 'none', $5)
		ON CONFLICT (user_id, metric) DO UPDATE SET
			quota_limit = CASE WHEN $3 > 0 THEN $3 ELSE quotas.quota_limit END,
			current_usage = quotas.current_usage + $4,
			updated_at = $5
		WHERE (quotas.quota_limit = 0 OR quotas.current_usage + $4 <= quotas.quota_limit);
	`
	cmd, err := r.pool.Exec(ctx, query, tenantID, string(metric), limit, delta, now)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return fmt.Errorf("%w: metric %s quota exceeded", billing.ErrQuotaExceeded, metric)
	}
	return nil
}

func (r *QuotaRepository) ReleaseQuota(ctx context.Context, tenantID string, metric billing.MetricType, delta int64) error {
	now := time.Now().UTC()
	query := `
		UPDATE quotas SET
			current_usage = GREATEST(0, current_usage - $3),
			updated_at = $4
		WHERE user_id = $1 AND metric = $2;
	`
	_, err := r.pool.Exec(ctx, query, tenantID, string(metric), delta, now)
	return err
}
