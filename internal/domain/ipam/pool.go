package ipam

import (
	"net"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// IPPool represents an allocatable CIDR subnet for virtual machine/container network interfaces.
type IPPool struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	LocationID string    `json:"locationId"` // DC or Region association
	IPVersion  int       `json:"ipVersion"`  // 4 or 6
	CIDR       string    `json:"cidr"`
	Gateway    string    `json:"gateway"`
	DNSServers []string  `json:"dnsServers"`
	VLANID     *int      `json:"vlanId,omitempty"`
	IsPrivate  bool      `json:"isPrivate"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Resource fulfills identity.Resource for RBAC permissions.
func (p *IPPool) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "ip_pool",
		ID:      p.ID,
		OwnerID: "", // Infrastructure level resource
	}
}

// PoolUtilization reports allocation statistics for an IPPool.
type PoolUtilization struct {
	PoolID           string  `json:"poolId"`
	CIDR             string  `json:"cidr"`
	TotalIPs         int64   `json:"totalIps"`
	AllocatedIPs     int64   `json:"allocatedIps"`
	ReservedIPs      int64   `json:"reservedIps"`
	FreeIPs          int64   `json:"freeIps"`
	UsagePercentage  float64 `json:"usagePercentage"`
}

// ContainsIP checks if a given IP address string belongs within this pool's CIDR.
func (p *IPPool) ContainsIP(ipStr string) bool {
	_, ipNet, err := net.ParseCIDR(p.CIDR)
	if err != nil {
		return false
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ipNet.Contains(ip)
}
