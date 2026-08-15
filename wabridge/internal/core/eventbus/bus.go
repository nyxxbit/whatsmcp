// Package eventbus implementa o EventBus do núcleo (Observer/Mediator).
package eventbus

import (
	"context"
	"sync"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// InMemoryBus é um EventBus síncrono e thread-safe. Simples por design (KISS):
// entrega cada evento aos handlers na ordem de assinatura. Um handler que
// falha é registrado no Logger e NÃO derruba os demais (isolamento de falha).
type InMemoryBus struct {
	mu       sync.RWMutex
	handlers map[string][]ports.EventHandler
	log      ports.Logger
}

// Garante em tempo de compilação que InMemoryBus satisfaz a porta.
var _ ports.EventBus = (*InMemoryBus)(nil)

// New cria um EventBus. O logger é obrigatório (fail-fast).
func New(log ports.Logger) *InMemoryBus {
	if log == nil {
		panic("eventbus: logger é obrigatório")
	}
	return &InMemoryBus{handlers: make(map[string][]ports.EventHandler), log: log}
}

// Subscribe registra um handler para um tipo de evento (fail-fast em args inválidos).
func (b *InMemoryBus) Subscribe(eventName string, handler ports.EventHandler) {
	if eventName == "" || handler == nil {
		panic("eventbus: subscribe exige eventName e handler")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.handlers[eventName] = append(b.handlers[eventName], handler)
}

// Publish entrega o evento a todos os handlers assinados para o seu nome.
func (b *InMemoryBus) Publish(ctx context.Context, evt domain.Event) {
	if evt == nil {
		return
	}
	b.mu.RLock()
	subscribers := b.handlers[evt.EventName()]
	b.mu.RUnlock()

	for _, handle := range subscribers {
		if err := handle(ctx, evt); err != nil {
			b.log.Error("handler de evento falhou", "event", evt.EventName(), "err", err)
		}
	}
}
