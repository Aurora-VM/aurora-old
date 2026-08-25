package network

import (
	"context"
	"fmt"
	"strings"
	"time"

	appNode "github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNetwork "github.com/aurora-vm/aurora/internal/domain/network"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	"github.com/google/uuid"
)

// Service coordinates virtual networking and firewall configuration.
type Service struct {
	firewallRepo domainNetwork.FirewallRepository
	instanceRepo domainCompute.InstanceRepository
	nodeRepo     domainNode.NodeRepository
	nodeService  *appNode.Service
	authorizer   identity.Authorizer
	auditRepo    audit.Repository
}

// NewService constructs a Network & Firewall application service.
func NewService(
	firewallRepo domainNetwork.FirewallRepository,
	instanceRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	nodeService *appNode.Service,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		firewallRepo: firewallRepo,
		instanceRepo: instanceRepo,
		nodeRepo:     nodeRepo,
		nodeService:  nodeService,
		authorizer:   authorizer,
		auditRepo:    auditRepo,
	}
}

type FirewallRuleInput struct {
	Direction  string `json:"direction"`  // "inbound" or "outbound"
	Action     string `json:"action"`     // "allow" or "drop"
	Protocol   string `json:"protocol"`   // "tcp", "udp", "icmp", "all"
	PortRange  string `json:"portRange"`  // "80", "443", "1000-2000", "any"
	SourceCIDR string `json:"sourceCidr"` // "0.0.0.0/0"
	DestCIDR   string `json:"destCidr"`   // "0.0.0.0/0"
	Priority   int    `json:"priority"`
}

func (s *Service) ApplyFirewallRules(ctx context.Context, sub *identity.Subject, instanceID string, inputs []FirewallRuleInput) ([]*domainNetwork.FirewallRule, error) {
	inst, err := s.instanceRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, domainCompute.ErrInstanceNotFound
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return nil, err
	}

	var rules []*domainNetwork.FirewallRule
	for i, inp := range inputs {
		dir := domainNetwork.Direction(strings.ToLower(inp.Direction))
		if dir != domainNetwork.DirectionInbound && dir != domainNetwork.DirectionOutbound {
			return nil, fmt.Errorf("rule #%d has invalid direction: %s", i+1, inp.Direction)
		}

		act := domainNetwork.Action(strings.ToLower(inp.Action))
		if act != domainNetwork.ActionAllow && act != domainNetwork.ActionDrop {
			return nil, fmt.Errorf("rule #%d has invalid action: %s", i+1, inp.Action)
		}

		proto := strings.ToLower(inp.Protocol)
		if proto == "" {
			proto = "tcp"
		}
		if proto != "tcp" && proto != "udp" && proto != "icmp" && proto != "all" {
			return nil, domainNetwork.ErrInvalidProtocol
		}

		port := inp.PortRange
		if port == "" {
			port = "any"
		}

		srcCIDR := inp.SourceCIDR
		if srcCIDR == "" {
			srcCIDR = "0.0.0.0/0"
		}

		dstCIDR := inp.DestCIDR
		if dstCIDR == "" {
			dstCIDR = "0.0.0.0/0"
		}

		priority := inp.Priority
		if priority <= 0 {
			priority = 100 + i*10
		}

		rule := &domainNetwork.FirewallRule{
			ID:         uuid.NewString(),
			InstanceID: instanceID,
			Direction:  dir,
			Action:     act,
			Protocol:   proto,
			PortRange:  port,
			SourceCIDR: srcCIDR,
			DestCIDR:   dstCIDR,
			Priority:   priority,
			CreatedAt:  time.Now().UTC(),
			UpdatedAt:  time.Now().UTC(),
		}
		rules = append(rules, rule)
	}

	if err := s.firewallRepo.ReplaceInstanceRules(ctx, instanceID, rules); err != nil {
		return nil, err
	}

	// Dispatch to hypervisor node if assigned
	if inst.NodeID != "" {
		cmd := &domainNode.Command{
			Type: "apply_firewall_rules",
			Payload: map[string]interface{}{
				"instance_id":   instanceID,
				"instance_name": inst.Name,
				"rules_count":   len(rules),
			},
		}
		_, _ = s.nodeService.SendCommand(ctx, inst.NodeID, cmd)
	}

	actorID := sub.UserID
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "instance:firewall:update",
		ActorID:      &actorID,
		ResourceType: "instance",
		ResourceID:   &instanceID,
		CreatedAt:    time.Now().UTC(),
	})

	return rules, nil
}

func (s *Service) ListFirewallRules(ctx context.Context, sub *identity.Subject, instanceID string) ([]*domainNetwork.FirewallRule, error) {
	inst, err := s.instanceRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, domainCompute.ErrInstanceNotFound
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
		return nil, err
	}

	return s.firewallRepo.ListByInstanceID(ctx, instanceID)
}

func (s *Service) ConfigureNetwork(ctx context.Context, sub *identity.Subject, instanceID, ifaceName, ipv4, gw4, ipv6, gw6, mac string, vlan int) error {
	inst, err := s.instanceRepo.GetByID(ctx, instanceID)
	if err != nil {
		return domainCompute.ErrInstanceNotFound
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return err
	}

	if ifaceName == "" {
		ifaceName = "eth0"
	}

	// Update instance IP addresses in repo
	if ipv4 == "" {
		ipv4 = inst.IPv4Address
	}
	if ipv6 == "" {
		ipv6 = inst.IPv6Address
	}
	_ = s.instanceRepo.UpdateStatus(ctx, inst.ID, inst.Status, ipv4, ipv6)

	// Dispatch to hypervisor node if assigned
	if inst.NodeID != "" {
		cmd := &domainNode.Command{
			Type: "configure_network",
			Payload: map[string]interface{}{
				"instance_id":    instanceID,
				"instance_name":  inst.Name,
				"interface_name": ifaceName,
				"ipv4_address":   ipv4,
				"ipv4_gateway":   gw4,
				"ipv6_address":   ipv6,
				"ipv6_gateway":   gw6,
				"mac_address":    mac,
				"vlan_id":        vlan,
			},
		}
		_, err := s.nodeService.SendCommand(ctx, inst.NodeID, cmd)
		if err != nil {
			return fmt.Errorf("failed to apply network configuration on node: %w", err)
		}
	}

	actorID := sub.UserID
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		Action:       "instance:network:configure",
		ActorID:      &actorID,
		ResourceType: "instance",
		ResourceID:   &instanceID,
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}
