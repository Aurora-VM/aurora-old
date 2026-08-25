package node

import (
	"time"

	"github.com/aurora-vm/aurora/internal/domain/identity"
)

// Status represents the operational lifecycle state of a hypervisor node.
type Status string

const (
	StatusEnrolling   Status = "enrolling"
	StatusOnline      Status = "online"
	StatusDegraded    Status = "degraded"
	StatusUnhealthy   Status = "unhealthy"
	StatusDraining    Status = "draining"
	StatusOffline     Status = "offline"
	StatusRevoked     Status = "revoked"
	StatusMaintenance Status = "maintenance"
)

// Node represents an enrolled physical/virtual hypervisor host running the Aurora Node Agent.
type Node struct {
	ID                    string                 `json:"id"`
	LocationID            string                 `json:"locationId"`
	Name                  string                 `json:"name"`
	FQDN                  string                 `json:"fqdn"`
	Status                Status                 `json:"status"`
	CertFingerprint       string                 `json:"certFingerprint"` // SHA-256 hex fingerprint
	CPUCores              int                    `json:"cpuCores"`
	MemoryBytes           int64                  `json:"memoryBytes"`
	StorageBytes          int64                  `json:"storageBytes"`
	CPUOvercommitRatio    float64                `json:"cpuOvercommitRatio"`
	MemoryOvercommitRatio float64                `json:"memoryOvercommitRatio"`
	Capabilities          map[string]interface{} `json:"capabilities"`
	MaintenanceMode       bool                   `json:"maintenanceMode"`
	DrainMode             bool                   `json:"drainMode"`
	UnhealthyReason       string                 `json:"unhealthyReason,omitempty"`
	LastHeartbeatAt       *time.Time             `json:"lastHeartbeatAt,omitempty"`
	LastStateChangeAt     *time.Time             `json:"lastStateChangeAt,omitempty"`
	CreatedAt             time.Time              `json:"createdAt"`
	UpdatedAt             time.Time              `json:"updatedAt"`
}

// IsConnectable returns true if the node is permitted to establish mTLS gateway tunnels.
func (n *Node) IsConnectable() bool {
	return n.Status != StatusRevoked
}

// IsSchedulable returns true if the node can accept new workload placements.
func (n *Node) IsSchedulable() bool {
	return (n.Status == StatusOnline || n.Status == StatusDegraded) && !n.MaintenanceMode && !n.DrainMode
}

func (n *Node) Resource() *identity.Resource {
	return &identity.Resource{
		Type:       "node",
		ID:         n.ID,
		LocationID: n.LocationID,
	}
}

// EnrollmentSecret represents a single-use or scoped token for enrolling a new node.
type EnrollmentSecret struct {
	ID              string     `json:"id"`
	LocationID      string     `json:"locationId"`
	SecretHash      string     `json:"-"` // SHA-256 hash of plaintext token; never stored in plaintext
	NodeNamePattern string     `json:"nodeNamePattern,omitempty"`
	ExpiresAt       time.Time  `json:"expiresAt"`
	UsedAt          *time.Time `json:"usedAt,omitempty"`
	UsedByNodeID    *string    `json:"usedByNodeId,omitempty"`
	CreatedBy       *string    `json:"createdBy,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

// IsValid checks whether the enrollment token is unexpired and unconsumed.
func (e *EnrollmentSecret) IsValid() bool {
	if e.UsedAt != nil {
		return false
	}
	return time.Now().Before(e.ExpiresAt)
}

// TelemetrySnapshot represents a structured point-in-time metrics sample from a node.
type TelemetrySnapshot struct {
	NodeID           string    `json:"nodeId"`
	CPUUsagePercent  float64   `json:"cpuUsagePercent"`
	MemoryUsedBytes  int64     `json:"memoryUsedBytes"`
	MemoryTotalBytes int64     `json:"memoryTotalBytes"`
	StorageUsedBytes int64     `json:"storageUsedBytes"`
	StorageTotalBytes int64    `json:"storageTotalBytes"`
	Load1m           float64   `json:"load1m"`
	Load5m           float64   `json:"load5m"`
	Load15m          float64   `json:"load15m"`
	RunningInstances int       `json:"runningInstances"`
	TotalInstances   int       `json:"totalInstances"`
	Timestamp        time.Time `json:"timestamp"`
}

// Command represents a typed control-plane instruction dispatched to a node agent.
type Command struct {
	CorrelationID string                 `json:"correlationId"`
	Type          string                 `json:"type"` // "ping", "collect_telemetry", "apply_config", "reboot", "cancel"
	Payload       map[string]interface{} `json:"payload,omitempty"`
	CreatedAt     time.Time              `json:"createdAt"`
}

// CommandResult represents the outcome of an executed typed command.
type CommandResult struct {
	CorrelationID string                 `json:"correlationId"`
	Success       bool                   `json:"success"`
	ErrorMessage  string                 `json:"errorMessage,omitempty"`
	Payload       map[string]interface{} `json:"payload,omitempty"`
	CompletedAt   time.Time              `json:"completedAt"`
}
