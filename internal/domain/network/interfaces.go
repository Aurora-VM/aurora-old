package network

import "context"

// FirewallRepository defines persistence for instance firewall rules.
type FirewallRepository interface {
	Create(ctx context.Context, rule *FirewallRule) error
	GetByID(ctx context.Context, id string) (*FirewallRule, error)
	ListByInstanceID(ctx context.Context, instanceID string) ([]*FirewallRule, error)
	ReplaceInstanceRules(ctx context.Context, instanceID string, rules []*FirewallRule) error
	Delete(ctx context.Context, id string) error
	DeleteByInstanceID(ctx context.Context, instanceID string) error
}

// NetworkDriver defines host/hypervisor-level network and firewall application port.
type NetworkDriver interface {
	ConfigureInterface(ctx context.Context, instanceName, ifaceName, ipv4, gw4, ipv6, gw6, mac string, vlan int) error
	ApplyFirewall(ctx context.Context, instanceName string, rules []*FirewallRule) error
}
