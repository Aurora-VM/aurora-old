package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/node"
)

// MemoryNodeStore provides shared state for in-memory node and enrollment repositories.
type MemoryNodeStore struct {
	mu          sync.RWMutex
	nodes       map[string]*node.Node             // key: nodeID
	nodesByFQDN map[string]*node.Node             // key: fqdn
	nodesByFP   map[string]*node.Node             // key: cert fingerprint
	secrets     map[string]*node.EnrollmentSecret // key: secret ID
	secretsByH  map[string]*node.EnrollmentSecret // key: secret hash
}

// NewMemoryNodeStore initializes an in-memory node store.
func NewMemoryNodeStore() *MemoryNodeStore {
	return &MemoryNodeStore{
		nodes:       make(map[string]*node.Node),
		nodesByFQDN: make(map[string]*node.Node),
		nodesByFP:   make(map[string]*node.Node),
		secrets:     make(map[string]*node.EnrollmentSecret),
		secretsByH:  make(map[string]*node.EnrollmentSecret),
	}
}

func (m *MemoryNodeStore) Nodes() *MemoryNodeRepo             { return &MemoryNodeRepo{s: m} }
func (m *MemoryNodeStore) Enrollments() *MemoryEnrollmentRepo { return &MemoryEnrollmentRepo{s: m} }

// ---------------- NODE REPOSITORY ----------------

type MemoryNodeRepo struct{ s *MemoryNodeStore }

func (r *MemoryNodeRepo) Create(ctx context.Context, n *node.Node) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	if _, exists := r.s.nodesByFQDN[n.FQDN]; exists {
		return node.ErrNodeAlreadyExists
	}

	copy := *n
	r.s.nodes[n.ID] = &copy
	r.s.nodesByFQDN[n.FQDN] = &copy
	if n.CertFingerprint != "" {
		r.s.nodesByFP[n.CertFingerprint] = &copy
	}
	return nil
}

func (r *MemoryNodeRepo) GetByID(ctx context.Context, id string) (*node.Node, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return nil, node.ErrNodeNotFound
	}
	copy := *n
	return &copy, nil
}

func (r *MemoryNodeRepo) GetByFQDN(ctx context.Context, fqdn string) (*node.Node, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	n, exists := r.s.nodesByFQDN[fqdn]
	if !exists {
		return nil, node.ErrNodeNotFound
	}
	copy := *n
	return &copy, nil
}

func (r *MemoryNodeRepo) GetByCertFingerprint(ctx context.Context, fingerprint string) (*node.Node, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	n, exists := r.s.nodesByFP[fingerprint]
	if !exists {
		return nil, node.ErrNodeNotFound
	}
	copy := *n
	return &copy, nil
}

func (r *MemoryNodeRepo) UpdateStatus(ctx context.Context, id string, status node.Status) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return node.ErrNodeNotFound
	}
	n.Status = status
	n.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryNodeRepo) UpdateHealthState(ctx context.Context, id string, status node.Status, reason string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return node.ErrNodeNotFound
	}
	n.Status = status
	n.UnhealthyReason = reason
	now := time.Now().UTC()
	n.LastStateChangeAt = &now
	n.UpdatedAt = now
	return nil
}

func (r *MemoryNodeRepo) UpdateDrainMode(ctx context.Context, id string, drainMode bool) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return node.ErrNodeNotFound
	}
	n.DrainMode = drainMode
	if drainMode && n.Status == node.StatusOnline {
		n.Status = node.StatusDraining
	} else if !drainMode && n.Status == node.StatusDraining {
		n.Status = node.StatusOnline
	}
	n.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryNodeRepo) UpdateHeartbeat(ctx context.Context, id string, lastSeen time.Time, caps map[string]interface{}) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return node.ErrNodeNotFound
	}
	n.LastHeartbeatAt = &lastSeen
	if caps != nil {
		n.Capabilities = caps
	}
	n.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryNodeRepo) UpdateMaintenanceMode(ctx context.Context, id string, inMaintenance bool) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return node.ErrNodeNotFound
	}
	n.MaintenanceMode = inMaintenance
	if inMaintenance {
		n.Status = node.StatusMaintenance
	} else {
		n.Status = node.StatusOnline
	}
	n.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryNodeRepo) Revoke(ctx context.Context, id string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	n, exists := r.s.nodes[id]
	if !exists {
		return node.ErrNodeNotFound
	}
	n.Status = node.StatusRevoked
	n.UpdatedAt = time.Now().UTC()
	return nil
}

func (r *MemoryNodeRepo) List(ctx context.Context) ([]*node.Node, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var nodes []*node.Node
	for _, n := range r.s.nodes {
		copy := *n
		nodes = append(nodes, &copy)
	}
	return nodes, nil
}

// ---------------- ENROLLMENT REPOSITORY ----------------

type MemoryEnrollmentRepo struct{ s *MemoryNodeStore }

func (r *MemoryEnrollmentRepo) Create(ctx context.Context, secret *node.EnrollmentSecret) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	copy := *secret
	r.s.secrets[secret.ID] = &copy
	r.s.secretsByH[secret.SecretHash] = &copy
	return nil
}

func (r *MemoryEnrollmentRepo) GetBySecretHash(ctx context.Context, hash string) (*node.EnrollmentSecret, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	s, exists := r.s.secretsByH[hash]
	if !exists {
		return nil, node.ErrEnrollmentTokenInvalid
	}
	copy := *s
	return &copy, nil
}

func (r *MemoryEnrollmentRepo) MarkUsed(ctx context.Context, id, nodeID string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()

	s, exists := r.s.secrets[id]
	if !exists {
		return node.ErrEnrollmentTokenInvalid
	}
	if s.UsedAt != nil {
		return node.ErrEnrollmentTokenUsed
	}
	now := time.Now().UTC()
	s.UsedAt = &now
	s.UsedByNodeID = &nodeID
	return nil
}

func (r *MemoryEnrollmentRepo) ListActive(ctx context.Context) ([]*node.EnrollmentSecret, error) {
	r.s.mu.RLock()
	defer r.s.mu.RUnlock()

	var active []*node.EnrollmentSecret
	now := time.Now().UTC()
	for _, s := range r.s.secrets {
		if s.UsedAt == nil && s.ExpiresAt.After(now) {
			copy := *s
			active = append(active, &copy)
		}
	}
	return active, nil
}
