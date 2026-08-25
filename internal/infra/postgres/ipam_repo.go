package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/ipam"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// IPPoolRepository implements ipam.IPPoolRepository with PostgreSQL.
type IPPoolRepository struct {
	pool *pgxpool.Pool
}

func NewIPPoolRepository(pool *pgxpool.Pool) *IPPoolRepository {
	return &IPPoolRepository{pool: pool}
}

func (r *IPPoolRepository) Create(ctx context.Context, p *ipam.IPPool) error {
	query := `
		INSERT INTO ip_pools (
			id, name, location_id, ip_version, cidr, gateway, dns_servers, vlan_id, is_private, created_at, updated_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		) RETURNING id, created_at, updated_at
	`
	now := time.Now().UTC()
	err := r.pool.QueryRow(ctx, query,
		p.ID, p.Name, p.LocationID, p.IPVersion, p.CIDR, p.Gateway, p.DNSServers, p.VLANID, p.IsPrivate, now, now,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return ipam.ErrIPPoolAlreadyExists
		}
		return err
	}
	return nil
}

func (r *IPPoolRepository) GetByID(ctx context.Context, id string) (*ipam.IPPool, error) {
	query := `
		SELECT id, name, location_id, ip_version, cidr, gateway, dns_servers, vlan_id, is_private, created_at, updated_at
		FROM ip_pools WHERE id = $1
	`
	var p ipam.IPPool
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.Name, &p.LocationID, &p.IPVersion, &p.CIDR, &p.Gateway, &p.DNSServers, &p.VLANID, &p.IsPrivate, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ipam.ErrIPPoolNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *IPPoolRepository) GetByCIDR(ctx context.Context, cidr string) (*ipam.IPPool, error) {
	query := `
		SELECT id, name, location_id, ip_version, cidr, gateway, dns_servers, vlan_id, is_private, created_at, updated_at
		FROM ip_pools WHERE cidr = $1
	`
	var p ipam.IPPool
	err := r.pool.QueryRow(ctx, query, cidr).Scan(
		&p.ID, &p.Name, &p.LocationID, &p.IPVersion, &p.CIDR, &p.Gateway, &p.DNSServers, &p.VLANID, &p.IsPrivate, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ipam.ErrIPPoolNotFound
		}
		return nil, err
	}
	return &p, nil
}

func (r *IPPoolRepository) List(ctx context.Context, locationID string) ([]*ipam.IPPool, error) {
	query := `
		SELECT id, name, location_id, ip_version, cidr, gateway, dns_servers, vlan_id, is_private, created_at, updated_at
		FROM ip_pools
		WHERE ($1 = '' OR location_id = $1)
		ORDER BY created_at DESC
	`
	rows, err := r.pool.Query(ctx, query, locationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ipam.IPPool
	for rows.Next() {
		var p ipam.IPPool
		if err := rows.Scan(
			&p.ID, &p.Name, &p.LocationID, &p.IPVersion, &p.CIDR, &p.Gateway, &p.DNSServers, &p.VLANID, &p.IsPrivate, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &p)
	}
	return results, rows.Err()
}

func (r *IPPoolRepository) Delete(ctx context.Context, id string) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM ip_pools WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ipam.ErrIPPoolNotFound
	}
	return nil
}

// ---------------- IP ALLOCATION POSTGRES REPO ----------------

type IPAllocationRepository struct {
	pool *pgxpool.Pool
}

func NewIPAllocationRepository(pool *pgxpool.Pool) *IPAllocationRepository {
	return &IPAllocationRepository{pool: pool}
}

func (r *IPAllocationRepository) Create(ctx context.Context, a *ipam.IPAllocation) error {
	query := `
		INSERT INTO ip_allocations (
			id, pool_id, instance_id, ip_address, mac_address, interface_name, is_reserved, notes, allocated_at, released_at
		) VALUES (
			COALESCE(NULLIF($1, '')::uuid, gen_random_uuid()), $2, $3, $4, $5, $6, $7, $8, $9, $10
		) RETURNING id, allocated_at
	`
	now := time.Now().UTC()
	err := r.pool.QueryRow(ctx, query,
		a.ID, a.PoolID, a.InstanceID, a.IPAddress, a.MACAddress, a.InterfaceName, a.IsReserved, a.Notes, now, a.ReleasedAt,
	).Scan(&a.ID, &a.AllocatedAt)

	if err != nil {
		if isUniqueViolation(err) {
			return ipam.ErrIPAlreadyAllocated
		}
		return err
	}
	return nil
}

func (r *IPAllocationRepository) GetByID(ctx context.Context, id string) (*ipam.IPAllocation, error) {
	query := `
		SELECT id, pool_id, instance_id, ip_address, mac_address, interface_name, is_reserved, notes, allocated_at, released_at
		FROM ip_allocations WHERE id = $1
	`
	var a ipam.IPAllocation
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.PoolID, &a.InstanceID, &a.IPAddress, &a.MACAddress, &a.InterfaceName, &a.IsReserved, &a.Notes, &a.AllocatedAt, &a.ReleasedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ipam.ErrIPAllocationNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *IPAllocationRepository) GetByIP(ctx context.Context, ip string) (*ipam.IPAllocation, error) {
	query := `
		SELECT id, pool_id, instance_id, ip_address, mac_address, interface_name, is_reserved, notes, allocated_at, released_at
		FROM ip_allocations WHERE ip_address = $1 AND released_at IS NULL
	`
	var a ipam.IPAllocation
	err := r.pool.QueryRow(ctx, query, ip).Scan(
		&a.ID, &a.PoolID, &a.InstanceID, &a.IPAddress, &a.MACAddress, &a.InterfaceName, &a.IsReserved, &a.Notes, &a.AllocatedAt, &a.ReleasedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ipam.ErrIPAllocationNotFound
		}
		return nil, err
	}
	return &a, nil
}

func (r *IPAllocationRepository) ListByPoolID(ctx context.Context, poolID string) ([]*ipam.IPAllocation, error) {
	query := `
		SELECT id, pool_id, instance_id, ip_address, mac_address, interface_name, is_reserved, notes, allocated_at, released_at
		FROM ip_allocations WHERE pool_id = $1 AND released_at IS NULL
		ORDER BY allocated_at ASC
	`
	rows, err := r.pool.Query(ctx, query, poolID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ipam.IPAllocation
	for rows.Next() {
		var a ipam.IPAllocation
		if err := rows.Scan(
			&a.ID, &a.PoolID, &a.InstanceID, &a.IPAddress, &a.MACAddress, &a.InterfaceName, &a.IsReserved, &a.Notes, &a.AllocatedAt, &a.ReleasedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &a)
	}
	return results, rows.Err()
}

func (r *IPAllocationRepository) ListByInstanceID(ctx context.Context, instanceID string) ([]*ipam.IPAllocation, error) {
	query := `
		SELECT id, pool_id, instance_id, ip_address, mac_address, interface_name, is_reserved, notes, allocated_at, released_at
		FROM ip_allocations WHERE instance_id = $1 AND released_at IS NULL
		ORDER BY allocated_at ASC
	`
	rows, err := r.pool.Query(ctx, query, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*ipam.IPAllocation
	for rows.Next() {
		var a ipam.IPAllocation
		if err := rows.Scan(
			&a.ID, &a.PoolID, &a.InstanceID, &a.IPAddress, &a.MACAddress, &a.InterfaceName, &a.IsReserved, &a.Notes, &a.AllocatedAt, &a.ReleasedAt,
		); err != nil {
			return nil, err
		}
		results = append(results, &a)
	}
	return results, rows.Err()
}

func (r *IPAllocationRepository) Release(ctx context.Context, id string) error {
	query := `UPDATE ip_allocations SET released_at = NOW() WHERE id = $1 AND released_at IS NULL`
	cmd, err := r.pool.Exec(ctx, query, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ipam.ErrIPAllocationNotFound
	}
	return nil
}

func (r *IPAllocationRepository) Delete(ctx context.Context, id string) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM ip_allocations WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if cmd.RowsAffected() == 0 {
		return ipam.ErrIPAllocationNotFound
	}
	return nil
}
