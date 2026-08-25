package network

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/authz"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNetwork "github.com/aurora-vm/aurora/internal/domain/network"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNetworkService_FirewallAndNetworkConfig(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	connMgr := appNode.NewConnectionManager()
	nodeService := appNode.NewService(memStore.Nodes(), memStore.Enrollments(), ca, connMgr, memStore.Audit(), "127.0.0.1:9443")

	svc := NewService(memStore.Firewall(), memStore.Instances(), memStore.Nodes(), nodeService, authorizer, memStore.Audit())

	adminSubject := &identity.Subject{
		UserID:      "usr_admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	cust1Subject := &identity.Subject{
		UserID:      "usr_cust_1",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:read", "instance:update"},
	}

	cust2Subject := &identity.Subject{
		UserID:      "usr_cust_2",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:read", "instance:update"},
	}

	// Create Instance owned by Customer 1
	inst := &domainCompute.Instance{
		ID:        "inst-net-01",
		UserID:    cust1Subject.UserID,
		Name:      "web-server-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = memStore.Instances().Create(ctx, inst)
	require.NoError(t, err)

	// 1. Customer 1 applies firewall rules
	rules, err := svc.ApplyFirewallRules(ctx, cust1Subject, inst.ID, []FirewallRuleInput{
		{Direction: "inbound", Action: "allow", Protocol: "tcp", PortRange: "22", SourceCIDR: "192.168.1.0/24", Priority: 10},
		{Direction: "inbound", Action: "allow", Protocol: "tcp", PortRange: "443", SourceCIDR: "0.0.0.0/0", Priority: 20},
		{Direction: "inbound", Action: "drop", Protocol: "all", PortRange: "any", SourceCIDR: "0.0.0.0/0", Priority: 100},
	})
	require.NoError(t, err)
	assert.Len(t, rules, 3)

	// 2. Customer 1 reads firewall rules
	listed, err := svc.ListFirewallRules(ctx, cust1Subject, inst.ID)
	require.NoError(t, err)
	assert.Len(t, listed, 3)
	assert.Equal(t, "22", listed[0].PortRange)
	assert.Equal(t, domainNetwork.ActionAllow, listed[0].Action)

	// 3. Customer 2 tries to modify Customer 1's firewall -> 403 Forbidden!
	_, err = svc.ApplyFirewallRules(ctx, cust2Subject, inst.ID, []FirewallRuleInput{
		{Direction: "inbound", Action: "allow", Protocol: "tcp", PortRange: "80"},
	})
	assert.Error(t, err)

	// 4. Invalid protocol rejection
	_, err = svc.ApplyFirewallRules(ctx, cust1Subject, inst.ID, []FirewallRuleInput{
		{Direction: "inbound", Action: "allow", Protocol: "invalid_proto", PortRange: "80"},
	})
	assert.ErrorIs(t, err, domainNetwork.ErrInvalidProtocol)

	// 5. Configure Network Interface
	err = svc.ConfigureNetwork(ctx, adminSubject, inst.ID, "eth0", "192.168.100.15", "192.168.100.1", "", "", "00:16:3e:xx:xx:xx", 100)
	require.NoError(t, err)

	updatedInst, err := memStore.Instances().GetByID(ctx, inst.ID)
	require.NoError(t, err)
	assert.Equal(t, "192.168.100.15", updatedInst.IPv4Address)
}
