package storage

import "errors"

var (
	ErrStoragePoolNotFound         = errors.New("storage pool not found")
	ErrStoragePoolAlreadyExists    = errors.New("storage pool with this name already exists on node")
	ErrStoragePoolFull             = errors.New("storage pool has insufficient space")
	ErrVolumeNotFound              = errors.New("volume not found")
	ErrVolumeAlreadyExists         = errors.New("volume with this name already exists in pool")
	ErrVolumeAttached              = errors.New("volume is currently attached to an instance")
	ErrVolumeNotAttached           = errors.New("volume is not attached to an instance")
	ErrVolumeInUse                 = errors.New("volume is in use")
	ErrVolumeResizeDownNotAllowed  = errors.New("shrinking volume size is not supported")
	ErrVolumeSnapshotNotFound      = errors.New("volume snapshot not found")
	ErrVolumeSnapshotAlreadyExists = errors.New("volume snapshot with this name already exists")
	ErrInvalidVolumeSpec           = errors.New("invalid volume specification")
	ErrInvalidStoragePoolSpec      = errors.New("invalid storage pool specification")
	ErrUnsupportedDriver           = errors.New("unsupported storage driver")
)
