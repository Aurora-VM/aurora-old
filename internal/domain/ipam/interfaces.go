package ipam

import "context"

// IPPoolRepository defines the persistence port for IP pools.
type IPPoolRepository interface {
	Create(ctx context.Context, pool *IPPool) error
	GetByID(ctx context.Context, id string) (*IPPool, error)
	GetByCIDR(ctx context.Context, cidr string) (*IPPool, error)
	List(ctx context.Context, locationID string) ([]*IPPool, error)
	Delete(ctx context.Context, id string) error
}

// IPAllocationRepository defines the persistence port for IP allocations.
type IPAllocationRepository interface {
	Create(ctx context.Context, alloc *IPAllocation) error
	GetByID(ctx context.Context, id string) (*IPAllocation, error)
	GetByIP(ctx context.Context, ip string) (*IPAllocation, error)
	ListByPoolID(ctx context.Context, poolID string) ([]*IPAllocation, error)
	ListByInstanceID(ctx context.Context, instanceID string) ([]*IPAllocation, error)
	Release(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}
