package compute

import (
	"context"
)

// InstanceRepository defines the database persistence port for instances.
type InstanceRepository interface {
	Create(ctx context.Context, inst *Instance) error
	GetByID(ctx context.Context, id string) (*Instance, error)
	GetByName(ctx context.Context, name string) (*Instance, error)
	ListByUser(ctx context.Context, userID string) ([]*Instance, error)
	ListByNode(ctx context.Context, nodeID string) ([]*Instance, error)
	ListAll(ctx context.Context) ([]*Instance, error)
	UpdateStatus(ctx context.Context, id string, status Status, ipv4, ipv6 string) error
	UpdateSpec(ctx context.Context, id string, cpu int, memory, storage int64) error
	UpdateNodeID(ctx context.Context, id string, nodeID string) error
	Delete(ctx context.Context, id string) error
}

// HypervisorDriver defines the abstraction for interacting with local Incus virtualization engines.
type HypervisorDriver interface {
	CreateInstance(ctx context.Context, spec *InstanceSpec) (*InstanceInfo, error)
	StartInstance(ctx context.Context, name string) error
	StopInstance(ctx context.Context, name string, force bool) error
	RestartInstance(ctx context.Context, name string, force bool) error
	DeleteInstance(ctx context.Context, name string, force bool) error
	UpdateSpec(ctx context.Context, name string, cpu int, memory, storage int64) error
	GetInstance(ctx context.Context, name string) (*InstanceInfo, error)
	GetMetrics(ctx context.Context, name string) (*InstanceMetrics, error)
	ListInstances(ctx context.Context) ([]*InstanceInfo, error)
}
