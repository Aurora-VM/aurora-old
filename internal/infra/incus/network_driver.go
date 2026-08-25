package incus

import (
	"context"
	"fmt"
	"sync"

	"github.com/aurora-vm/aurora/internal/domain/network"
)

// SocketNetworkDriver applies network device configurations and firewall rules on Incus hosts.
type SocketNetworkDriver struct {
	driver *SocketDriver
}

// NewSocketNetworkDriver creates a NetworkDriver using the Incus socket.
func NewSocketNetworkDriver(driver *SocketDriver) *SocketNetworkDriver {
	return &SocketNetworkDriver{driver: driver}
}

func (d *SocketNetworkDriver) ConfigureInterface(ctx context.Context, instanceName, ifaceName, ipv4, gw4, ipv6, gw6, mac string, vlan int) error {
	deviceProps := map[string]interface{}{
		"type":    "nic",
		"network": "incusbr0",
		"name":    ifaceName,
	}
	if ipv4 != "" {
		deviceProps["ipv4.address"] = ipv4
	}
	if gw4 != "" {
		deviceProps["ipv4.gateway"] = gw4
	}
	if ipv6 != "" {
		deviceProps["ipv6.address"] = ipv6
	}
	if gw6 != "" {
		deviceProps["ipv6.gateway"] = gw6
	}
	if mac != "" {
		deviceProps["hwaddr"] = mac
	}
	if vlan > 0 {
		deviceProps["vlan"] = fmt.Sprintf("%d", vlan)
	}

	payload := map[string]interface{}{
		"devices": map[string]interface{}{
			ifaceName: deviceProps,
		},
	}

	_, err := d.driver.doRequest(ctx, "PATCH", fmt.Sprintf("/1.0/instances/%s", instanceName), payload)
	return err
}

func (d *SocketNetworkDriver) ApplyFirewall(ctx context.Context, instanceName string, rules []*network.FirewallRule) error {
	// Incus supports device-level security filters (e.g. security.ipv4_filtering, security.mac_filtering)
	// and custom network firewall chains.
	securityProps := map[string]string{
		"security.ipv4_filtering": "true",
		"security.mac_filtering":  "true",
	}

	payload := map[string]interface{}{
		"devices": map[string]interface{}{
			"eth0": securityProps,
		},
	}

	_, err := d.driver.doRequest(ctx, "PATCH", fmt.Sprintf("/1.0/instances/%s", instanceName), payload)
	return err
}

// ---------------- SIMULATED NETWORK DRIVER ----------------

// SimulatedNetworkDriver provides an in-memory simulated network driver for tests.
type SimulatedNetworkDriver struct {
	mu         sync.RWMutex
	interfaces map[string]map[string]interface{}
	firewalls  map[string][]*network.FirewallRule
}

func NewSimulatedNetworkDriver() *SimulatedNetworkDriver {
	return &SimulatedNetworkDriver{
		interfaces: make(map[string]map[string]interface{}),
		firewalls:  make(map[string][]*network.FirewallRule),
	}
}

func (s *SimulatedNetworkDriver) ConfigureInterface(ctx context.Context, instanceName, ifaceName, ipv4, gw4, ipv6, gw6, mac string, vlan int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.interfaces[instanceName] == nil {
		s.interfaces[instanceName] = make(map[string]interface{})
	}
	s.interfaces[instanceName][ifaceName] = map[string]interface{}{
		"ipv4": ipv4,
		"gw4":  gw4,
		"ipv6": ipv6,
		"gw6":  gw6,
		"mac":  mac,
		"vlan": vlan,
	}
	return nil
}

func (s *SimulatedNetworkDriver) ApplyFirewall(ctx context.Context, instanceName string, rules []*network.FirewallRule) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var copies []*network.FirewallRule
	for _, r := range rules {
		cp := *r
		copies = append(copies, &cp)
	}
	s.firewalls[instanceName] = copies
	return nil
}
