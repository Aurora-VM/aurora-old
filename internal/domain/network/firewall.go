package network

import (
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// Direction represents inbound or outbound traffic.
type Direction string

const (
	DirectionInbound  Direction = "inbound"
	DirectionOutbound Direction = "outbound"
)

// Action represents packet action (allow or drop).
type Action string

const (
	ActionAllow Action = "allow"
	ActionDrop  Action = "drop"
)

// FirewallRule represents a security rule enforced on an instance virtual interface.
type FirewallRule struct {
	ID         string    `json:"id"`
	InstanceID string    `json:"instanceId"`
	Direction  Direction `json:"direction"`
	Action     Action    `json:"action"`
	Protocol   string    `json:"protocol"`   // "tcp", "udp", "icmp", "all"
	PortRange  string    `json:"portRange"`  // "80", "443", "1000-2000", "any"
	SourceCIDR string    `json:"sourceCidr"` // "0.0.0.0/0"
	DestCIDR   string    `json:"destCidr"`   // "0.0.0.0/0"
	Priority   int       `json:"priority"`   // Lower number = higher priority
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// Resource fulfills identity.Resource for tenancy checks.
func (f *FirewallRule) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "firewall_rule",
		ID:      f.ID,
		OwnerID: "",
	}
}
