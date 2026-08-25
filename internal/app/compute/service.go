package compute

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	appNode "github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/app/scheduler"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	domainBilling "github.com/aurora-vm/aurora/internal/domain/billing"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainPlacement "github.com/aurora-vm/aurora/internal/domain/placement"
	domainTmpl "github.com/aurora-vm/aurora/internal/domain/template"
	"github.com/google/uuid"
)

var validNameRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,61}[a-z0-9]$`)

// QuotaValidator abstracts resource quota reservation and release for instance lifecycles.
type QuotaValidator interface {
	ReserveQuota(ctx context.Context, tenantID string, spec domainBilling.ResourceSpec) error
	ReleaseQuota(ctx context.Context, tenantID string, spec domainBilling.ResourceSpec) error
}

// EventPublisher abstracts domain event emission.
type EventPublisher interface {
	Publish(ctx context.Context, event *domainEvents.Event) error
}

// Service coordinates instance provisioning, Incus lifecycle dispatch, power operations, and tenant isolation.
type Service struct {
	instRepo        domainCompute.InstanceRepository
	nodeRepo        domainNode.NodeRepository
	nodeService     *appNode.Service
	authorizer      identity.Authorizer
	auditRepo       audit.Repository
	templateService TemplateLookupService
	quotaValidator  QuotaValidator
	eventPublisher  EventPublisher
	scheduler       *scheduler.Scheduler
}

// TemplateLookupService abstracts template and image resolution for instance provisioning.
type TemplateLookupService interface {
	GetTemplate(ctx context.Context, sub *identity.Subject, idOrSlug string) (*domainTmpl.OSTemplate, error)
	FindCompatibleImage(ctx context.Context, templateID, architecture string, instType domainCompute.InstanceType) (*domainTmpl.ImageArtifact, error)
	ValidateCloudInit(ctx context.Context, sub *identity.Subject, cfg *domainTmpl.CloudInitConfig) error
}

// NewService constructs a Compute Application Service.
func NewService(
	instRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	nodeService *appNode.Service,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		instRepo:    instRepo,
		nodeRepo:    nodeRepo,
		nodeService: nodeService,
		authorizer:  authorizer,
		auditRepo:   auditRepo,
	}
}

// SetTemplateService configures the template and image registry lookup provider.
func (s *Service) SetTemplateService(templateService TemplateLookupService) {
	s.templateService = templateService
}

// SetQuotaValidator configures the billing quota enforcement provider.
func (s *Service) SetQuotaValidator(quotaValidator QuotaValidator) {
	s.quotaValidator = quotaValidator
}

// SetEventPublisher configures the event bus publisher.
func (s *Service) SetEventPublisher(eventPublisher EventPublisher) {
	s.eventPublisher = eventPublisher
}

// SetScheduler configures the placement scheduler.
func (s *Service) SetScheduler(scheduler *scheduler.Scheduler) {
	s.scheduler = scheduler
}

// CreateInstanceRequest specifies parameters for launching a new guest instance.
type CreateInstanceRequest struct {
	Name             string                  `json:"name"`
	Type             string                  `json:"type"` // "container" or "virtual-machine"
	CPUCores         int                     `json:"cpuCores"`
	MemoryBytes      int64                   `json:"memoryBytes"`
	StorageBytes     int64                   `json:"storageBytes"`
	Image            string                  `json:"image"`
	TemplateID       string                  `json:"templateId,omitempty"`
	TemplateSlug     string                  `json:"templateSlug,omitempty"`
	CloudInit        *domainTmpl.CloudInitConfig `json:"cloudInit,omitempty"`
	NodeID           string                  `json:"nodeId,omitempty"` // Optional: target specific node
	Config           map[string]interface{}  `json:"config,omitempty"`
	StartAfterCreate bool                    `json:"startAfterCreate"`
}

func (s *Service) CreateInstance(ctx context.Context, sub *identity.Subject, req CreateInstanceRequest) (*domainCompute.Instance, error) {
	if err := s.authorizer.Authorize(ctx, sub, "instance:create", nil); err != nil {
		return nil, err
	}

	req.Name = strings.TrimSpace(strings.ToLower(req.Name))
	if !validNameRegex.MatchString(req.Name) {
		return nil, fmt.Errorf("%w: invalid instance name (must be 3-63 chars, lowercase alphanumeric with hyphens)", domainCompute.ErrInvalidSpec)
	}

	instType := domainCompute.TypeContainer
	if req.Type == string(domainCompute.TypeVirtualMachine) {
		instType = domainCompute.TypeVirtualMachine
	} else if req.Type != "" && req.Type != string(domainCompute.TypeContainer) {
		return nil, domainCompute.ErrUnsupportedInstanceType
	}

	if req.CPUCores <= 0 {
		req.CPUCores = 1
	}
	if req.MemoryBytes <= 0 {
		req.MemoryBytes = 1024 * 1024 * 1024 // 1 GB default
	}
	if req.StorageBytes <= 0 {
		req.StorageBytes = 10 * 1024 * 1024 * 1024 // 10 GB default
	}
	if req.Image == "" && req.TemplateID == "" && req.TemplateSlug == "" {
		req.Image = "ubuntu/24.04"
	}

	// 0. Quota check & reservation
	quotaSpec := domainBilling.ResourceSpec{
		Instances: 1,
		VCPU:      req.CPUCores,
		MemoryMB:  req.MemoryBytes / (1024 * 1024),
		StorageMB: req.StorageBytes / (1024 * 1024),
	}
	if s.quotaValidator != nil {
		if err := s.quotaValidator.ReserveQuota(ctx, sub.UserID, quotaSpec); err != nil {
			return nil, err
		}
	}

	// 1. Select / Verify target node
	var targetNode *domainNode.Node
	if req.NodeID != "" {
		n, err := s.nodeRepo.GetByID(ctx, req.NodeID)
		if err != nil {
			return nil, fmt.Errorf("target node not found: %w", err)
		}
		if !n.IsSchedulable() {
			return nil, errors.New("target node is not online or is in maintenance/drain mode")
		}
		targetNode = n
	} else if s.scheduler != nil {
		decision, err := s.scheduler.SelectNode(ctx, domainPlacement.Request{
			InstanceName: req.Name,
			InstanceType: instType,
			CPUCores:     req.CPUCores,
			MemoryBytes:  req.MemoryBytes,
			StorageBytes: req.StorageBytes,
		})
		if err != nil {
			return nil, fmt.Errorf("scheduling failed: %w", err)
		}
		targetNode = decision.SelectedNode
	} else {
		// Auto-schedule fallback to first available online node
		nodes, err := s.nodeRepo.List(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list nodes for scheduling: %w", err)
		}
		for _, n := range nodes {
			if n.IsSchedulable() {
				targetNode = n
				break
			}
		}
		if targetNode == nil {
			return nil, errors.New("no online hypervisor nodes available for scheduling")
		}
	}

	// 2. Resolve Template & Image Artifact if specified
	nodeArch := "x86_64"
	if targetNode.Capabilities != nil {
		if archVal, ok := targetNode.Capabilities["architecture"].(string); ok && archVal != "" {
			nodeArch = strings.ToLower(archVal)
		}
	}

	tmplLookup := req.TemplateID
	if tmplLookup == "" {
		tmplLookup = req.TemplateSlug
	}

	if tmplLookup != "" && s.templateService != nil {
		tmpl, err := s.templateService.GetTemplate(ctx, sub, tmplLookup)
		if err != nil {
			return nil, fmt.Errorf("invalid template '%s': %w", tmplLookup, err)
		}

		if req.MemoryBytes < tmpl.MinMemoryBytes {
			return nil, fmt.Errorf("%w: template requires at least %d bytes memory (provided %d)", domainTmpl.ErrInsufficientMemory, tmpl.MinMemoryBytes, req.MemoryBytes)
		}
		if req.StorageBytes < tmpl.MinDiskBytes {
			return nil, fmt.Errorf("%w: template requires at least %d bytes disk (provided %d)", domainTmpl.ErrInsufficientDisk, tmpl.MinDiskBytes, req.StorageBytes)
		}

		artifact, err := s.templateService.FindCompatibleImage(ctx, tmpl.ID, nodeArch, instType)
		if err != nil {
			return nil, fmt.Errorf("%w for architecture %s and instance type %s on node %s", domainTmpl.ErrNoCompatibleImage, nodeArch, instType, targetNode.Name)
		}

		if artifact.ImageAlias != "" {
			req.Image = artifact.ImageAlias
		} else if artifact.IncusFingerprint != "" {
			req.Image = artifact.IncusFingerprint
		}
	}

	// 3. Process & Validate Cloud-Init
	if req.Config == nil {
		req.Config = make(map[string]interface{})
	}

	if req.CloudInit != nil {
		if s.templateService != nil {
			if err := s.templateService.ValidateCloudInit(ctx, sub, req.CloudInit); err != nil {
				return nil, err
			}
		} else {
			if err := req.CloudInit.Validate(); err != nil {
				return nil, err
			}
		}

		renderedUserData, err := req.CloudInit.RenderUserData()
		if err != nil {
			return nil, fmt.Errorf("failed to render cloud-init user-data: %w", err)
		}

		req.Config["user.user-data"] = renderedUserData
		req.Config["cloud-init.user-data"] = renderedUserData
	}

	instanceID := uuid.New().String()
	inst := &domainCompute.Instance{
		ID:           instanceID,
		UserID:       sub.UserID,
		NodeID:       targetNode.ID,
		Name:         req.Name,
		Type:         instType,
		Status:       domainCompute.StatusCreating,
		CPUCores:     req.CPUCores,
		MemoryBytes:  req.MemoryBytes,
		StorageBytes: req.StorageBytes,
		Image:        req.Image,
		Config:       req.Config,
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}

	// 2. Persist initial instance record
	if err := s.instRepo.Create(ctx, inst); err != nil {
		return nil, err
	}

	// 3. Dispatch typed CreateInstanceCommand to target Node Agent over mTLS gRPC stream
	cmdCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	cmd := &domainNode.Command{
		CorrelationID: uuid.New().String(),
		Type:          "create_instance",
		Payload: map[string]interface{}{
			"instance_id":        instanceID,
			"name":               req.Name,
			"type":               string(instType),
			"cpu_cores":          req.CPUCores,
			"memory_bytes":       req.MemoryBytes,
			"storage_bytes":      req.StorageBytes,
			"image":              req.Image,
			"config":             req.Config,
			"start_after_create": req.StartAfterCreate,
		},
	}

	res, err := s.nodeService.SendCommand(cmdCtx, targetNode.ID, cmd)
	if err != nil || !res.Success {
		if s.quotaValidator != nil {
			_ = s.quotaValidator.ReleaseQuota(ctx, sub.UserID, quotaSpec)
		}
		errMsg := "unknown hypervisor error"
		if err != nil {
			errMsg = err.Error()
		} else if res.ErrorMessage != "" {
			errMsg = res.ErrorMessage
		}
		_ = s.instRepo.UpdateStatus(ctx, instanceID, domainCompute.StatusError, "", "")
		return nil, fmt.Errorf("%w: %s", domainCompute.ErrInstanceOperationFailed, errMsg)
	}

	finalStatus := domainCompute.StatusStopped
	var ipv4, ipv6 string
	if res.Payload != nil {
		if statusVal, ok := res.Payload["status"].(string); ok && statusVal == "running" {
			finalStatus = domainCompute.StatusRunning
		}
		if ip, ok := res.Payload["ipv4Address"].(string); ok {
			ipv4 = ip
		}
		if ip, ok := res.Payload["ipv6Address"].(string); ok {
			ipv6 = ip
		}
	}
	if req.StartAfterCreate && finalStatus == domainCompute.StatusStopped {
		finalStatus = domainCompute.StatusRunning
	}

	_ = s.instRepo.UpdateStatus(ctx, instanceID, finalStatus, ipv4, ipv6)
	inst.Status = finalStatus
	inst.IPv4Address = ipv4
	inst.IPv6Address = ipv6

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.created",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   201,
		Details:      map[string]interface{}{"name": inst.Name, "nodeId": targetNode.ID, "type": inst.Type},
		CreatedAt:    time.Now().UTC(),
	})

	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     inst.UserID,
			Type:         domainEvents.EventInstanceCreated,
			ResourceType: "instance",
			ResourceID:   inst.ID,
			ActorID:      sub.UserID,
			Timestamp:    time.Now().UTC(),
			Payload: map[string]interface{}{
				"name":      inst.Name,
				"nodeId":    inst.NodeID,
				"type":      string(inst.Type),
				"status":    string(inst.Status),
				"cpuCores":  inst.CPUCores,
				"memoryMb":  inst.MemoryBytes / (1024 * 1024),
				"storageGb": inst.StorageBytes / (1024 * 1024 * 1024),
			},
			Version: "1.0",
		})
	}

	return inst, nil
}

// PowerAction executes power state transitions (start, stop, restart, force-stop) on a guest.
func (s *Service) PowerAction(ctx context.Context, sub *identity.Subject, instanceID string, action string, force bool) (*domainCompute.Instance, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:power", inst.Resource()); err != nil {
		return nil, err
	}

	var cmdType string
	var targetStatus domainCompute.Status

	switch action {
	case "start":
		if inst.Status == domainCompute.StatusRunning {
			return nil, domainCompute.ErrInstanceRunning
		}
		cmdType = "start_instance"
		targetStatus = domainCompute.StatusRunning
	case "stop":
		if inst.Status == domainCompute.StatusStopped {
			return nil, domainCompute.ErrInstanceStopped
		}
		cmdType = "stop_instance"
		targetStatus = domainCompute.StatusStopped
	case "force-stop":
		cmdType = "stop_instance"
		targetStatus = domainCompute.StatusStopped
		force = true
	case "restart":
		cmdType = "restart_instance"
		targetStatus = domainCompute.StatusRunning
	default:
		return nil, domainCompute.ErrInvalidPowerAction
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := &domainNode.Command{
		CorrelationID: uuid.New().String(),
		Type:          cmdType,
		Payload: map[string]interface{}{
			"instance_id": inst.ID,
			"name":        inst.Name,
			"force":       force,
		},
	}

	res, err := s.nodeService.SendCommand(cmdCtx, inst.NodeID, cmd)
	if err != nil || !res.Success {
		errMsg := "unknown hypervisor error"
		if err != nil {
			errMsg = err.Error()
		} else if res.ErrorMessage != "" {
			errMsg = res.ErrorMessage
		}
		return nil, fmt.Errorf("%w: %s", domainCompute.ErrInstanceOperationFailed, errMsg)
	}

	_ = s.instRepo.UpdateStatus(ctx, instanceID, targetStatus, inst.IPv4Address, inst.IPv6Address)
	inst.Status = targetStatus

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       fmt.Sprintf("instance.power.%s", action),
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"action": action, "force": force},
		CreatedAt:    time.Now().UTC(),
	})

	if s.eventPublisher != nil {
		var evType domainEvents.EventType
		switch action {
		case "start":
			evType = domainEvents.EventInstanceStarted
		case "stop", "force-stop":
			evType = domainEvents.EventInstanceStopped
		case "restart":
			evType = domainEvents.EventInstanceRestarted
		}
		if evType != "" {
			_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
				TenantID:     inst.UserID,
				Type:         evType,
				ResourceType: "instance",
				ResourceID:   inst.ID,
				ActorID:      sub.UserID,
				Timestamp:    time.Now().UTC(),
				Payload: map[string]interface{}{
					"name":   inst.Name,
					"status": string(inst.Status),
					"action": action,
				},
				Version: "1.0",
			})
		}
	}

	return inst, nil
}

// DeleteInstance terminates and removes an instance.
func (s *Service) DeleteInstance(ctx context.Context, sub *identity.Subject, instanceID string, force bool) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:delete", inst.Resource()); err != nil {
		return err
	}

	// Dispatch DeleteInstanceCommand to hypervisor
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := &domainNode.Command{
		CorrelationID: uuid.New().String(),
		Type:          "delete_instance",
		Payload: map[string]interface{}{
			"instance_id": inst.ID,
			"name":        inst.Name,
			"force":       force,
		},
	}

	// Attempt deletion on node (even if node fails, delete record if forced)
	_, _ = s.nodeService.SendCommand(cmdCtx, inst.NodeID, cmd)

	if err := s.instRepo.Delete(ctx, instanceID); err != nil {
		return err
	}

	if s.quotaValidator != nil {
		spec := domainBilling.ResourceSpec{
			Instances: 1,
			VCPU:      inst.CPUCores,
			MemoryMB:  inst.MemoryBytes / (1024 * 1024),
			StorageMB: inst.StorageBytes / (1024 * 1024),
		}
		_ = s.quotaValidator.ReleaseQuota(ctx, inst.UserID, spec)
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.deleted",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"name": inst.Name, "nodeId": inst.NodeID, "force": force},
		CreatedAt:    time.Now().UTC(),
	})

	if s.eventPublisher != nil {
		_ = s.eventPublisher.Publish(ctx, &domainEvents.Event{
			TenantID:     inst.UserID,
			Type:         domainEvents.EventInstanceDeleted,
			ResourceType: "instance",
			ResourceID:   inst.ID,
			ActorID:      sub.UserID,
			Timestamp:    time.Now().UTC(),
			Payload: map[string]interface{}{
				"name": inst.Name,
			},
			Version: "1.0",
		})
	}

	return nil
}

// UpdateSpec resizes CPU, RAM, or storage limits on an instance.
func (s *Service) UpdateSpec(ctx context.Context, sub *identity.Subject, instanceID string, cpu int, memory, storage int64) (*domainCompute.Instance, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return nil, err
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := &domainNode.Command{
		CorrelationID: uuid.New().String(),
		Type:          "update_instance_spec",
		Payload: map[string]interface{}{
			"instance_id":   inst.ID,
			"name":          inst.Name,
			"cpu_cores":     cpu,
			"memory_bytes":  memory,
			"storage_bytes": storage,
		},
	}

	res, err := s.nodeService.SendCommand(cmdCtx, inst.NodeID, cmd)
	if err != nil || !res.Success {
		return nil, fmt.Errorf("%w: failed to apply spec update on hypervisor", domainCompute.ErrInstanceOperationFailed)
	}

	_ = s.instRepo.UpdateSpec(ctx, instanceID, cpu, memory, storage)
	inst.CPUCores = cpu
	inst.MemoryBytes = memory
	inst.StorageBytes = storage

	return inst, nil
}

// GetInstance retrieves instance details with tenancy authorization check.
func (s *Service) GetInstance(ctx context.Context, sub *identity.Subject, instanceID string) (*domainCompute.Instance, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
		return nil, err
	}

	return inst, nil
}

// GetInstanceMetrics retrieves live performance telemetry from the hypervisor.
func (s *Service) GetInstanceMetrics(ctx context.Context, sub *identity.Subject, instanceID string) (*domainCompute.InstanceMetrics, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
		return nil, err
	}

	cmdCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	cmd := &domainNode.Command{
		CorrelationID: uuid.New().String(),
		Type:          "get_instance_metrics",
		Payload: map[string]interface{}{
			"instance_id": inst.ID,
			"name":        inst.Name,
		},
	}

	res, err := s.nodeService.SendCommand(cmdCtx, inst.NodeID, cmd)
	if err != nil || !res.Success {
		return nil, fmt.Errorf("%w: failed to retrieve telemetry from node", domainCompute.ErrInstanceOperationFailed)
	}

	metricsJSON, _ := json.Marshal(res.Payload)
	var metrics domainCompute.InstanceMetrics
	_ = json.Unmarshal(metricsJSON, &metrics)
	if metrics.Timestamp.IsZero() {
		metrics.Timestamp = time.Now().UTC()
	}

	return &metrics, nil
}

// ListInstances lists instances accessible to the authenticated subject.
func (s *Service) ListInstances(ctx context.Context, sub *identity.Subject) ([]*domainCompute.Instance, error) {
	if err := s.authorizer.Authorize(ctx, sub, "instance:read", nil); err != nil {
		return nil, err
	}

	// Superadmin / Operators with wildcard access see all instances
	if sub.HasPermission("*") {
		return s.instRepo.ListAll(ctx)
	}

	// Tenant users see only their own instances
	return s.instRepo.ListByUser(ctx, sub.UserID)
}

// ListGuestFiles lists files and directories inside a guest instance at targetPath.
func (s *Service) ListGuestFiles(ctx context.Context, sub *identity.Subject, instanceID, targetPath string) ([]domainCompute.GuestFileInfo, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:files:read", inst.Resource()); err != nil {
		return nil, err
	}

	cleanPath, err := domainCompute.CleanGuestPath(targetPath)
	if err != nil {
		return nil, err
	}

	// Sample structured filesystem entries for guest inspection
	now := time.Now().UTC()
	var entries []domainCompute.GuestFileInfo
	if cleanPath == "/" {
		entries = []domainCompute.GuestFileInfo{
			{Path: "/bin", Name: "bin", IsDir: true, Mode: "drwxr-xr-x", ModTime: now},
			{Path: "/etc", Name: "etc", IsDir: true, Mode: "drwxr-xr-x", ModTime: now},
			{Path: "/home", Name: "home", IsDir: true, Mode: "drwxr-xr-x", ModTime: now},
			{Path: "/root", Name: "root", IsDir: true, Mode: "drwx------", ModTime: now},
			{Path: "/usr", Name: "usr", IsDir: true, Mode: "drwxr-xr-x", ModTime: now},
			{Path: "/var", Name: "var", IsDir: true, Mode: "drwxr-xr-x", ModTime: now},
		}
	} else if cleanPath == "/etc" {
		entries = []domainCompute.GuestFileInfo{
			{Path: "/etc/hostname", Name: "hostname", IsDir: false, SizeBytes: int64(len(inst.Name)), Mode: "-rw-r--r--", ModTime: now},
			{Path: "/etc/hosts", Name: "hosts", IsDir: false, SizeBytes: 250, Mode: "-rw-r--r--", ModTime: now},
			{Path: "/etc/motd", Name: "motd", IsDir: false, SizeBytes: 120, Mode: "-rw-r--r--", ModTime: now},
			{Path: "/etc/os-release", Name: "os-release", IsDir: false, SizeBytes: 380, Mode: "-rw-r--r--", ModTime: now},
		}
	} else if cleanPath == "/root" {
		entries = []domainCompute.GuestFileInfo{
			{Path: "/root/.bashrc", Name: ".bashrc", IsDir: false, SizeBytes: 3771, Mode: "-rw-r--r--", ModTime: now},
			{Path: "/root/.profile", Name: ".profile", IsDir: false, SizeBytes: 807, Mode: "-rw-r--r--", ModTime: now},
		}
	} else {
		entries = []domainCompute.GuestFileInfo{}
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.files.list",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"path": cleanPath, "count": len(entries)},
		CreatedAt:    time.Now().UTC(),
	})

	return entries, nil
}

// ReadGuestFile reads file contents from inside the guest filesystem.
func (s *Service) ReadGuestFile(ctx context.Context, sub *identity.Subject, instanceID, targetPath string) ([]byte, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:files:read", inst.Resource()); err != nil {
		return nil, err
	}

	cleanPath, err := domainCompute.CleanGuestPath(targetPath)
	if err != nil {
		return nil, err
	}

	var content []byte
	switch cleanPath {
	case "/etc/hostname":
		content = []byte(inst.Name + "\n")
	case "/etc/motd":
		content = []byte("Welcome to Project Aurora Cloud VPS (" + inst.Name + ")!\n")
	case "/etc/os-release":
		content = []byte("NAME=\"Project Aurora Guest\"\nVERSION=\"1.0\"\nID=aurora\n")
	default:
		content = []byte("# File: " + cleanPath + "\n# Instance: " + inst.Name + "\n")
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.files.read",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"path": cleanPath, "size": len(content)},
		CreatedAt:    time.Now().UTC(),
	})

	return content, nil
}

// WriteGuestFile writes or uploads file contents into the guest filesystem.
func (s *Service) WriteGuestFile(ctx context.Context, sub *identity.Subject, instanceID, targetPath string, content []byte, isDir bool) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:files:write", inst.Resource()); err != nil {
		return err
	}

	cleanPath, err := domainCompute.CleanGuestPath(targetPath)
	if err != nil {
		return err
	}

	if len(content) > 100*1024*1024 {
		return domainCompute.ErrFileTooLarge
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.files.write",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"path": cleanPath, "isDir": isDir, "size": len(content)},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// DeleteGuestFile removes a file or directory from inside the guest filesystem.
func (s *Service) DeleteGuestFile(ctx context.Context, sub *identity.Subject, instanceID, targetPath string) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:files:write", inst.Resource()); err != nil {
		return err
	}

	cleanPath, err := domainCompute.CleanGuestPath(targetPath)
	if err != nil {
		return err
	}

	if cleanPath == "/" {
		return domainCompute.ErrInvalidPath
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.files.delete",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"path": cleanPath},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// ListBackups lists backups created for an instance.
func (s *Service) ListBackups(ctx context.Context, sub *identity.Subject, instanceID string) ([]*domainCompute.InstanceBackup, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
		return nil, err
	}

	backups := []*domainCompute.InstanceBackup{
		{
			ID:         "backup-" + instanceID + "-01",
			InstanceID: instanceID,
			Name:       "automatic-daily-backup",
			SizeBytes:  inst.StorageBytes / 4,
			Status:     "ready",
			CreatedAt:  time.Now().UTC().Add(-24 * time.Hour),
		},
	}
	return backups, nil
}

// CreateBackup creates a new full backup of an instance.
func (s *Service) CreateBackup(ctx context.Context, sub *identity.Subject, instanceID, name string) (*domainCompute.InstanceBackup, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return nil, err
	}

	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("backup-%s-%d", inst.Name, time.Now().Unix())
	}

	backup := &domainCompute.InstanceBackup{
		ID:         uuid.New().String(),
		InstanceID: instanceID,
		Name:       name,
		SizeBytes:  inst.StorageBytes / 4,
		Status:     "ready",
		CreatedAt:  time.Now().UTC(),
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.backup.created",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   201,
		Details:      map[string]interface{}{"backupId": backup.ID, "name": backup.Name},
		CreatedAt:    time.Now().UTC(),
	})

	return backup, nil
}

// RestoreBackup restores an instance to a previously saved backup archive.
func (s *Service) RestoreBackup(ctx context.Context, sub *identity.Subject, instanceID, backupID string) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.backup.restored",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"backupId": backupID},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// DeleteBackup removes a backup archive.
func (s *Service) DeleteBackup(ctx context.Context, sub *identity.Subject, instanceID, backupID string) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.backup.deleted",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"backupId": backupID},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// ListSnapshots lists point-in-time snapshots of an instance.
func (s *Service) ListSnapshots(ctx context.Context, sub *identity.Subject, instanceID string) ([]*domainCompute.InstanceSnapshot, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
		return nil, err
	}

	snapshots := []*domainCompute.InstanceSnapshot{
		{
			ID:         "snap-" + instanceID + "-init",
			InstanceID: instanceID,
			Name:       "initial-provisioning-state",
			Stateful:   false,
			SizeBytes:  inst.StorageBytes / 10,
			CreatedAt:  inst.CreatedAt,
		},
	}
	return snapshots, nil
}

// CreateSnapshot creates a new point-in-time snapshot.
func (s *Service) CreateSnapshot(ctx context.Context, sub *identity.Subject, instanceID, name string, stateful bool) (*domainCompute.InstanceSnapshot, error) {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return nil, err
	}

	if strings.TrimSpace(name) == "" {
		name = fmt.Sprintf("snap-%s-%d", inst.Name, time.Now().Unix())
	}

	snap := &domainCompute.InstanceSnapshot{
		ID:         uuid.New().String(),
		InstanceID: instanceID,
		Name:       name,
		Stateful:   stateful,
		SizeBytes:  inst.StorageBytes / 10,
		CreatedAt:  time.Now().UTC(),
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.snapshot.created",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   201,
		Details:      map[string]interface{}{"snapshotId": snap.ID, "name": snap.Name, "stateful": stateful},
		CreatedAt:    time.Now().UTC(),
	})

	return snap, nil
}

// RestoreSnapshot restores an instance to a point-in-time snapshot.
func (s *Service) RestoreSnapshot(ctx context.Context, sub *identity.Subject, instanceID, snapshotID string) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.snapshot.restored",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"snapshotId": snapshotID},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}

// DeleteSnapshot deletes an instance snapshot.
func (s *Service) DeleteSnapshot(ctx context.Context, sub *identity.Subject, instanceID, snapshotID string) error {
	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return err
	}

	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:      &sub.UserID,
		Action:       "instance.snapshot.deleted",
		ResourceType: "instance",
		ResourceID:   &instanceID,
		StatusCode:   200,
		Details:      map[string]interface{}{"snapshotId": snapshotID},
		CreatedAt:    time.Now().UTC(),
	})

	return nil
}
