package events

import (
	"context"
	"log"
	"strings"
	"sync"
	"time"

	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/google/uuid"
)

// EventHandler is a callback invoked when a matching domain event occurs.
type EventHandler func(ctx context.Context, event *domainEvents.Event) error

type subscription struct {
	id        string
	pattern   string
	handler   EventHandler
	tenantID  string // optional tenant filter (empty for global handlers)
}

// EventBus coordinates non-blocking publishing and asynchronous worker dispatching.
type EventBus struct {
	mu           sync.RWMutex
	eventRepo    domainEvents.Repository
	subscribers  map[string]*subscription
	eventQueue   chan *domainEvents.Event
	workerCount  int
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	closed       bool
}

// NewEventBus constructs a worker-pool event bus with bounded buffer queue.
func NewEventBus(eventRepo domainEvents.Repository, bufferSize int, workerCount int) *EventBus {
	if bufferSize <= 0 {
		bufferSize = 2048
	}
	if workerCount <= 0 {
		workerCount = 8
	}

	ctx, cancel := context.WithCancel(context.Background())

	bus := &EventBus{
		eventRepo:   eventRepo,
		subscribers: make(map[string]*subscription),
		eventQueue:  make(chan *domainEvents.Event, bufferSize),
		workerCount: workerCount,
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start worker pool
	for i := 0; i < bus.workerCount; i++ {
		bus.wg.Add(1)
		go bus.worker(i)
	}

	return bus
}

// Publish enqueues a domain event for asynchronous delivery and persists it.
func (b *EventBus) Publish(ctx context.Context, event *domainEvents.Event) error {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return domainEvents.ErrEventBusClosed
	}
	b.mu.RUnlock()

	if event.ID == "" {
		event.ID = uuid.NewString()
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Version == "" {
		event.Version = "1.0"
	}

	// Always persist to durable storage
	if b.eventRepo != nil {
		if err := b.eventRepo.Store(ctx, event); err != nil {
			log.Printf("[WARN] Failed to persist domain event %s: %v", event.ID, err)
		}
	}

	// Non-blocking dispatch to queue
	select {
	case b.eventQueue <- event:
		return nil
	default:
		// Queue saturated: process in separate goroutine rather than dropping
		go func(e *domainEvents.Event) {
			b.dispatchDirect(e)
		}(event)
		return nil
	}
}

// Subscribe registers a handler for an event pattern (e.g. "instance.*", "*", "billing.invoice.created").
func (b *EventBus) Subscribe(pattern string, handler EventHandler) string {
	return b.SubscribeTenant("", pattern, handler)
}

// SubscribeTenant registers a handler scoped to a specific tenant ID.
func (b *EventBus) SubscribeTenant(tenantID string, pattern string, handler EventHandler) string {
	b.mu.Lock()
	defer b.mu.Unlock()

	subID := uuid.NewString()
	b.subscribers[subID] = &subscription{
		id:       subID,
		pattern:  pattern,
		handler:  handler,
		tenantID: tenantID,
	}
	return subID
}

// Unsubscribe removes an active subscriber.
func (b *EventBus) Unsubscribe(subID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, subID)
}

func (b *EventBus) worker(workerID int) {
	defer b.wg.Done()

	for {
		select {
		case <-b.ctx.Done():
			// Drain remaining events in queue
			for {
				select {
				case evt, ok := <-b.eventQueue:
					if !ok || evt == nil {
						return
					}
					b.dispatchDirect(evt)
				default:
					return
				}
			}
		case evt, ok := <-b.eventQueue:
			if !ok || evt == nil {
				return
			}
			b.dispatchDirect(evt)
		}
	}
}

func (b *EventBus) dispatchDirect(event *domainEvents.Event) {
	if event == nil {
		return
	}
	b.mu.RLock()
	subs := make([]*subscription, 0, len(b.subscribers))
	for _, sub := range b.subscribers {
		subs = append(subs, sub)
	}
	b.mu.RUnlock()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for _, sub := range subs {
		if sub.tenantID != "" && sub.tenantID != event.TenantID {
			continue
		}
		if !matchesPattern(sub.pattern, string(event.Type)) {
			continue
		}

		func(s *subscription) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[ERROR] Panic in event handler for %s: %v", event.Type, r)
				}
			}()
			if err := s.handler(ctx, event); err != nil {
				log.Printf("[WARN] Event handler error on %s: %v", event.Type, err)
			}
		}(sub)
	}
}

func matchesPattern(pattern, eventType string) bool {
	if pattern == "*" || pattern == eventType {
		return true
	}
	if strings.HasSuffix(pattern, ".*") {
		prefix := strings.TrimSuffix(pattern, ".*")
		if strings.HasPrefix(eventType, prefix+".") {
			return true
		}
	}
	return false
}

// Close gracefully terminates the event bus workers after draining.
func (b *EventBus) Close() {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return
	}
	b.closed = true
	b.cancel()
	close(b.eventQueue)
	b.mu.Unlock()

	b.wg.Wait()
}
