package placement

import (
	"context"
	"errors"

	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
)

var (
	ErrNoEligibleNodes     = errors.New("no eligible hypervisor nodes match placement requirements")
	ErrNodeCapacityExceeded = errors.New("node capacity would be exceeded")
)

// Request defines specifications and requirements for scheduling an instance onto a node.
type Request struct {
	InstanceName     string                      `json:"instanceName"`
	InstanceType     domainCompute.InstanceType  `json:"instanceType"`
	CPUCores         int                         `json:"cpuCores"`
	MemoryBytes      int64                       `json:"memoryBytes"`
	StorageBytes     int64                       `json:"storageBytes"`
	Architecture     string                      `json:"architecture"` // "x86_64", "aarch64"
	LocationID       string                      `json:"locationId,omitempty"`
	PreferredNodeID  string                      `json:"preferredNodeId,omitempty"`
	ExcludeNodeIDs   []string                    `json:"excludeNodeIds,omitempty"`
	RequiredFeatures []string                    `json:"requiredFeatures,omitempty"`
}

// NodeCandidate represents an evaluated candidate node with capacity scores.
type NodeCandidate struct {
	Node               *domainNode.Node `json:"node"`
	AvailableCPUCores  float64          `json:"availableCpuCores"`
	AvailableMemoryMB  int64            `json:"availableMemoryMb"`
	AvailableStorageGB int64            `json:"availableStorageGb"`
	CurrentInstances   int              `json:"currentInstances"`
	Score              float64          `json:"score"` // Higher is better (e.g. balanced resource distribution)
	Eligible           bool             `json:"eligible"`
	IneligibleReason   string           `json:"ineligibleReason,omitempty"`
}

// Decision represents the final placement decision made by the scheduling engine.
type Decision struct {
	SelectedNode *domainNode.Node `json:"selectedNode"`
	Candidates   []NodeCandidate  `json:"candidates"`
	Reason       string           `json:"reason"`
}

// Engine defines the placement and scheduling port.
type Engine interface {
	SelectNode(ctx context.Context, req Request) (*Decision, error)
	EvaluateCandidates(ctx context.Context, req Request) ([]NodeCandidate, error)
}
