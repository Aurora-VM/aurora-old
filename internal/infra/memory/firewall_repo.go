package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/network"
	"github.com/google/uuid"
)

type MemoryFirewallRepo struct {
	mu    sync.RWMutex
	rules map[string]*network.FirewallRule
}

func NewMemoryFirewallRepo() *MemoryFirewallRepo {
	return &MemoryFirewallRepo{
		rules: make(map[string]*network.FirewallRule),
	}
}

func (r *MemoryFirewallRepo) Create(ctx context.Context, rule *network.FirewallRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if rule.ID == "" {
		rule.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	rule.CreatedAt = now
	rule.UpdatedAt = now

	cp := *rule
	r.rules[rule.ID] = &cp
	return nil
}

func (r *MemoryFirewallRepo) GetByID(ctx context.Context, id string) (*network.FirewallRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	rule, exists := r.rules[id]
	if !exists {
		return nil, network.ErrFirewallRuleNotFound
	}
	cp := *rule
	return &cp, nil
}

func (r *MemoryFirewallRepo) ListByInstanceID(ctx context.Context, instanceID string) ([]*network.FirewallRule, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []*network.FirewallRule
	for _, rule := range r.rules {
		if rule.InstanceID == instanceID {
			cp := *rule
			results = append(results, &cp)
		}
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Priority < results[j].Priority
	})
	return results, nil
}

func (r *MemoryFirewallRepo) ReplaceInstanceRules(ctx context.Context, instanceID string, rules []*network.FirewallRule) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Remove existing rules for this instance
	for id, rule := range r.rules {
		if rule.InstanceID == instanceID {
			delete(r.rules, id)
		}
	}

	// Insert new rules
	now := time.Now().UTC()
	for _, rule := range rules {
		if rule.ID == "" {
			rule.ID = uuid.NewString()
		}
		rule.InstanceID = instanceID
		rule.CreatedAt = now
		rule.UpdatedAt = now
		cp := *rule
		r.rules[rule.ID] = &cp
	}
	return nil
}

func (r *MemoryFirewallRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.rules[id]; !exists {
		return network.ErrFirewallRuleNotFound
	}
	delete(r.rules, id)
	return nil
}

func (r *MemoryFirewallRepo) DeleteByInstanceID(ctx context.Context, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	for id, rule := range r.rules {
		if rule.InstanceID == instanceID {
			delete(r.rules, id)
		}
	}
	return nil
}
