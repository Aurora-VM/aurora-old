package storage

import (
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

type VolumeContentType string

const (
	ContentTypeFilesystem VolumeContentType = "filesystem"
	ContentTypeBlock      VolumeContentType = "block"
)

type VolumeStatus string

const (
	VolumeStatusCreating VolumeStatus = "creating"
	VolumeStatusReady    VolumeStatus = "ready"
	VolumeStatusAttached VolumeStatus = "attached"
	VolumeStatusDeleting VolumeStatus = "deleting"
)

// Volume represents a custom persistent storage volume.
type Volume struct {
	ID          string            `json:"id"`
	UserID      string            `json:"userId"`
	PoolID      string            `json:"poolId"`
	InstanceID  *string           `json:"instanceId,omitempty"`
	Name        string            `json:"name"`
	SizeBytes   int64             `json:"sizeBytes"`
	ContentType VolumeContentType `json:"contentType"`
	MountPath   string            `json:"mountPath"`
	ReadOnly    bool              `json:"readOnly"`
	Status      VolumeStatus      `json:"status"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

func (v *Volume) IsAttached() bool {
	return v.InstanceID != nil && *v.InstanceID != ""
}

func (v *Volume) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "volume",
		ID:      v.ID,
		OwnerID: v.UserID,
	}
}
