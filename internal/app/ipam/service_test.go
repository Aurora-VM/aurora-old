package ipam

import (
	"context"
	"testing"

	"github.com/aurora-vm/aurora/internal/app/authz"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainIPAM "github.com/aurora-vm/aurora/internal/domain/ipam"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIPAMService_FullWorkflow(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	svc := NewService(memStore.IPPools(), memStore.IPAllocations(), authorizer, memStore.Audit())

	adminSubject := &identity.Subject{
		UserID:      "usr_admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	custSubject := &identity.Subject{
		UserID:      "usr_cust",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:read"},
	}

	// 1. Customer attempts to create pool -> Forbidden
	_, err := svc.CreatePool(ctx, custSubject, CreatePoolRequest{
		Name:    "Public IPv4 DC1",
		CIDR:    "192.168.100.0/29", // /29 gives 6 usable IPs (.1 to .6)
		Gateway: "192.168.100.1",
	})
	assert.Error(t, err)

	// 2. Admin creates /29 pool (Usable: .2, .3, .4, .5, .6 since .1 is gateway)
	pool, err := svc.CreatePool(ctx, adminSubject, CreatePoolRequest{
		Name:       "Public IPv4 DC1",
		LocationID: "us-east-1",
		CIDR:       "192.168.100.0/29",
		Gateway:    "192.168.100.1",
		DNSServers: []string{"1.1.1.1", "8.8.8.8"},
	})
	require.NoError(t, err)
	assert.Equal(t, "192.168.100.0/29", pool.CIDR)

	// 3. Allocate IPs sequentially
	var allocations []*domainIPAM.IPAllocation
	for i := 0; i < 5; i++ {
		alloc, err := svc.AllocateIP(ctx, adminSubject, pool.ID, nil, "eth0", false, "vps allocation")
		require.NoError(t, err)
		allocations = append(allocations, alloc)
	}

	assert.Equal(t, "192.168.100.2", allocations[0].IPAddress)
	assert.Equal(t, "192.168.100.3", allocations[1].IPAddress)
	assert.Equal(t, "192.168.100.4", allocations[2].IPAddress)
	assert.Equal(t, "192.168.100.5", allocations[3].IPAddress)
	assert.Equal(t, "192.168.100.6", allocations[4].IPAddress)

	// 4. Next allocation should fail with ErrIPPoolExhausted!
	_, err = svc.AllocateIP(ctx, adminSubject, pool.ID, nil, "eth0", false, "exceed pool")
	assert.ErrorIs(t, err, domainIPAM.ErrIPPoolExhausted)

	// 5. Query Utilization
	_, util, err := svc.GetPool(ctx, adminSubject, pool.ID)
	require.NoError(t, err)
	assert.Equal(t, int64(6), util.TotalIPs)
	assert.Equal(t, int64(5), util.AllocatedIPs)
	assert.Equal(t, int64(1), util.FreeIPs) // .1 is gateway

	// 6. Release IP .3
	err = svc.ReleaseIP(ctx, adminSubject, allocations[1].ID)
	require.NoError(t, err)

	// 7. Allocate again -> should reuse released IP .3!
	reallocated, err := svc.AllocateIP(ctx, adminSubject, pool.ID, nil, "eth0", false, "reallocated")
	require.NoError(t, err)
	assert.Equal(t, "192.168.100.3", reallocated.IPAddress)
}
