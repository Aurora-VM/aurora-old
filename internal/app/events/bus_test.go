package events_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	appEvents "github.com/aurora-vm/aurora/internal/app/events"
	domainEvents "github.com/aurora-vm/aurora/internal/domain/events"
	"github.com/aurora-vm/aurora/internal/infra/memory"
)

func TestEventBus_PublishAndSubscribePatterns(t *testing.T) {
	memStore := memory.NewMemoryStore()
	bus := appEvents.NewEventBus(memStore.Events(), 100, 4)
	defer bus.Close()

	var instanceEventsCount int32
	var wildcardEventsCount int32
	var tenantEventsCount int32

	var wg sync.WaitGroup
	wg.Add(3)

	bus.Subscribe("instance.*", func(ctx context.Context, ev *domainEvents.Event) error {
		atomic.AddInt32(&instanceEventsCount, 1)
		return nil
	})

	bus.Subscribe("*", func(ctx context.Context, ev *domainEvents.Event) error {
		atomic.AddInt32(&wildcardEventsCount, 1)
		return nil
	})

	bus.SubscribeTenant("tenant-alpha", "instance.*", func(ctx context.Context, ev *domainEvents.Event) error {
		atomic.AddInt32(&tenantEventsCount, 1)
		return nil
	})

	// 1. Publish matching event for tenant-alpha
	err := bus.Publish(context.Background(), &domainEvents.Event{
		TenantID:     "tenant-alpha",
		Type:         domainEvents.EventInstanceCreated,
		ResourceType: "instance",
		ResourceID:   "inst-01",
	})
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	// 2. Publish event for different tenant (tenant-beta)
	err = bus.Publish(context.Background(), &domainEvents.Event{
		TenantID:     "tenant-beta",
		Type:         domainEvents.EventInstanceStarted,
		ResourceType: "instance",
		ResourceID:   "inst-02",
	})
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	// 3. Publish non-instance event (invoice.created)
	err = bus.Publish(context.Background(), &domainEvents.Event{
		TenantID:     "tenant-alpha",
		Type:         domainEvents.EventInvoiceCreated,
		ResourceType: "invoice",
		ResourceID:   "inv-01",
	})
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}

	// Allow worker pool to process
	time.Sleep(100 * time.Millisecond)

	if got := atomic.LoadInt32(&instanceEventsCount); got != 2 {
		t.Errorf("expected 2 instance events, got %d", got)
	}
	if got := atomic.LoadInt32(&wildcardEventsCount); got != 3 {
		t.Errorf("expected 3 wildcard events, got %d", got)
	}
	if got := atomic.LoadInt32(&tenantEventsCount); got != 1 {
		t.Errorf("expected 1 tenant-alpha instance event, got %d", got)
	}

	// Verify persistence in repository
	list, total, err := memStore.Events().List(context.Background(), domainEvents.EventFilter{Limit: 10})
	if err != nil {
		t.Fatalf("failed to query event repository: %v", err)
	}
	if total != 3 || len(list) != 3 {
		t.Errorf("expected 3 persisted events, got total=%d len=%d", total, len(list))
	}
}

func TestEventBus_GracefulDrainage(t *testing.T) {
	memStore := memory.NewMemoryStore()
	bus := appEvents.NewEventBus(memStore.Events(), 500, 2)

	var processedCount int32

	bus.Subscribe("*", func(ctx context.Context, ev *domainEvents.Event) error {
		time.Sleep(2 * time.Millisecond)
		atomic.AddInt32(&processedCount, 1)
		return nil
	})

	for i := 0; i < 20; i++ {
		_ = bus.Publish(context.Background(), &domainEvents.Event{
			TenantID:     "tenant-drain",
			Type:         domainEvents.EventInstanceStopped,
			ResourceType: "instance",
			ResourceID:   "inst-drain",
		})
	}

	// Close immediately — should drain all buffered events before returning
	bus.Close()

	if got := atomic.LoadInt32(&processedCount); got != 20 {
		t.Errorf("expected 20 drained and processed events, got %d", got)
	}
}
