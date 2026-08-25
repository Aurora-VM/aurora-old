package storage

import "context"

// StoragePoolRepository defines the persistence operations for storage pools.
type StoragePoolRepository interface {
	Create(ctx context.Context, pool *StoragePool) error
	GetByID(ctx context.Context, id string) (*StoragePool, error)
	GetByNodeAndName(ctx context.Context, nodeID, name string) (*StoragePool, error)
	List(ctx context.Context, nodeID string) ([]*StoragePool, error)
	Update(ctx context.Context, pool *StoragePool) error
	Delete(ctx context.Context, id string) error
}

// VolumeRepository defines the persistence operations for custom storage volumes.
type VolumeRepository interface {
	Create(ctx context.Context, vol *Volume) error
	GetByID(ctx context.Context, id string) (*Volume, error)
	GetByPoolAndName(ctx context.Context, poolID, name string) (*Volume, error)
	ListByUser(ctx context.Context, userID string) ([]*Volume, error)
	ListByPool(ctx context.Context, poolID string) ([]*Volume, error)
	ListByInstance(ctx context.Context, instanceID string) ([]*Volume, error)
	Update(ctx context.Context, vol *Volume) error
	Delete(ctx context.Context, id string) error
}

// VolumeSnapshotRepository defines the persistence operations for volume snapshots.
type VolumeSnapshotRepository interface {
	Create(ctx context.Context, snap *VolumeSnapshot) error
	GetByID(ctx context.Context, id string) (*VolumeSnapshot, error)
	GetByVolumeAndName(ctx context.Context, volumeID, name string) (*VolumeSnapshot, error)
	ListByVolume(ctx context.Context, volumeID string) ([]*VolumeSnapshot, error)
	Delete(ctx context.Context, id string) error
}

// StorageDriver is the hypervisor-level storage manager port.
type StorageDriver interface {
	CreateVolume(ctx context.Context, poolName, volumeName string, sizeBytes int64, contentType string) error
	ResizeVolume(ctx context.Context, poolName, volumeName string, newSizeBytes int64) error
	AttachVolume(ctx context.Context, instanceName, poolName, volumeName, mountPath string, readOnly bool) error
	DetachVolume(ctx context.Context, instanceName, volumeName string) error
	DeleteVolume(ctx context.Context, poolName, volumeName string) error
	CreateSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error
	RestoreSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error
	DeleteSnapshot(ctx context.Context, poolName, volumeName, snapshotName string) error
}
