package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/webhook"
	"github.com/google/uuid"
)

// MemoryWebhookRepo implements webhook.WebhookRepository in-memory.
type MemoryWebhookRepo struct {
	mu        sync.RWMutex
	endpoints map[string]*webhook.WebhookEndpoint
	order     []string
}

func NewMemoryWebhookRepo() *MemoryWebhookRepo {
	return &MemoryWebhookRepo{
		endpoints: make(map[string]*webhook.WebhookEndpoint),
		order:     make([]string, 0),
	}
}

func (r *MemoryWebhookRepo) Create(ctx context.Context, ep *webhook.WebhookEndpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if ep.ID == "" {
		ep.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	ep.CreatedAt = now
	ep.UpdatedAt = now

	cp := *ep
	r.endpoints[ep.ID] = &cp
	r.order = append(r.order, ep.ID)
	return nil
}

func (r *MemoryWebhookRepo) GetByID(ctx context.Context, id string) (*webhook.WebhookEndpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	ep, exists := r.endpoints[id]
	if !exists {
		return nil, webhook.ErrWebhookNotFound
	}
	cp := *ep
	return &cp, nil
}

func (r *MemoryWebhookRepo) List(ctx context.Context, filter webhook.WebhookFilter) ([]*webhook.WebhookEndpoint, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*webhook.WebhookEndpoint
	for i := len(r.order) - 1; i >= 0; i-- {
		id := r.order[i]
		ep := r.endpoints[id]

		if filter.TenantID != "" && ep.TenantID != filter.TenantID {
			continue
		}
		if filter.Active != nil && ep.Active != *filter.Active {
			continue
		}

		cp := *ep
		matched = append(matched, &cp)
	}

	total := int64(len(matched))
	if filter.Offset >= len(matched) {
		return []*webhook.WebhookEndpoint{}, total, nil
	}

	end := len(matched)
	if filter.Limit > 0 && filter.Offset+filter.Limit < end {
		end = filter.Offset + filter.Limit
	}

	return matched[filter.Offset:end], total, nil
}

func (r *MemoryWebhookRepo) ListSubscribed(ctx context.Context, eventType string) ([]*webhook.WebhookEndpoint, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*webhook.WebhookEndpoint
	for _, ep := range r.endpoints {
		if ep.SubscribesTo(eventType) {
			cp := *ep
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (r *MemoryWebhookRepo) Update(ctx context.Context, ep *webhook.WebhookEndpoint) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.endpoints[ep.ID]
	if !exists {
		return webhook.ErrWebhookNotFound
	}

	ep.CreatedAt = existing.CreatedAt
	ep.UpdatedAt = time.Now().UTC()

	cp := *ep
	r.endpoints[ep.ID] = &cp
	return nil
}

func (r *MemoryWebhookRepo) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.endpoints[id]; !exists {
		return webhook.ErrWebhookNotFound
	}
	delete(r.endpoints, id)

	newOrder := make([]string, 0, len(r.order))
	for _, item := range r.order {
		if item != id {
			newOrder = append(newOrder, item)
		}
	}
	r.order = newOrder
	return nil
}

func (r *MemoryWebhookRepo) UpdateDeliveryStats(ctx context.Context, id string, lastStatus string, failureIncrement bool) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	ep, exists := r.endpoints[id]
	if !exists {
		return webhook.ErrWebhookNotFound
	}

	now := time.Now().UTC()
	ep.LastDeliveryAt = &now
	ep.LastStatus = lastStatus
	if failureIncrement {
		ep.FailureCount++
	} else {
		ep.FailureCount = 0
	}
	ep.UpdatedAt = now
	return nil
}

// MemoryDeliveryRepo implements webhook.DeliveryRepository in-memory.
type MemoryDeliveryRepo struct {
	mu         sync.RWMutex
	deliveries map[string]*webhook.WebhookDelivery
	order      []string
}

func NewMemoryDeliveryRepo() *MemoryDeliveryRepo {
	return &MemoryDeliveryRepo{
		deliveries: make(map[string]*webhook.WebhookDelivery),
		order:      make([]string, 0),
	}
}

func (r *MemoryDeliveryRepo) Create(ctx context.Context, d *webhook.WebhookDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if d.ID == "" {
		d.ID = uuid.NewString()
	}
	if d.CreatedAt.IsZero() {
		d.CreatedAt = time.Now().UTC()
	}

	cp := *d
	r.deliveries[d.ID] = &cp
	r.order = append(r.order, d.ID)
	return nil
}

func (r *MemoryDeliveryRepo) GetByID(ctx context.Context, id string) (*webhook.WebhookDelivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, exists := r.deliveries[id]
	if !exists {
		return nil, webhook.ErrDeliveryNotFound
	}
	cp := *d
	return &cp, nil
}

func (r *MemoryDeliveryRepo) List(ctx context.Context, filter webhook.DeliveryFilter) ([]*webhook.WebhookDelivery, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*webhook.WebhookDelivery
	for i := len(r.order) - 1; i >= 0; i-- {
		id := r.order[i]
		d := r.deliveries[id]

		if filter.TenantID != "" && d.TenantID != filter.TenantID {
			continue
		}
		if filter.WebhookID != "" && d.WebhookID != filter.WebhookID {
			continue
		}
		if filter.EventID != "" && d.EventID != filter.EventID {
			continue
		}
		if filter.Status != nil && d.Status != *filter.Status {
			continue
		}

		cp := *d
		matched = append(matched, &cp)
	}

	total := int64(len(matched))
	if filter.Offset >= len(matched) {
		return []*webhook.WebhookDelivery{}, total, nil
	}

	end := len(matched)
	if filter.Limit > 0 && filter.Offset+filter.Limit < end {
		end = filter.Offset + filter.Limit
	}

	return matched[filter.Offset:end], total, nil
}

func (r *MemoryDeliveryRepo) ListPendingRetries(ctx context.Context, before time.Time, limit int) ([]*webhook.WebhookDelivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*webhook.WebhookDelivery
	for _, d := range r.deliveries {
		if d.Status == webhook.DeliveryPending && d.NextRetryAt != nil && !d.NextRetryAt.After(before) {
			cp := *d
			result = append(result, &cp)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}
	return result, nil
}

func (r *MemoryDeliveryRepo) Update(ctx context.Context, d *webhook.WebhookDelivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, exists := r.deliveries[d.ID]
	if !exists {
		return webhook.ErrDeliveryNotFound
	}

	d.CreatedAt = existing.CreatedAt
	cp := *d
	r.deliveries[d.ID] = &cp
	return nil
}
