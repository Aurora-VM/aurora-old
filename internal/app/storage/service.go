package storage

import (
	"context"
	"errors"
	"fmt"
	"time"

	appNode "github.com/aurora-vm/aurora/internal/app/node"
	"github.com/aurora-vm/aurora/internal/domain/audit"
	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	"github.com/aurora-vm/aurora/internal/domain/identity"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainStorage "github.com/aurora-vm/aurora/internal/domain/storage"
)

// Service coordinates storage pools, custom volumes, attachments, and snapshots.
type Service struct {
	poolRepo     domainStorage.StoragePoolRepository
	volumeRepo   domainStorage.VolumeRepository
	snapshotRepo domainStorage.VolumeSnapshotRepository
	instRepo     domainCompute.InstanceRepository
	nodeRepo     domainNode.NodeRepository
	nodeService  *appNode.Service
	authorizer   identity.Authorizer
	auditRepo    audit.Repository
}

// NewService constructs a Storage Application Service.
func NewService(
	poolRepo domainStorage.StoragePoolRepository,
	volumeRepo domainStorage.VolumeRepository,
	snapshotRepo domainStorage.VolumeSnapshotRepository,
	instRepo domainCompute.InstanceRepository,
	nodeRepo domainNode.NodeRepository,
	nodeService *appNode.Service,
	authorizer identity.Authorizer,
	auditRepo audit.Repository,
) *Service {
	return &Service{
		poolRepo:     poolRepo,
		volumeRepo:   volumeRepo,
		snapshotRepo: snapshotRepo,
		instRepo:     instRepo,
		nodeRepo:     nodeRepo,
		nodeService:  nodeService,
		authorizer:   authorizer,
		auditRepo:    auditRepo,
	}
}

type CreateStoragePoolRequest struct {
	NodeID          string                            `json:"nodeId"`
	Name            string                            `json:"name"`
	Driver          domainStorage.DriverType          `json:"driver"`
	TotalSpaceBytes int64                             `json:"totalSpaceBytes"`
	Config          map[string]interface{}            `json:"config,omitempty"`
}

func (s *Service) CreateStoragePool(ctx context.Context, sub *identity.Subject, req CreateStoragePoolRequest) (*domainStorage.StoragePool, error) {
	if req.NodeID == "" || req.Name == "" {
		return nil, domainStorage.ErrInvalidStoragePoolSpec
	}

	if err := s.authorizer.Authorize(ctx, sub, "storage:manage", &identity.Resource{Type: "node", ID: req.NodeID}); err != nil {
		return nil, err
	}

	// Verify node exists
	node, err := s.nodeRepo.GetByID(ctx, req.NodeID)
	if err != nil {
		return nil, err
	}

	driver := req.Driver
	if driver == "" {
		driver = domainStorage.DriverDir
	}

	pool := &domainStorage.StoragePool{
		NodeID:          node.ID,
		Name:            req.Name,
		Driver:          driver,
		TotalSpaceBytes: req.TotalSpaceBytes,
		UsedSpaceBytes:  0,
		Status:          domainStorage.PoolStatusOnline,
		Config:          req.Config,
		CreatedAt:       time.Now().UTC(),
		UpdatedAt:       time.Now().UTC(),
	}

	if err := s.poolRepo.Create(ctx, pool); err != nil {
		return nil, err
	}

	s.logAudit(ctx, sub, "storage_pool:create", pool.ID, map[string]interface{}{
		"nodeId": pool.NodeID,
		"name":   pool.Name,
		"driver": pool.Driver,
	})

	return pool, nil
}

func (s *Service) ListStoragePools(ctx context.Context, sub *identity.Subject, nodeID string) ([]*domainStorage.StoragePool, error) {
	if err := s.authorizer.Authorize(ctx, sub, "storage:read", nil); err != nil {
		return nil, err
	}
	return s.poolRepo.List(ctx, nodeID)
}

func (s *Service) GetStoragePool(ctx context.Context, sub *identity.Subject, id string) (*domainStorage.StoragePool, error) {
	pool, err := s.poolRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "storage:read", pool.Resource()); err != nil {
		return nil, err
	}

	return pool, nil
}

type CreateVolumeRequest struct {
	PoolID      string                            `json:"poolId"`
	Name        string                            `json:"name"`
	SizeBytes   int64                             `json:"sizeBytes"`
	ContentType domainStorage.VolumeContentType   `json:"contentType"`
	MountPath   string                            `json:"mountPath,omitempty"`
	ReadOnly    bool                              `json:"readOnly,omitempty"`
}

func (s *Service) CreateVolume(ctx context.Context, sub *identity.Subject, req CreateVolumeRequest) (*domainStorage.Volume, error) {
	if req.PoolID == "" || req.Name == "" || req.SizeBytes <= 0 {
		return nil, domainStorage.ErrInvalidVolumeSpec
	}

	pool, err := s.poolRepo.GetByID(ctx, req.PoolID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:create", pool.Resource()); err != nil {
		return nil, err
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = domainStorage.ContentTypeFilesystem
	}

	mountPath := req.MountPath
	if mountPath == "" {
		mountPath = fmt.Sprintf("/mnt/%s", req.Name)
	}

	vol := &domainStorage.Volume{
		UserID:      sub.UserID,
		PoolID:      pool.ID,
		Name:        req.Name,
		SizeBytes:   req.SizeBytes,
		ContentType: contentType,
		MountPath:   mountPath,
		ReadOnly:    req.ReadOnly,
		Status:      domainStorage.VolumeStatusReady,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
	}

	if err := s.volumeRepo.Create(ctx, vol); err != nil {
		return nil, err
	}

	// Update pool used space
	pool.UsedSpaceBytes += req.SizeBytes
	_ = s.poolRepo.Update(ctx, pool)

	// Dispatch CreateVolumeCommand across mTLS to hypervisor node if online
	_, _ = s.nodeService.SendCommand(ctx, pool.NodeID, &domainNode.Command{
		Type: "create_volume",
		Payload: map[string]interface{}{
			"volume_id":    vol.ID,
			"pool_name":    pool.Name,
			"volume_name":  vol.Name,
			"size_bytes":   vol.SizeBytes,
			"content_type": string(vol.ContentType),
		},
	})

	s.logAudit(ctx, sub, "volume:create", vol.ID, map[string]interface{}{
		"poolId":    vol.PoolID,
		"name":      vol.Name,
		"sizeBytes": vol.SizeBytes,
	})

	return vol, nil
}

func (s *Service) ListVolumes(ctx context.Context, sub *identity.Subject, poolID, instanceID string) ([]*domainStorage.Volume, error) {
	if err := s.authorizer.Authorize(ctx, sub, "volume:read", nil); err != nil {
		return nil, err
	}

	if instanceID != "" {
		inst, err := s.instRepo.GetByID(ctx, instanceID)
		if err != nil {
			return nil, err
		}
		if err := s.authorizer.Authorize(ctx, sub, "instance:read", inst.Resource()); err != nil {
			return nil, err
		}
		return s.volumeRepo.ListByInstance(ctx, instanceID)
	}

	if poolID != "" {
		return s.volumeRepo.ListByPool(ctx, poolID)
	}

	// If superadmin or operator, can list all volumes; regular customer lists only their own
	if isGlobalAdmin(sub) {
		return s.volumeRepo.ListByUser(ctx, "")
	}

	return s.volumeRepo.ListByUser(ctx, sub.UserID)
}

func (s *Service) GetVolume(ctx context.Context, sub *identity.Subject, id string) (*domainStorage.Volume, error) {
	vol, err := s.volumeRepo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:read", vol.Resource()); err != nil {
		return nil, err
	}

	return vol, nil
}

func (s *Service) AttachVolume(ctx context.Context, sub *identity.Subject, volumeID, instanceID, mountPath string, readOnly bool) (*domainStorage.Volume, error) {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:attach", vol.Resource()); err != nil {
		return nil, err
	}

	if vol.IsAttached() {
		return nil, domainStorage.ErrVolumeAttached
	}

	inst, err := s.instRepo.GetByID(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "instance:update", inst.Resource()); err != nil {
		return nil, err
	}

	pool, err := s.poolRepo.GetByID(ctx, vol.PoolID)
	if err != nil {
		return nil, err
	}

	// Must be on the same node
	if pool.NodeID != inst.NodeID {
		return nil, errors.New("volume storage pool and instance reside on different hypervisor nodes")
	}

	if mountPath != "" {
		vol.MountPath = mountPath
	}
	vol.ReadOnly = readOnly
	vol.InstanceID = &inst.ID
	vol.Status = domainStorage.VolumeStatusAttached

	if err := s.volumeRepo.Update(ctx, vol); err != nil {
		return nil, err
	}

	// Dispatch AttachVolumeCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, inst.NodeID, &domainNode.Command{
		Type: "attach_volume",
		Payload: map[string]interface{}{
			"instance_id":   inst.ID,
			"instance_name": inst.Name,
			"pool_name":     pool.Name,
			"volume_name":   vol.Name,
			"mount_path":    vol.MountPath,
			"read_only":     vol.ReadOnly,
		},
	})

	s.logAudit(ctx, sub, "volume:attach", vol.ID, map[string]interface{}{
		"instanceId": inst.ID,
		"mountPath":  vol.MountPath,
	})

	return vol, nil
}

func (s *Service) DetachVolume(ctx context.Context, sub *identity.Subject, volumeID string) (*domainStorage.Volume, error) {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:detach", vol.Resource()); err != nil {
		return nil, err
	}

	if !vol.IsAttached() {
		return nil, domainStorage.ErrVolumeNotAttached
	}

	inst, err := s.instRepo.GetByID(ctx, *vol.InstanceID)
	if err != nil {
		return nil, err
	}

	vol.InstanceID = nil
	vol.Status = domainStorage.VolumeStatusReady

	if err := s.volumeRepo.Update(ctx, vol); err != nil {
		return nil, err
	}

	// Dispatch DetachVolumeCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, inst.NodeID, &domainNode.Command{
		Type: "detach_volume",
		Payload: map[string]interface{}{
			"instance_id":   inst.ID,
			"instance_name": inst.Name,
			"volume_name":   vol.Name,
		},
	})

	s.logAudit(ctx, sub, "volume:detach", vol.ID, map[string]interface{}{
		"instanceId": inst.ID,
	})

	return vol, nil
}

func (s *Service) ResizeVolume(ctx context.Context, sub *identity.Subject, volumeID string, newSizeBytes int64) (*domainStorage.Volume, error) {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:update", vol.Resource()); err != nil {
		return nil, err
	}

	if newSizeBytes < vol.SizeBytes {
		return nil, domainStorage.ErrVolumeResizeDownNotAllowed
	}

	pool, err := s.poolRepo.GetByID(ctx, vol.PoolID)
	if err != nil {
		return nil, err
	}

	delta := newSizeBytes - vol.SizeBytes
	vol.SizeBytes = newSizeBytes

	if err := s.volumeRepo.Update(ctx, vol); err != nil {
		return nil, err
	}

	pool.UsedSpaceBytes += delta
	_ = s.poolRepo.Update(ctx, pool)

	// Dispatch ResizeVolumeCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, pool.NodeID, &domainNode.Command{
		Type: "resize_volume",
		Payload: map[string]interface{}{
			"volume_id":      vol.ID,
			"pool_name":      pool.Name,
			"volume_name":    vol.Name,
			"new_size_bytes": newSizeBytes,
		},
	})

	s.logAudit(ctx, sub, "volume:resize", vol.ID, map[string]interface{}{
		"newSizeBytes": newSizeBytes,
	})

	return vol, nil
}

func (s *Service) DeleteVolume(ctx context.Context, sub *identity.Subject, volumeID string) error {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:delete", vol.Resource()); err != nil {
		return err
	}

	if vol.IsAttached() {
		return domainStorage.ErrVolumeAttached
	}

	pool, err := s.poolRepo.GetByID(ctx, vol.PoolID)
	if err != nil {
		return err
	}

	if err := s.volumeRepo.Delete(ctx, vol.ID); err != nil {
		return err
	}

	pool.UsedSpaceBytes -= vol.SizeBytes
	if pool.UsedSpaceBytes < 0 {
		pool.UsedSpaceBytes = 0
	}
	_ = s.poolRepo.Update(ctx, pool)

	// Dispatch DeleteVolumeCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, pool.NodeID, &domainNode.Command{
		Type: "delete_volume",
		Payload: map[string]interface{}{
			"volume_id":   vol.ID,
			"pool_name":   pool.Name,
			"volume_name": vol.Name,
		},
	})

	s.logAudit(ctx, sub, "volume:delete", vol.ID, map[string]interface{}{
		"poolId": vol.PoolID,
		"name":   vol.Name,
	})

	return nil
}

func (s *Service) CreateSnapshot(ctx context.Context, sub *identity.Subject, volumeID, snapshotName string) (*domainStorage.VolumeSnapshot, error) {
	if snapshotName == "" {
		snapshotName = fmt.Sprintf("snap-%d", time.Now().Unix())
	}

	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:snapshot", vol.Resource()); err != nil {
		return nil, err
	}

	pool, err := s.poolRepo.GetByID(ctx, vol.PoolID)
	if err != nil {
		return nil, err
	}

	snap := &domainStorage.VolumeSnapshot{
		VolumeID:  vol.ID,
		Name:      snapshotName,
		SizeBytes: vol.SizeBytes,
		CreatedAt: time.Now().UTC(),
	}

	if err := s.snapshotRepo.Create(ctx, snap); err != nil {
		return nil, err
	}

	// Dispatch CreateVolumeSnapshotCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, pool.NodeID, &domainNode.Command{
		Type: "create_volume_snapshot",
		Payload: map[string]interface{}{
			"volume_id":     vol.ID,
			"pool_name":     pool.Name,
			"volume_name":   vol.Name,
			"snapshot_name": snap.Name,
		},
	})

	s.logAudit(ctx, sub, "volume:snapshot:create", snap.ID, map[string]interface{}{
		"volumeId":     vol.ID,
		"snapshotName": snap.Name,
	})

	return snap, nil
}

func (s *Service) ListSnapshots(ctx context.Context, sub *identity.Subject, volumeID string) ([]*domainStorage.VolumeSnapshot, error) {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return nil, err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:read", vol.Resource()); err != nil {
		return nil, err
	}

	return s.snapshotRepo.ListByVolume(ctx, volumeID)
}

func (s *Service) RestoreSnapshot(ctx context.Context, sub *identity.Subject, volumeID, snapshotID string) error {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:restore", vol.Resource()); err != nil {
		return err
	}

	snap, err := s.snapshotRepo.GetByID(ctx, snapshotID)
	if err != nil {
		return err
	}

	pool, err := s.poolRepo.GetByID(ctx, vol.PoolID)
	if err != nil {
		return err
	}

	// Dispatch RestoreVolumeSnapshotCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, pool.NodeID, &domainNode.Command{
		Type: "restore_volume_snapshot",
		Payload: map[string]interface{}{
			"volume_id":     vol.ID,
			"pool_name":     pool.Name,
			"volume_name":   vol.Name,
			"snapshot_name": snap.Name,
		},
	})

	s.logAudit(ctx, sub, "volume:snapshot:restore", snap.ID, map[string]interface{}{
		"volumeId": vol.ID,
	})

	return nil
}

func (s *Service) DeleteSnapshot(ctx context.Context, sub *identity.Subject, volumeID, snapshotID string) error {
	vol, err := s.volumeRepo.GetByID(ctx, volumeID)
	if err != nil {
		return err
	}

	if err := s.authorizer.Authorize(ctx, sub, "volume:snapshot", vol.Resource()); err != nil {
		return err
	}

	snap, err := s.snapshotRepo.GetByID(ctx, snapshotID)
	if err != nil {
		return err
	}

	pool, err := s.poolRepo.GetByID(ctx, vol.PoolID)
	if err != nil {
		return err
	}

	if err := s.snapshotRepo.Delete(ctx, snapshotID); err != nil {
		return err
	}

	// Dispatch DeleteVolumeSnapshotCommand across mTLS to node
	_, _ = s.nodeService.SendCommand(ctx, pool.NodeID, &domainNode.Command{
		Type: "delete_volume_snapshot",
		Payload: map[string]interface{}{
			"volume_id":     vol.ID,
			"pool_name":     pool.Name,
			"volume_name":   vol.Name,
			"snapshot_name": snap.Name,
		},
	})

	s.logAudit(ctx, sub, "volume:snapshot:delete", snap.ID, map[string]interface{}{
		"volumeId": vol.ID,
	})

	return nil
}

func (s *Service) logAudit(ctx context.Context, sub *identity.Subject, action, resourceID string, details map[string]interface{}) {
	if s.auditRepo == nil {
		return
	}
	var actorID *string
	if sub != nil && sub.UserID != "" {
		actorID = &sub.UserID
	}
	var rID *string
	if resourceID != "" {
		rID = &resourceID
	}
	_ = s.auditRepo.Record(ctx, &audit.AuditLog{
		ActorID:    actorID,
		Action:     action,
		ResourceID: rID,
		Details:    details,
		CreatedAt:  time.Now().UTC(),
	})
}

func isGlobalAdmin(sub *identity.Subject) bool {
	if sub == nil {
		return false
	}
	for _, r := range sub.Roles {
		if r == "superadmin" || r == "operator" {
			return true
		}
	}
	return false
}
