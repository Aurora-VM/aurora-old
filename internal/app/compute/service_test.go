package compute

import (
	"context"
	"testing"
	"time"

	"github.com/aurora-vm/aurora/internal/app/authz"
	appNode "github.com/aurora-vm/aurora/internal/app/node"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainTmpl "github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/aurora-vm/aurora/internal/infra/incus"
	"github.com/aurora-vm/aurora/internal/infra/memory"
	"github.com/aurora-vm/aurora/internal/infra/pki"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockComputeStreamSender struct {
	driver      domainCompute.HypervisorDriver
	nodeService *appNode.Service
}

func (m *mockComputeStreamSender) Send(cmd *domainNode.Command) error {
	ctx := context.Background()
	switch cmd.Type {
	case "create_instance":
		spec := &domainCompute.InstanceSpec{
			Name:             cmd.Payload["name"].(string),
			Type:             domainCompute.Type(cmd.Payload["type"].(string)),
			CPUCores:         cmd.Payload["cpu_cores"].(int),
			MemoryBytes:      cmd.Payload["memory_bytes"].(int64),
			StorageBytes:     cmd.Payload["storage_bytes"].(int64),
			Image:            cmd.Payload["image"].(string),
			StartAfterCreate: cmd.Payload["start_after_create"].(bool),
		}
		info, err := m.driver.CreateInstance(ctx, spec)
		success := err == nil
		errMsg := ""
		if err != nil {
			errMsg = err.Error()
		}
		payload := map[string]interface{}{}
		if info != nil {
			payload["name"] = info.Name
			payload["status"] = string(info.Status)
			payload["ipv4Address"] = info.IPv4Address
			payload["ipv6Address"] = info.IPv6Address
		}
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       success,
			ErrorMessage:  errMsg,
			Payload:       payload,
			CompletedAt:   time.Now().UTC(),
		})

	case "start_instance":
		name := cmd.Payload["name"].(string)
		err := m.driver.StartInstance(ctx, name)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"status": "running"},
			CompletedAt:   time.Now().UTC(),
		})

	case "stop_instance":
		name := cmd.Payload["name"].(string)
		force := cmd.Payload["force"].(bool)
		err := m.driver.StopInstance(ctx, name, force)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"status": "stopped"},
			CompletedAt:   time.Now().UTC(),
		})

	case "restart_instance":
		name := cmd.Payload["name"].(string)
		force := cmd.Payload["force"].(bool)
		err := m.driver.RestartInstance(ctx, name, force)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"status": "running"},
			CompletedAt:   time.Now().UTC(),
		})

	case "delete_instance":
		name := cmd.Payload["name"].(string)
		force := cmd.Payload["force"].(bool)
		err := m.driver.DeleteInstance(ctx, name, force)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"deleted": true},
			CompletedAt:   time.Now().UTC(),
		})

	case "update_instance_spec":
		name := cmd.Payload["name"].(string)
		cpu := cmd.Payload["cpu_cores"].(int)
		mem := cmd.Payload["memory_bytes"].(int64)
		disk := cmd.Payload["storage_bytes"].(int64)
		err := m.driver.UpdateSpec(ctx, name, cpu, mem, disk)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload:       map[string]interface{}{"updated": true},
			CompletedAt:   time.Now().UTC(),
		})

	case "get_instance_metrics":
		name := cmd.Payload["name"].(string)
		metrics, err := m.driver.GetMetrics(ctx, name)
		go m.nodeService.HandleCommandResult(&domainNode.CommandResult{
			CorrelationID: cmd.CorrelationID,
			Success:       err == nil,
			Payload: map[string]interface{}{
				"cpuUsagePercent":  metrics.CPUUsagePercent,
				"memoryUsageBytes": metrics.MemoryUsageBytes,
				"memoryPeakBytes":  metrics.MemoryPeakBytes,
				"diskReadBytes":    metrics.DiskReadBytes,
				"diskWriteBytes":   metrics.DiskWriteBytes,
				"networkRxBytes":   metrics.NetworkRxBytes,
				"networkTxBytes":   metrics.NetworkTxBytes,
				"processesCount":   metrics.ProcessesCount,
			},
			CompletedAt: time.Now().UTC(),
		})
	}
	return nil
}

func setupComputeServiceTest(t *testing.T) (*Service, *memory.MemoryStore, string) {
	memStore := memory.NewMemoryStore()
	authorizer := authz.NewAuthorizer(memStore.Roles())
	ca, err := pki.NewInternalCA(nil, nil)
	require.NoError(t, err)

	connMgr := appNode.NewConnectionManager()
	nodeService := appNode.NewService(memStore.Nodes(), memStore.Enrollments(), ca, connMgr, memStore.Audit(), "127.0.0.1:8443")

	// Register a simulated online node
	nodeID := "node-test-hv-01"
	nodeRecord := &domainNode.Node{
		ID:              nodeID,
		Name:            "hv-01",
		FQDN:            "127.0.0.1",
		Status:          domainNode.StatusOnline,
		MaintenanceMode: false,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}
	err = memStore.Nodes().Create(context.Background(), nodeRecord)
	require.NoError(t, err)

	driver := incus.NewSimulatedDriver()
	mockSender := &mockComputeStreamSender{driver: driver, nodeService: nodeService}
	err = nodeService.OnStreamConnected(context.Background(), nodeID, mockSender)
	require.NoError(t, err)

	svc := NewService(
		memStore.Instances(),
		memStore.Nodes(),
		nodeService,
		authorizer,
		memStore.Audit(),
	)

	return svc, memStore, nodeID
}

func TestComputeService_Create_Power_Spec_Delete_FullFlow(t *testing.T) {
	ctx := context.Background()
	svc, _, _ := setupComputeServiceTest(t)

	adminSubject := &identity.Subject{
		UserID:      "usr_admin_1",
		Roles:       []string{"superadmin"},
		Permissions: []string{"*"},
	}

	custSubject := &identity.Subject{
		UserID:      "usr_customer_1",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:create", "instance:read", "instance:power", "instance:update", "instance:delete"},
	}

	// 1. Customer creates a container instance
	inst, err := svc.CreateInstance(ctx, custSubject, CreateInstanceRequest{
		Name:             "web-server-01",
		Type:             "container",
		CPUCores:         2,
		MemoryBytes:      2 * 1024 * 1024 * 1024,
		StorageBytes:     25 * 1024 * 1024 * 1024,
		Image:            "images:ubuntu/24.04",
		StartAfterCreate: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, inst.ID)
	assert.Equal(t, "web-server-01", inst.Name)
	assert.Equal(t, domainCompute.StatusRunning, inst.Status)
	assert.NotEmpty(t, inst.IPv4Address)

	// 2. Customer reads instance
	fetched, err := svc.GetInstance(ctx, custSubject, inst.ID)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, fetched.ID)

	// 3. Customer powers down instance (Stop)
	stopped, err := svc.PowerAction(ctx, custSubject, inst.ID, "stop", false)
	require.NoError(t, err)
	assert.Equal(t, domainCompute.StatusStopped, stopped.Status)

	// 4. Customer powers up instance (Start)
	started, err := svc.PowerAction(ctx, custSubject, inst.ID, "start", false)
	require.NoError(t, err)
	assert.Equal(t, domainCompute.StatusRunning, started.Status)

	// 5. Customer fetches live telemetry metrics
	metrics, err := svc.GetInstanceMetrics(ctx, custSubject, inst.ID)
	require.NoError(t, err)
	assert.Greater(t, metrics.CPUUsagePercent, 0.0)
	assert.Greater(t, metrics.MemoryUsageBytes, int64(0))

	// 6. Customer updates spec
	resized, err := svc.UpdateSpec(ctx, custSubject, inst.ID, 4, 4*1024*1024*1024, 50*1024*1024*1024)
	require.NoError(t, err)
	assert.Equal(t, 4, resized.CPUCores)
	assert.Equal(t, int64(4*1024*1024*1024), resized.MemoryBytes)

	// 7. Tenant Isolation Check: User B attempts to access User A's instance -> Forbidden!
	otherCustSubject := &identity.Subject{
		UserID:      "usr_customer_2",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:read", "instance:power", "instance:delete"},
	}

	_, err = svc.GetInstance(ctx, otherCustSubject, inst.ID)
	assert.Error(t, err)

	_, err = svc.PowerAction(ctx, otherCustSubject, inst.ID, "stop", false)
	assert.Error(t, err)

	err = svc.DeleteInstance(ctx, otherCustSubject, inst.ID, false)
	assert.Error(t, err)

	// Superadmin CAN access and list all instances
	adminFetched, err := svc.GetInstance(ctx, adminSubject, inst.ID)
	require.NoError(t, err)
	assert.Equal(t, inst.ID, adminFetched.ID)

	allInstances, err := svc.ListInstances(ctx, adminSubject)
	require.NoError(t, err)
	assert.Len(t, allInstances, 1)

	// 8. Delete instance
	err = svc.DeleteInstance(ctx, custSubject, inst.ID, false)
	require.NoError(t, err)

	_, err = svc.GetInstance(ctx, custSubject, inst.ID)
	assert.ErrorIs(t, err, domainCompute.ErrInstanceNotFound)
}

func TestComputeService_Provisioning_With_Template_And_CloudInit(t *testing.T) {
	ctx := context.Background()
	svc, memStore, _ := setupComputeServiceTest(t)

	// Set up template service
	tmplRepo := memStore.Templates()
	imgRepo := memStore.Images()

	// Create mock template lookup service
	mockTmplSvc := &mockTemplateLookup{
		tmplRepo: tmplRepo,
		imgRepo:  imgRepo,
	}
	svc.SetTemplateService(mockTmplSvc)

	custSubject := &identity.Subject{
		UserID:      "usr_customer_cloudinit",
		Roles:       []string{"customer"},
		Permissions: []string{"instance:create", "instance:read", "instance:power", "instance:delete", "template:read"},
	}

	// 1. Provision with valid template and cloud-init
	inst, err := svc.CreateInstance(ctx, custSubject, CreateInstanceRequest{
		Name:         "cloud-init-instance",
		Type:         "container",
		CPUCores:     2,
		MemoryBytes:  1024 * 1024 * 1024,
		StorageBytes: 10 * 1024 * 1024 * 1024,
		TemplateSlug: "ubuntu-24.04",
		CloudInit: &domainTmpl.CloudInitConfig{
			Hostname: "my-cloud-vps",
			Users: []domainTmpl.CloudInitUser{
				{
					Name: "admin",
					SSHAuthorizedKeys: []string{
						"ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGo4k7E8o9t1H+g6u8B/z/d5W1j9l2k3m4n5o6p7q8r9 admin@aurora",
					},
					LockPasswd: true,
				},
			},
		},
		StartAfterCreate: true,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, inst.ID)
	assert.Equal(t, "images:ubuntu/24.04", inst.Image)
	assert.NotEmpty(t, inst.Config["user.user-data"])

	// 2. Reject insufficient memory for template
	_, err = svc.CreateInstance(ctx, custSubject, CreateInstanceRequest{
		Name:         "low-mem-instance",
		Type:         "container",
		CPUCores:     1,
		MemoryBytes:  128 * 1024 * 1024, // 128 MB < template min 512 MB
		StorageBytes: 10 * 1024 * 1024 * 1024,
		TemplateSlug: "ubuntu-24.04",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "template requires at least")

	// 3. Reject malformed cloud-init
	_, err = svc.CreateInstance(ctx, custSubject, CreateInstanceRequest{
		Name:         "bad-cloudinit-instance",
		Type:         "container",
		CPUCores:     1,
		MemoryBytes:  1024 * 1024 * 1024,
		StorageBytes: 10 * 1024 * 1024 * 1024,
		TemplateSlug: "ubuntu-24.04",
		CloudInit: &domainTmpl.CloudInitConfig{
			Users: []domainTmpl.CloudInitUser{
				{Name: "INVALID USERNAME WITH SPACES!"},
			},
		},
	})
	assert.Error(t, err)
}

type mockTemplateLookup struct {
	tmplRepo domainTmpl.TemplateRepository
	imgRepo  domainTmpl.ImageRepository
}

func (m *mockTemplateLookup) GetTemplate(ctx context.Context, sub *identity.Subject, idOrSlug string) (*domainTmpl.OSTemplate, error) {
	t, err := m.tmplRepo.GetByID(ctx, idOrSlug)
	if err == nil && t != nil {
		return t, nil
	}
	return m.tmplRepo.GetBySlug(ctx, idOrSlug)
}

func (m *mockTemplateLookup) FindCompatibleImage(ctx context.Context, templateID, architecture string, instType domainCompute.InstanceType) (*domainTmpl.ImageArtifact, error) {
	return m.imgRepo.FindCompatible(ctx, templateID, architecture, instType)
}

func (m *mockTemplateLookup) ValidateCloudInit(ctx context.Context, sub *identity.Subject, cfg *domainTmpl.CloudInitConfig) error {
	if cfg == nil {
		return nil
	}
	return cfg.Validate()
}
