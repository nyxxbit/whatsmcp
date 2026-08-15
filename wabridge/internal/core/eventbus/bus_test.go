package eventbus_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/eventbus"
	"github.com/nyxxbit/wabridge/internal/core/ports"
	"github.com/nyxxbit/wabridge/internal/platform/logging"
)

func anEvent(t *testing.T) domain.MessageReceived {
	t.Helper()
	jid := domain.MustJID("100000000000003@lid")
	msg, err := domain.NewMessage("id1", jid, jid, "oi", time.Now(), false, nil)
	if err != nil {
		t.Fatal(err)
	}
	chat, err := domain.NewChat(jid, "Contact", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	e, err := domain.NewMessageReceived(msg, chat)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestBus_entregaAosAssinantes(t *testing.T) {
	bus := eventbus.New(logging.Noop{})
	calls := 0
	bus.Subscribe("messaging.message_received", func(context.Context, domain.Event) error {
		calls++
		return nil
	})

	bus.Publish(context.Background(), anEvent(t))

	if calls != 1 {
		t.Fatalf("handler called %d times, expected 1", calls)
	}
}

func TestBus_naoEntregaAEventoDeOutroNome(t *testing.T) {
	bus := eventbus.New(logging.Noop{})
	called := false
	bus.Subscribe("outro.evento", func(context.Context, domain.Event) error {
		called = true
		return nil
	})

	bus.Publish(context.Background(), anEvent(t))

	if called {
		t.Fatal("handler for a different event should not have been called")
	}
}

func TestBus_handlerComErroNaoDerrubaOsDemais(t *testing.T) {
	bus := eventbus.New(logging.Noop{})
	var second ports.EventHandler
	secondRan := false
	second = func(context.Context, domain.Event) error { secondRan = true; return nil }

	bus.Subscribe("messaging.message_received", func(context.Context, domain.Event) error {
		return errors.New("boom")
	})
	bus.Subscribe("messaging.message_received", second)

	bus.Publish(context.Background(), anEvent(t))

	if !secondRan {
		t.Fatal("the second handler should run even if the first one fails")
	}
}
