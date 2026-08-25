package ipam

import (
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// IPAllocation represents a leased or reserved IP address mapped to an instance interface.
type IPAllocation struct {
	ID            string     `json:"id"`
	PoolID        string     `json:"poolId"`
	InstanceID    *string    `json:"instanceId,omitempty"`
	IPAddress     string     `json:"ipAddress"`
	MACAddress    string     `json:"macAddress,omitempty"`
	InterfaceName string     `json:"interfaceName"`
	IsReserved    bool       `json:"isReserved"`
	Notes         string     `json:"notes,omitempty"`
	AllocatedAt   time.Time  `json:"allocatedAt"`
	ReleasedAt    *time.Time `json:"releasedAt,omitempty"`
}

// Resource fulfills identity.Resource for RBAC permissions.
func (a *IPAllocation) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "ip_allocation",
		ID:      a.ID,
		OwnerID: "",
	}
}
