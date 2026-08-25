package network

import "errors"

// Standard domain errors for Networking & Firewall.
var (
	ErrFirewallRuleNotFound = errors.New("firewall rule not found")
	ErrInvalidRule          = errors.New("invalid firewall rule specification")
	ErrInvalidPortRange     = errors.New("invalid port range")
	ErrInvalidProtocol      = errors.New("unsupported firewall protocol")
)
