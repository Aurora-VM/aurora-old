package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	domainReconcile "github.com/aurora-vm/aurora/internal/domain/reconcile"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ReconcileRepository implements domainReconcile.Repository using PostgreSQL.
type ReconcileRepository struct {
	pool *pgxpool.Pool
}

func NewReconcileRepository(pool *pgxpool.Pool) *ReconcileRepository {
	return &ReconcileRepository{pool: pool}
}

func (r *ReconcileRepository) SaveReport(ctx context.Context, rep *domainReconcile.Report) error {
	query := `
		INSERT INTO reconciliation_reports (
			id, trigger, dry_run, orphaned_instances_count, missing_nodes_count,
			stale_jobs_count, abandoned_migrations, inconsistent_quotas,
			total_discrepancies, repaired_count, unsafe_count, discrepancies,
			duration_ms, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14
		);
	`
	discrepancyBytes, _ := json.Marshal(rep.Discrepancies)
	if len(discrepancyBytes) == 0 {
		discrepancyBytes = []byte("[]")
	}

	_, err := r.pool.Exec(ctx, query,
		rep.ID, rep.Trigger, rep.DryRun, rep.OrphanedInstancesCount, rep.MissingNodesCount,
		rep.StaleJobsCount, rep.AbandonedMigrations, rep.InconsistentQuotas,
		rep.TotalDiscrepancies, rep.RepairedCount, rep.UnsafeCount, discrepancyBytes,
		rep.DurationMs, rep.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save reconciliation report: %w", err)
	}
	return nil
}

func (r *ReconcileRepository) GetLatestReport(ctx context.Context) (*domainReconcile.Report, error) {
	query := `
		SELECT id, trigger, dry_run, orphaned_instances_count, missing_nodes_count,
		       stale_jobs_count, abandoned_migrations, inconsistent_quotas,
		       total_discrepancies, repaired_count, unsafe_count, discrepancies,
		       duration_ms, created_at
		FROM reconciliation_reports
		ORDER BY created_at DESC
		LIMIT 1;
	`
	var rep domainReconcile.Report
	var discrepancyBytes []byte

	err := r.pool.QueryRow(ctx, query).Scan(
		&rep.ID, &rep.Trigger, &rep.DryRun, &rep.OrphanedInstancesCount, &rep.MissingNodesCount,
		&rep.StaleJobsCount, &rep.AbandonedMigrations, &rep.InconsistentQuotas,
		&rep.TotalDiscrepancies, &rep.RepairedCount, &rep.UnsafeCount, &discrepancyBytes,
		&rep.DurationMs, &rep.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get latest reconciliation report: %w", err)
	}

	_ = json.Unmarshal(discrepancyBytes, &rep.Discrepancies)
	return &rep, nil
}

func (r *ReconcileRepository) ListReports(ctx context.Context, limit, offset int) ([]*domainReconcile.Report, int, error) {
	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM reconciliation_reports;").Scan(&total); err != nil {
		return nil, 0, err
	}

	query := `
		SELECT id, trigger, dry_run, orphaned_instances_count, missing_nodes_count,
		       stale_jobs_count, abandoned_migrations, inconsistent_quotas,
		       total_discrepancies, repaired_count, unsafe_count, discrepancies,
		       duration_ms, created_at
		FROM reconciliation_reports
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2;
	`
	if limit <= 0 {
		limit = 50
	}

	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var reports []*domainReconcile.Report
	for rows.Next() {
		var rep domainReconcile.Report
		var discrepancyBytes []byte

		if err := rows.Scan(
			&rep.ID, &rep.Trigger, &rep.DryRun, &rep.OrphanedInstancesCount, &rep.MissingNodesCount,
			&rep.StaleJobsCount, &rep.AbandonedMigrations, &rep.InconsistentQuotas,
			&rep.TotalDiscrepancies, &rep.RepairedCount, &rep.UnsafeCount, &discrepancyBytes,
			&rep.DurationMs, &rep.CreatedAt,
		); err != nil {
			return nil, 0, err
		}

		_ = json.Unmarshal(discrepancyBytes, &rep.Discrepancies)
		reports = append(reports, &rep)
	}

	return reports, total, nil
}
