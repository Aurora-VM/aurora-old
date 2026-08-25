package template

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/authz"
	"github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainTmpl "github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/aurora-vm/aurora/internal/infra/imagesource"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTemplateTest(t *testing.T) (*Service, *memory.MemoryStore, *identity.Subject, *identity.Subject) {
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	imgSource := imagesource.NewRegistry([]string{"images", "ubuntu"})
	connManager := node.NewConnectionManager()

	nodeService := node.NewService(memStore.Nodes(), memStore.Enrollments(), nil, connManager, memStore.Audit(), "127.0.0.1:8443")
	service := NewService(memStore.Templates(), memStore.Images(), memStore.Nodes(), nodeService, imgSource, authorizer, memStore.Audit())

	adminSub := &identity.Subject{
		UserID:      "usr-admin",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	custSub := &identity.Subject{
		UserID:      "usr-cust",
		Roles:       []string{"customer"},
		Permissions: []string{"template:read"},
	}

	return service, memStore, adminSub, custSub
}

func TestTemplateService_TemplateLifecycle_And_RBAC(t *testing.T) {
	ctx := context.Background()
	svc, _, adminSub, custSub := setupTemplateTest(t)

	// 1. Customer attempts to create template -> Forbidden
	_, err := svc.CreateTemplate(ctx, custSub, CreateTemplateRequest{
		Name:         "Hacked OS",
		Slug:         "hacked-os",
		Distribution: "ubuntu",
		Version:      "24.04",
	})
	assert.ErrorIs(t, err, identity.ErrInsufficientPermission)

	// 2. Admin creates template
	tmpl, err := svc.CreateTemplate(ctx, adminSub, CreateTemplateRequest{
		Name:                   "Rocky Linux 9",
		Slug:                   "rocky-9",
		Description:            "Enterprise Linux enterprise OS",
		Distribution:           "rocky",
		Version:                "9",
		SupportedArchitectures: []string{"x86_64", "aarch64"},
		SupportedInstanceTypes: []compute.InstanceType{compute.TypeContainer, compute.TypeVirtualMachine},
		MinDiskBytes:           10 * 1024 * 1024 * 1024,
		MinMemoryBytes:         1024 * 1024 * 1024,
		CloudInitSupported:     true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, tmpl.ID)
	assert.Equal(t, "rocky-9", tmpl.Slug)

	// 3. Customer can read template
	readTmpl, err := svc.GetTemplate(ctx, custSub, "rocky-9")
	require.NoError(t, err)
	assert.Equal(t, tmpl.ID, readTmpl.ID)

	// 4. Update template
	updated, err := svc.UpdateTemplate(ctx, adminSub, tmpl.ID, UpdateTemplateRequest{
		Description: "Updated Enterprise Linux description",
		Status:      domainTmpl.TemplateStatusActive,
	})
	require.NoError(t, err)
	assert.Equal(t, "Updated Enterprise Linux description", updated.Description)

	// 5. Delete template
	err = svc.DeleteTemplate(ctx, adminSub, tmpl.ID)
	require.NoError(t, err)

	_, err = svc.GetTemplate(ctx, adminSub, tmpl.ID)
	assert.ErrorIs(t, err, domainTmpl.ErrTemplateNotFound)
}

func TestTemplateService_ImageArtifacts_And_Compatibility(t *testing.T) {
	ctx := context.Background()
	svc, _, adminSub, _ := setupTemplateTest(t)

	// Create test template
	tmpl, err := svc.CreateTemplate(ctx, adminSub, CreateTemplateRequest{
		Name:         "Ubuntu 24.04 LTS Test",
		Slug:         "ubuntu-24.04-test",
		Distribution: "ubuntu",
		Version:      "24.04",
	})
	require.NoError(t, err)

	fp1 := strings.Repeat("1", 64)
	fp2 := strings.Repeat("2", 64)

	// 1. Register x86_64 Container Image
	imgC, err := svc.RegisterImage(ctx, adminSub, RegisterImageRequest{
		TemplateID:       tmpl.ID,
		Architecture:     "x86_64",
		InstanceType:     compute.TypeContainer,
		IncusFingerprint: fp1,
		ImageAlias:       "images:ubuntu/24.04",
		SourceRemote:     "images",
		Checksum:         fp1,
		SizeBytes:        350 * 1024 * 1024,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, imgC.ID)

	// 2. Register aarch64 VM Image
	imgVM, err := svc.RegisterImage(ctx, adminSub, RegisterImageRequest{
		TemplateID:       tmpl.ID,
		Architecture:     "aarch64",
		InstanceType:     compute.TypeVirtualMachine,
		IncusFingerprint: fp2,
		ImageAlias:       "images:ubuntu/24.04/arm64",
		SourceRemote:     "images",
		Checksum:         fp2,
		SizeBytes:        1200 * 1024 * 1024,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, imgVM.ID)

	// 3. Find compatible image for x86_64 container
	compatC, err := svc.FindCompatibleImage(ctx, tmpl.ID, "x86_64", compute.TypeContainer)
	require.NoError(t, err)
	assert.Equal(t, imgC.ID, compatC.ID)

	// 4. Find compatible image for aarch64 VM
	compatVM, err := svc.FindCompatibleImage(ctx, tmpl.ID, "aarch64", compute.TypeVirtualMachine)
	require.NoError(t, err)
	assert.Equal(t, imgVM.ID, compatVM.ID)

	// 5. Incompatible query -> ErrNoCompatibleImage
	_, err = svc.FindCompatibleImage(ctx, tmpl.ID, "riscv64", compute.TypeContainer)
	assert.ErrorIs(t, err, domainTmpl.ErrNoCompatibleImage)

	// 6. Verify image checksum
	valid, err := svc.VerifyImage(ctx, adminSub, imgC.ID)
	require.NoError(t, err)
	assert.True(t, valid)

	// 7. Retire image
	err = svc.RetireImage(ctx, adminSub, imgC.ID)
	require.NoError(t, err)

	retiredImg, err := svc.GetImage(ctx, adminSub, imgC.ID)
	require.NoError(t, err)
	assert.Equal(t, domainTmpl.ImageStatusRetired, retiredImg.Status)
}

func TestTemplateService_SyncImageToNode(t *testing.T) {
	ctx := context.Background()
	svc, memStore, adminSub, _ := setupTemplateTest(t)

	// Create test node
	testNode := &domainNode.Node{
		ID:        "node-sync-01",
		Name:      "hypervisor-01",
		FQDN:      "192.168.1.50",
		Status:    domainNode.StatusOnline,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}
	require.NoError(t, memStore.Nodes().Create(ctx, testNode))

	// Register image
	fp := strings.Repeat("f", 64)
	img, err := svc.RegisterImage(ctx, adminSub, RegisterImageRequest{
		TemplateID:       "tmpl-ubuntu-24-04",
		Architecture:     "x86_64",
		InstanceType:     compute.TypeContainer,
		IncusFingerprint: fp,
		ImageAlias:       "images:ubuntu/24.04",
		SourceRemote:     "images",
		Checksum:         fp,
	})
	require.NoError(t, err)

	// Sync image to node
	err = svc.SyncImageToNode(ctx, adminSub, SyncImageRequest{
		ImageID: img.ID,
		NodeID:  testNode.ID,
	})
	require.NoError(t, err)
}
