package scheduler

import (
	"context"
	"fmt"
	"sort"
	"strings"

	domainCompute "github.com/aurora-vm/aurora/internal/domain/compute"
	domainNode "github.com/aurora-vm/aurora/internal/domain/node"
	domainPlacement "github.com/aurora-vm/aurora/internal/domain/placement"
)

// Scheduler implements domainPlacement.Engine for intelligent multi-node workload placement.
type Scheduler struct {
	nodeRepo domainNode.NodeRepository
	instRepo domainCompute.InstanceRepository
}

// NewScheduler constructs a new Placement Scheduler.
func NewScheduler(nodeRepo domainNode.NodeRepository, instRepo domainCompute.InstanceRepository) *Scheduler {
	return &Scheduler{
		nodeRepo: nodeRepo,
		instRepo: instRepo,
	}
}

// SelectNode evaluates all cluster nodes and selects the optimal hypervisor host.
func (s *Scheduler) SelectNode(ctx context.Context, req domainPlacement.Request) (*domainPlacement.Decision, error) {
	candidates, err := s.EvaluateCandidates(ctx, req)
	if err != nil {
		return nil, err
	}

	var eligible []domainPlacement.NodeCandidate
	for _, c := range candidates {
		if c.Eligible {
			eligible = append(eligible, c)
		}
	}

	if len(eligible) == 0 {
		var reasons []string
		for _, c := range candidates {
			if c.IneligibleReason != "" {
				reasons = append(reasons, fmt.Sprintf("%s (%s)", c.Node.Name, c.IneligibleReason))
			}
		}
		return nil, fmt.Errorf("%w: %s", domainPlacement.ErrNoEligibleNodes, strings.Join(reasons, ", "))
	}

	// Sort eligible candidates by score (highest first)
	sort.Slice(eligible, func(i, j int) bool {
		return eligible[i].Score > eligible[j].Score
	})

	selected := eligible[0].Node

	return &domainPlacement.Decision{
		SelectedNode: selected,
		Candidates:   candidates,
		Reason:       fmt.Sprintf("Selected %s with capacity score %.2f", selected.Name, eligible[0].Score),
	}, nil
}

// EvaluateCandidates calculates resource metrics and eligibility for all cluster nodes.
func (s *Scheduler) EvaluateCandidates(ctx context.Context, req domainPlacement.Request) ([]domainPlacement.NodeCandidate, error) {
	nodes, err := s.nodeRepo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes for scheduling: %w", err)
	}

	instances, err := s.instRepo.ListAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instances for capacity calculation: %w", err)
	}

	// Index instances by node
	nodeAllocations := make(map[string]struct {
		cpuCores    int
		memoryBytes int64
		storageBytes int64
		count       int
	})
	for _, inst := range instances {
		if inst.Status == domainCompute.StatusDeleted || inst.Status == domainCompute.StatusPending {
			continue
		}
		alloc := nodeAllocations[inst.NodeID]
		alloc.cpuCores += inst.CPUCores
		alloc.memoryBytes += inst.MemoryBytes
		alloc.storageBytes += inst.StorageBytes
		alloc.count++
		nodeAllocations[inst.NodeID] = alloc
	}

	excludeMap := make(map[string]bool)
	for _, id := range req.ExcludeNodeIDs {
		excludeMap[id] = true
	}

	var results []domainPlacement.NodeCandidate

	for _, n := range nodes {
		candidate := domainPlacement.NodeCandidate{
			Node:     n,
			Eligible: true,
		}

		// 1. Exclude list
		if excludeMap[n.ID] {
			candidate.Eligible = false
			candidate.IneligibleReason = "node is in excluded list"
			results = append(results, candidate)
			continue
		}

		// 2. Health & Schedulability Check
		if !n.IsSchedulable() {
			candidate.Eligible = false
			if n.MaintenanceMode {
				candidate.IneligibleReason = "node is in maintenance mode"
			} else if n.DrainMode {
				candidate.IneligibleReason = "node is draining workloads"
			} else {
				candidate.IneligibleReason = fmt.Sprintf("node health status is %s", n.Status)
			}
			results = append(results, candidate)
			continue
		}

		// 3. Location match
		if req.LocationID != "" && n.LocationID != req.LocationID {
			candidate.Eligible = false
			candidate.IneligibleReason = fmt.Sprintf("location mismatch (node: %s, required: %s)", n.LocationID, req.LocationID)
			results = append(results, candidate)
			continue
		}

		// 4. Architecture match
		nodeArch := "x86_64"
		if n.Capabilities != nil {
			if a, ok := n.Capabilities["architecture"].(string); ok && a != "" {
				nodeArch = strings.ToLower(a)
			}
		}
		if req.Architecture != "" && !strings.EqualFold(req.Architecture, nodeArch) {
			candidate.Eligible = false
			candidate.IneligibleReason = fmt.Sprintf("architecture mismatch (node: %s, required: %s)", nodeArch, req.Architecture)
			results = append(results, candidate)
			continue
		}

		// 5. Virtual machine KVM support
		if req.InstanceType == domainCompute.TypeVirtualMachine && n.Capabilities != nil {
			if kvmSupported, ok := n.Capabilities["kvm_supported"].(bool); ok && !kvmSupported {
				candidate.Eligible = false
				candidate.IneligibleReason = "node does not support KVM hardware virtualization"
				results = append(results, candidate)
				continue
			}
		}

		// 6. Capacity & Overcommit calculations
		cpuRatio := n.CPUOvercommitRatio
		if cpuRatio <= 0 {
			cpuRatio = 1.0
		}
		memRatio := n.MemoryOvercommitRatio
		if memRatio <= 0 {
			memRatio = 1.0
		}

		maxCPUCores := float64(n.CPUCores) * cpuRatio
		if maxCPUCores <= 0 {
			maxCPUCores = 128 // Fallback default capacity if not reported
		}
		maxMemoryBytes := int64(float64(n.MemoryBytes) * memRatio)
		if maxMemoryBytes <= 0 {
			maxMemoryBytes = 512 * 1024 * 1024 * 1024 // 512GB fallback
		}
		maxStorageBytes := n.StorageBytes
		if maxStorageBytes <= 0 {
			maxStorageBytes = 2000 * 1024 * 1024 * 1024 // 2TB fallback
		}

		alloc := nodeAllocations[n.ID]
		candidate.CurrentInstances = alloc.count
		candidate.AvailableCPUCores = maxCPUCores - float64(alloc.cpuCores)
		candidate.AvailableMemoryMB = (maxMemoryBytes - alloc.memoryBytes) / (1024 * 1024)
		candidate.AvailableStorageGB = (maxStorageBytes - alloc.storageBytes) / (1024 * 1024 * 1024)

		if float64(req.CPUCores) > candidate.AvailableCPUCores {
			candidate.Eligible = false
			candidate.IneligibleReason = fmt.Sprintf("insufficient CPU capacity (available: %.1f, required: %d)", candidate.AvailableCPUCores, req.CPUCores)
			results = append(results, candidate)
			continue
		}

		if req.MemoryBytes > (maxMemoryBytes - alloc.memoryBytes) {
			candidate.Eligible = false
			candidate.IneligibleReason = fmt.Sprintf("insufficient memory capacity (available: %dMB, required: %dMB)", candidate.AvailableMemoryMB, req.MemoryBytes/(1024*1024))
			results = append(results, candidate)
			continue
		}

		if req.StorageBytes > (maxStorageBytes - alloc.storageBytes) {
			candidate.Eligible = false
			candidate.IneligibleReason = fmt.Sprintf("insufficient storage capacity (available: %dGB, required: %dGB)", candidate.AvailableStorageGB, req.StorageBytes/(1024*1024*1024))
			results = append(results, candidate)
			continue
		}

		// 7. Calculate Placement Score (0.0 to 100.0, higher is better)
		// Balances resource utilization to spread load across hypervisors
		cpuUtilRatio := float64(alloc.cpuCores+req.CPUCores) / maxCPUCores
		memUtilRatio := float64(alloc.memoryBytes+req.MemoryBytes) / float64(maxMemoryBytes)
		score := ((1.0 - cpuUtilRatio)*0.5 + (1.0 - memUtilRatio)*0.5) * 100.0

		// Boost preferred node if specified
		if req.PreferredNodeID == n.ID {
			score += 15.0
		}

		candidate.Score = score
		results = append(results, candidate)
	}

	return results, nil
}
