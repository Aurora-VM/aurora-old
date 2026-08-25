package storage

import (
	"math"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

type DriverType string

const (
	DriverZFS   DriverType = "zfs"
	DriverBtrfs DriverType = "btrfs"
	DriverLVM   DriverType = "lvm"
	DriverCeph  DriverType = "ceph"
	DriverDir   DriverType = "dir"
)

type PoolStatus string

const (
	PoolStatusOnline   PoolStatus = "online"
	PoolStatusDegraded PoolStatus = "degraded"
	PoolStatusOffline  PoolStatus = "offline"
)

// StoragePool represents a backing storage driver pool on a hypervisor node.
type StoragePool struct {
	ID              string                 `json:"id"`
	NodeID          string                 `json:"nodeId"`
	Name            string                 `json:"name"`
	Driver          DriverType             `json:"driver"`
	TotalSpaceBytes int64                  `json:"totalSpaceBytes"`
	UsedSpaceBytes  int64                  `json:"usedSpaceBytes"`
	Status          PoolStatus             `json:"status"`
	Config          map[string]interface{} `json:"config,omitempty"`
	CreatedAt       time.Time              `json:"createdAt"`
	UpdatedAt       time.Time              `json:"updatedAt"`
}

func (p *StoragePool) FreeSpaceBytes() int64 {
	free := p.TotalSpaceBytes - p.UsedSpaceBytes
	if free < 0 {
		return 0
	}
	return free
}

func (p *StoragePool) UsagePercentage() float64 {
	if p.TotalSpaceBytes <= 0 {
		return 0.0
	}
	pct := (float64(p.UsedSpaceBytes) / float64(p.TotalSpaceBytes)) * 100.0
	return math.Round(pct*100) / 100
}

func (p *StoragePool) Resource() *identity.Resource {
	return &identity.Resource{
		Type:       "storage_pool",
		ID:         p.ID,
		LocationID: p.NodeID,
	}
}
