package storage

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/authz"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainStorage "github.com/aurora-vm/aurora/internal/domain/storage"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStorageService_FullLifecycle_And_Tenancy(t *testing.T) {
	ctx := context.Background()
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	connMgr := appNode.NewConnectionManager()
	nodeService := appNode.NewService(memStore.Nodes(), memStore.Enrollments(), ca, connMgr, memStore.Audit(), "127.0.0.1:9443")

	svc := NewService(
		memStore.StoragePools(),
		memStore.Volumes(),
		memStore.Snapshots(),
		memStore.Instances(),
		memStore.Nodes(),
		nodeService,
		authorizer,
		memStore.Audit(),
	)

	adminSubject := &identity.Subject{
		UserID:      "usr_admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	cust1Subject := &identity.Subject{
		UserID: "usr_cust_1",
		Roles:  []string{"customer"},
		Permissions: []string{
			"instance:read", "instance:create", "instance:update", "instance:delete",
			"volume:read", "volume:create", "volume:update", "volume:delete",
			"volume:attach", "volume:detach", "volume:snapshot", "volume:restore",
			"storage:read",
		},
	}

	cust2Subject := &identity.Subject{
		UserID: "usr_cust_2",
		Roles:  []string{"customer"},
		Permissions: []string{
			"instance:read", "instance:create", "instance:update", "instance:delete",
			"volume:read", "volume:create", "volume:update", "volume:delete",
			"volume:attach", "volume:detach", "volume:snapshot", "volume:restore",
			"storage:read",
		},
	}

	// 1. Create Node
	node := &domainNode.Node{
		ID:        "node-storage-01",
		Name:      "hv-storage-01",
		FQDN:      "127.0.0.1",
		Status:    domainNode.StatusOnline,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = memStore.Nodes().Create(ctx, node)
	require.NoError(t, err)

	// 2. Admin creates StoragePool (ZFS)
	pool, err := svc.CreateStoragePool(ctx, adminSubject, CreateStoragePoolRequest{
		NodeID:          node.ID,
		Name:            "zfs-nvme-pool",
		Driver:          domainStorage.DriverZFS,
		TotalSpaceBytes: 1099511627776, // 1 TiB
	})
	require.NoError(t, err)
	assert.NotEmpty(t, pool.ID)
	assert.Equal(t, domainStorage.DriverZFS, pool.Driver)

	// 3. Customer 1 lists storage pools
	pools, err := svc.ListStoragePools(ctx, cust1Subject, node.ID)
	require.NoError(t, err)
	assert.Len(t, pools, 1)

	// 4. Customer 1 creates custom volume (50 GiB)
	vol, err := svc.CreateVolume(ctx, cust1Subject, CreateVolumeRequest{
		PoolID:      pool.ID,
		Name:        "db-data-volume",
		SizeBytes:   53687091200, // 50 GiB
		ContentType: domainStorage.ContentTypeFilesystem,
		MountPath:   "/var/lib/postgresql/data",
	})
	require.NoError(t, err)
	assert.NotEmpty(t, vol.ID)
	assert.Equal(t, cust1Subject.UserID, vol.UserID)

	// 5. Customer 1 resizes volume to 100 GiB
	resizedVol, err := svc.ResizeVolume(ctx, cust1Subject, vol.ID, 107374182400)
	require.NoError(t, err)
	assert.Equal(t, int64(107374182400), resizedVol.SizeBytes)

	// 6. Customer 1 creates snapshot
	snap, err := svc.CreateSnapshot(ctx, cust1Subject, vol.ID, "db-pre-migration-snap")
	require.NoError(t, err)
	assert.NotEmpty(t, snap.ID)
	assert.Equal(t, "db-pre-migration-snap", snap.Name)

	// 7. Customer 1 provisions an instance and attaches volume
	inst := &domainCompute.Instance{
		ID:        "inst-storage-01",
		UserID:    cust1Subject.UserID,
		NodeID:    node.ID,
		Name:      "db-master-01",
		Type:      domainCompute.TypeContainer,
		Status:    domainCompute.StatusRunning,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	err = memStore.Instances().Create(ctx, inst)
	require.NoError(t, err)

	attachedVol, err := svc.AttachVolume(ctx, cust1Subject, vol.ID, inst.ID, "/mnt/db-data", false)
	require.NoError(t, err)
	assert.True(t, attachedVol.IsAttached())
	assert.Equal(t, inst.ID, *attachedVol.InstanceID)

	// 8. Customer 2 tries to delete Customer 1's volume -> 403 Forbidden
	err = svc.DeleteVolume(ctx, cust2Subject, vol.ID)
	assert.Error(t, err)

	// 9. Customer 1 tries to delete attached volume -> ErrVolumeAttached
	err = svc.DeleteVolume(ctx, cust1Subject, vol.ID)
	assert.ErrorIs(t, err, domainStorage.ErrVolumeAttached)

	// 10. Customer 1 detaches volume and then deletes it
	detachedVol, err := svc.DetachVolume(ctx, cust1Subject, vol.ID)
	require.NoError(t, err)
	assert.False(t, detachedVol.IsAttached())

	err = svc.DeleteVolume(ctx, cust1Subject, vol.ID)
	require.NoError(t, err)

	// Volume no longer found
	_, err = svc.GetVolume(ctx, cust1Subject, vol.ID)
	assert.ErrorIs(t, err, domainStorage.ErrVolumeNotFound)
}
