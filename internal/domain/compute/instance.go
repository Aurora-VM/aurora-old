package compute

import (
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// Type represents the guest virtualization type.
type Type string

// InstanceType is an alias for Type.
type InstanceType = Type

const (
	TypeContainer      Type = "container"
	TypeVirtualMachine Type = "virtual-machine"
)

// Status represents the operational power/lifecycle state of an instance.
type Status string

const (
	StatusPending    Status = "pending"
	StatusCreating   Status = "creating"
	StatusRunning    Status = "running"
	StatusStopped    Status = "stopped"
	StatusRestarting Status = "restarting"
	StatusFrozen     Status = "frozen"
	StatusError      Status = "error"
	StatusDeleting   Status = "deleting"
	StatusDeleted    Status = "deleted"
)

// Instance represents a provisioned container or KVM virtual machine.
type Instance struct {
	ID           string                 `json:"id"`
	UserID       string                 `json:"userId"`
	NodeID       string                 `json:"nodeId"`
	Name         string                 `json:"name"`
	Type         Type                   `json:"type"`
	Status       Status                 `json:"status"`
	CPUCores     int                    `json:"cpuCores"`
	MemoryBytes  int64                  `json:"memoryBytes"`
	StorageBytes int64                  `json:"storageBytes"`
	Image        string                 `json:"image"`
	IPv4Address  string                 `json:"ipv4Address,omitempty"`
	IPv6Address  string                 `json:"ipv6Address,omitempty"`
	Config       map[string]interface{} `json:"config,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	UpdatedAt    time.Time              `json:"updatedAt"`
}

// Resource converts an Instance into an identity.Resource for RBAC tenancy ownership checks.
func (i *Instance) Resource() *identity.Resource {
	return &identity.Resource{
		Type:    "instance",
		ID:      i.ID,
		OwnerID: i.UserID,
	}
}

// InstanceSpec describes desired specifications for creating an instance on a hypervisor.
type InstanceSpec struct {
	ID               string            `json:"id"`
	Name             string            `json:"name"`
	Type             Type              `json:"type"`
	CPUCores         int               `json:"cpuCores"`
	MemoryBytes      int64             `json:"memoryBytes"`
	StorageBytes     int64             `json:"storageBytes"`
	Image            string            `json:"image"`
	Config           map[string]string `json:"config,omitempty"`
	StartAfterCreate bool              `json:"startAfterCreate"`
}

// InstanceInfo represents live guest status reported by Incus.
type InstanceInfo struct {
	Name        string    `json:"name"`
	Status      Status    `json:"status"`
	StatusCode  int       `json:"statusCode"`
	Type        Type      `json:"type"`
	IPv4Address string    `json:"ipv4Address,omitempty"`
	IPv6Address string    `json:"ipv6Address,omitempty"`
	PID         int64     `json:"pid,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// InstanceMetrics describes point-in-time performance and resource utilization of an instance.
type InstanceMetrics struct {
	CPUUsagePercent  float64   `json:"cpuUsagePercent"`
	MemoryUsageBytes int64     `json:"memoryUsageBytes"`
	MemoryPeakBytes  int64     `json:"memoryPeakBytes"`
	DiskReadBytes    int64     `json:"diskReadBytes"`
	DiskWriteBytes   int64     `json:"diskWriteBytes"`
	NetworkRxBytes   int64     `json:"networkRxBytes"`
	NetworkTxBytes   int64     `json:"networkTxBytes"`
	ProcessesCount   int       `json:"processesCount"`
	Timestamp        time.Time `json:"timestamp"`
}
