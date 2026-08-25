package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/network"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type FirewallRepository struct {
	pool *pgxpool.Pool
}

func NewFirewallRepository(pool *pgxpool.Pool) *FirewallRepository {
	return &FirewallRepository{pool: pool}
}

func (r *FirewallRepository) Create(ctx context.Context, f *network.FirewallRule) error {
	query := `
		INSERT INTO firewall_rules (
			id, instance_id, direction, action, protocol, port_range, source_cidr, dest_cidr, priority, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	return r.pool.QueryRow(ctx, query,
		f.ID, f.InstanceID, string(f.Direction), string(f.Action), f.Protocol, f.PortRange, f.SourceCIDR, f.DestCIDR, f.Priority, now, now,
	).Scan(&f.ID, &f.CreatedAt, &f.UpdatedAt)
}

func (r *FirewallRepository) GetByID(ctx context.Context, id string) (*network.FirewallRule, error) {
	query := `
		SELECT id, instance_id, direction, action, protocol, port_range, source_cidr, dest_cidr, priority, created_at, updated_at
		FROM firewall_rules WHERE id = $1
	`
	var f network.FirewallRule
	var dir, act string
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&f.ID, &f.InstanceID, &dir, &act, &f.Protocol, &f.PortRange, &f.SourceCIDR, &f.DestCIDR, &f.Priority, &f.CreatedAt, &f.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, network.ErrFirewallRuleNotFound
		}
		return nil, err
	}
	f.Direction = network.Direction(dir)
	f.Action = network.Action(act)
	return &f, nil
}

func (r *FirewallRepository) ListByInstanceID(ctx context.Context, instanceID string) ([]*network.FirewallRule, error) {
	query := `
		SELECT id, instance_id, direction, action, protocol, port_range, source_cidr, dest_cidr, priority, created_at, updated_at
		FROM firewall_rules WHERE instance_id = $1
		ORDER BY priority ASC, created_at ASC
	`
	rows, err := r.pool.Query(ctx, query, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*network.FirewallRule
	for rows.Next() {
		var f network.FirewallRule
		var dir, act string
		if err := rows.Scan(
			&f.ID, &f.InstanceID, &dir, &act, &f.Protocol, &f.PortRange, &f.SourceCIDR, &f.DestCIDR, &f.Priority, &f.CreatedAt, &f.UpdatedAt,
		); err != nil {
			return nil, err
		}
		f.Direction = network.Direction(dir)
		f.Action = network.Action(act)
		results = append(results, &f)
	}
	return results, rows.Err()
}

func (r *FirewallRepository) ReplaceInstanceRules(ctx context.Context, instanceID string, rules []*network.FirewallRule) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. Delete old rules
	if _, err := tx.Exec(ctx, `DELETE FROM firewall_rules WHERE instance_id = $1`, instanceID); err != nil {
		return err
	}

	// 2. Insert new rules
	now := time.Now().UTC()
	for _, f := range rules {
		query := `
			INSERT INTO firewall_rules (
				id, instance_id, direction, action, protocol, port_range, source_cidr, dest_cidr, priority, created_at, updated_at
			) VALUES (
				COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
			)
		`
		if _, err := tx.Exec(ctx, query,
			f.ID, instanceID, string(f.Direction), string(f.Action), f.Protocol, f.PortRange, f.SourceCIDR, f.DestCIDR, f.Priority, now, now,
		); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *FirewallRepository) Delete(ctx context.Context, id string) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM firewall_rules WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return network.ErrFirewallRuleNotFound
	}
	return nil
}

func (r *FirewallRepository) DeleteByInstanceID(ctx context.Context, instanceID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM firewall_rules WHERE instance_id = $1`, instanceID)
	return err
}
