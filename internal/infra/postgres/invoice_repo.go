package postgres

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/billing"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// InvoiceRepository implements billing.InvoiceRepository using PostgreSQL.
type InvoiceRepository struct {
	pool *pgxpool.Pool
}

func NewInvoiceRepository(pool *pgxpool.Pool) *InvoiceRepository {
	return &InvoiceRepository{pool: pool}
}

func (r *InvoiceRepository) Create(ctx context.Context, inv *billing.Invoice) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	now := time.Now().UTC()
	query := `
		INSERT INTO invoices (
			id, user_id, subscription_id, currency, subtotal_minor, tax_minor, total_minor,
			status, period_start, period_end, due_at, paid_at, idempotency_key, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, NULLIF($3, '')::uuid, $4, $5, $6, $7,
			$8, $9, $10, $11, $12, NULLIF($13, ''), $14
		)
		ON CONFLICT (idempotency_key) DO NOTHING
		RETURNING id, created_at;
	`
	err = tx.QueryRow(ctx, query,
		inv.ID, inv.TenantID, inv.SubscriptionID, inv.Currency,
		inv.SubtotalMinor, inv.TaxMinor, inv.TotalMinor,
		string(inv.Status), inv.PeriodStart, inv.PeriodEnd, inv.DueAt, inv.PaidAt,
		inv.IdempotencyKey, now,
	).Scan(&inv.ID, &inv.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "no rows in result set") {
			// Idempotent duplicate: fetch existing invoice
			existing, getErr := r.GetByIdempotencyKey(ctx, inv.IdempotencyKey)
			if getErr == nil && existing != nil {
				*inv = *existing
				return nil
			}
		}
		return err
	}

	lineQuery := `
		INSERT INTO invoice_lines (
			id, invoice_id, description, metric, quantity, unit_price_minor, total_minor, created_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8
		) RETURNING id, created_at;
	`
	for _, l := range inv.Lines {
		var lineCreated time.Time
		err := tx.QueryRow(ctx, lineQuery,
			l.ID, inv.ID, l.Description, string(l.Metric), l.Quantity, l.UnitPriceMinor, l.TotalMinor, now,
		).Scan(&l.ID, &lineCreated)
		if err != nil {
			return err
		}
		l.InvoiceID = inv.ID
	}

	return tx.Commit(ctx)
}

func (r *InvoiceRepository) GetByID(ctx context.Context, id string) (*billing.Invoice, error) {
	query := `
		SELECT id, user_id, COALESCE(subscription_id::text, ''), currency, subtotal_minor, tax_minor, total_minor,
			status, period_start, period_end, due_at, paid_at, COALESCE(idempotency_key, ''), created_at
		FROM invoices WHERE id = $1;
	`
	inv, err := r.scanInvoice(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		return nil, err
	}
	lines, err := r.loadLines(ctx, inv.ID)
	if err != nil {
		return nil, err
	}
	inv.Lines = lines
	return inv, nil
}

func (r *InvoiceRepository) GetByIdempotencyKey(ctx context.Context, key string) (*billing.Invoice, error) {
	query := `
		SELECT id, user_id, COALESCE(subscription_id::text, ''), currency, subtotal_minor, tax_minor, total_minor,
			status, period_start, period_end, due_at, paid_at, COALESCE(idempotency_key, ''), created_at
		FROM invoices WHERE idempotency_key = $1;
	`
	inv, err := r.scanInvoice(r.pool.QueryRow(ctx, query, key))
	if err != nil {
		return nil, err
	}
	lines, err := r.loadLines(ctx, inv.ID)
	if err != nil {
		return nil, err
	}
	inv.Lines = lines
	return inv, nil
}

func (r *InvoiceRepository) ListByTenant(ctx context.Context, tenantID string, limit, offset int) ([]*billing.Invoice, error) {
	query := `
		SELECT id, user_id, COALESCE(subscription_id::text, ''), currency, subtotal_minor, tax_minor, total_minor,
			status, period_start, period_end, due_at, paid_at, COALESCE(idempotency_key, ''), created_at
		FROM invoices
		WHERE user_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3;
	`
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, query, tenantID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*billing.Invoice
	for rows.Next() {
		inv, err := r.scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, inv)
	}

	for _, inv := range result {
		lines, _ := r.loadLines(ctx, inv.ID)
		inv.Lines = lines
	}

	return result, rows.Err()
}

func (r *InvoiceRepository) ListAll(ctx context.Context, limit, offset int) ([]*billing.Invoice, error) {
	query := `
		SELECT id, user_id, COALESCE(subscription_id::text, ''), currency, subtotal_minor, tax_minor, total_minor,
			status, period_start, period_end, due_at, paid_at, COALESCE(idempotency_key, ''), created_at
		FROM invoices
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2;
	`
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, query, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*billing.Invoice
	for rows.Next() {
		inv, err := r.scanInvoice(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, inv)
	}

	for _, inv := range result {
		lines, _ := r.loadLines(ctx, inv.ID)
		inv.Lines = lines
	}

	return result, rows.Err()
}

func (r *InvoiceRepository) UpdateStatus(ctx context.Context, id string, status billing.InvoiceStatus, paidAt *time.Time) error {
	query := `
		UPDATE invoices SET status = $2, paid_at = $3 WHERE id = $1;
	`
	cmd, err := r.pool.Exec(ctx, query, id, string(status), paidAt)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return billing.ErrInvoiceNotFound
	}
	return nil
}

func (r *InvoiceRepository) loadLines(ctx context.Context, invoiceID string) ([]*billing.InvoiceLine, error) {
	query := `
		SELECT id, invoice_id, description, metric, quantity, unit_price_minor, total_minor
		FROM invoice_lines
		WHERE invoice_id = $1
		ORDER BY created_at ASC;
	`
	rows, err := r.pool.Query(ctx, query, invoiceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var lines []*billing.InvoiceLine
	for rows.Next() {
		var l billing.InvoiceLine
		var metricStr string
		if err := rows.Scan(&l.ID, &l.InvoiceID, &l.Description, &metricStr, &l.Quantity, &l.UnitPriceMinor, &l.TotalMinor); err != nil {
			return nil, err
		}
		l.Metric = billing.MetricType(metricStr)
		lines = append(lines, &l)
	}
	return lines, rows.Err()
}

func (r *InvoiceRepository) scanInvoice(row pgx.Row) (*billing.Invoice, error) {
	var inv billing.Invoice
	var statusStr string

	err := row.Scan(
		&inv.ID, &inv.TenantID, &inv.SubscriptionID, &inv.Currency,
		&inv.SubtotalMinor, &inv.TaxMinor, &inv.TotalMinor,
		&statusStr, &inv.PeriodStart, &inv.PeriodEnd, &inv.DueAt, &inv.PaidAt,
		&inv.IdempotencyKey, &inv.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, billing.ErrInvoiceNotFound
		}
		return nil, err
	}
	inv.Status = billing.InvoiceStatus(statusStr)
	return &inv, nil
}
