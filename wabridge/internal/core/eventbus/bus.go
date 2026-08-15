// Package eventbus implements the core's EventBus (Observer/Mediator).
package eventbus

import (
	"context"
	"sync"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// InMemoryBus is a synchronous, thread-safe EventBus. Simple by design (KISS):
// it delivers each event to handlers in subscription order. A handler that
// fails is logged to the Logger and does NOT bring down the others (failure isolation).
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]ports.EventHandler
	log      ports.Logger
}

// Ensures at compile time that InMemoryBus satisfies the port.
var _ ports.EventBus = (*InMemoryBus)(nil)

// New creates an EventBus. The logger is required (fail-fast).
func New(log ports.Logger) *InMemoryBus {
	if log == nil {
		panic("eventbus: logger is required")
	}
	return &InMemoryBus{handlers: make(map[string][]ports.EventHandler), log: log}
}

// Subscribe registers a handler for an event type (fail-fast on invalid args).
func (b *InMemoryBus) Subscribe(eventName string, handler ports.EventHandler) {
	if eventName == "" || handler == nil {
		panic("eventbus: subscribe requires eventName and handler")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], handler)
}

// Publish delivers the event to all handlers subscribed to its name.
func (b *InMemoryBus) Publish(ctx context.Context, evt domain.Event) {
	if evt == nil {
		return
	}
	b.mu.RLock()
	subscribers := b.handlers[evt.EventName()]
	b.mu.RUnlock()

	for _, handle := range subscribers {
		if err := handle(ctx, evt); err != nil {
			b.log.Error("event handler failed", "event", evt.EventName(), "err", err)
		}
	}
}
