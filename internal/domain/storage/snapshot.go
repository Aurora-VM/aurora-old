package storage

import "time"

// VolumeSnapshot represents a point-in-time snapshot of a storage volume.
type VolumeSnapshot struct {
	ID        string    `json:"id"`
	VolumeID  string    `json:"volumeId"`
	Name      string    `json:"name"`
	SizeBytes int64     `json:"sizeBytes"`
	CreatedAt time.Time `json:"createdAt"`
}
