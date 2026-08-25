package memory

import (
	"context"
	"sync"
	"time"

	"github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/google/uuid"
)

// MemoryEventRepo implements events.Repository in-memory.
type MemoryEventRepo struct {
	mu     sync.RWMutex
	events map[string]*events.Event
	order  []string
}

func NewMemoryEventRepo() *MemoryEventRepo {
	return &MemoryEventRepo{
		events: make(map[string]*events.Event),
		order:  make([]string, 0),
	}
}

func (r *MemoryEventRepo) Store(ctx context.Context, event *events.Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Version == "" {
		event.Version = "1.0"
	}

	cp := *event
	r.events[event.ID] = &cp
	r.order = append(r.order, event.ID)
	return nil
}

func (r *MemoryEventRepo) GetByID(ctx context.Context, id string) (*events.Event, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	evt, exists := r.events[id]
	if !exists {
		return nil, events.ErrEventNotFound
	}
	cp := *evt
	return &cp, nil
}

func (r *MemoryEventRepo) List(ctx context.Context, filter events.EventFilter) ([]*events.Event, int64, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var matched []*events.Event
	// Traverse in reverse order (newest first)
	for i := len(r.order) - 1; i >= 0; i-- {
		id := r.order[i]
		evt := r.events[id]

		if filter.TenantID != "" && evt.TenantID != filter.TenantID {
			continue
		}
		if filter.Type != "" && evt.Type != filter.Type {
			continue
		}
		if filter.ResourceType != "" && evt.ResourceType != filter.ResourceType {
			continue
		}
		if filter.ResourceID != "" && evt.ResourceID != filter.ResourceID {
			continue
		}
		if filter.ActorID != "" && evt.ActorID != filter.ActorID {
			continue
		}
		if filter.StartTime != nil && evt.Timestamp.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && evt.Timestamp.After(*filter.EndTime) {
			continue
		}

		cp := *evt
		matched = append(matched, &cp)
	}

	total := int64(len(matched))
	if filter.Offset >= len(matched) {
		return []*events.Event{}, total, nil
	}

	end := len(matched)
	if filter.Limit > 0 && filter.Offset+filter.Limit < end {
		end = filter.Offset + filter.Limit
	}

	return matched[filter.Offset:end], total, nil
}
