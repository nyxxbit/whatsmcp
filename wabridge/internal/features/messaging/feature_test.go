package messaging_test

import (
	"context"
	"testing"
	"time"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/eventbus"
	"github.com/nyxxbit/wabridge/internal/core/ports"
	"github.com/nyxxbit/wabridge/internal/features/messaging"
	"github.com/nyxxbit/wabridge/internal/platform/logging"
)

type fakeMessageRepo struct {
	saved []domain.Message
	batch [][]domain.Message
}

func (r *fakeMessageRepo) Save(_ context.Context, m domain.Message) error {
	r.saved = append(r.saved, m)
	return nil
}
func (r *fakeMessageRepo) SaveBatch(_ context.Context, m []domain.Message) error {
	r.batch = append(r.batch, m)
	return nil
}
func (r *fakeMessageRepo) FindMedia(context.Context, string, string) (domain.Media, error) {
	return domain.Media{}, domain.ErrMediaNotFound
}

type fakeChatRepo struct{ upserts []domain.Chat }

func (r *fakeChatRepo) Upsert(_ context.Context, c domain.Chat) error {
	r.upserts = append(r.upserts, c)
	return nil
}
func (r *fakeChatRepo) FindName(context.Context, domain.JID) (string, error) {
	return "", domain.ErrChatNameUnknown
}

func wire(t *testing.T) (ports.EventBus, *fakeMessageRepo, *fakeChatRepo) {
	t.Helper()
	bus := eventbus.New(logging.Noop{})
	msgs, chats := &fakeMessageRepo{}, &fakeChatRepo{}
	err := messaging.New().Register(ports.FeatureDeps{
		Log: logging.Noop{}, Bus: bus, Messages: msgs, Chats: chats,
	})
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	return bus, msgs, chats
}

func TestMessaging_persisteMensagemAoVivo(t *testing.T) {
	bus, msgs, chats := wire(t)
	jid := domain.MustJID("100000000000003@lid")
	msg, _ := domain.NewMessage("ABC", jid, jid, "trabalhei dia 17", time.Now(), false, nil)
	chat, _ := domain.NewChat(jid, "Alex Carter", time.Now())
	evt, _ := domain.NewMessageReceived(msg, chat)

	bus.Publish(context.Background(), evt)

	if len(msgs.saved) != 1 || msgs.saved[0].Content() != "trabalhei dia 17" {
		t.Fatalf("mensagem não persistida: %+v", msgs.saved)
	}
	if len(chats.upserts) != 1 || chats.upserts[0].Name() != "Alex Carter" {
		t.Fatalf("conversa não persistida: %+v", chats.upserts)
	}
}

func TestMessaging_persisteHistoricoEmLote(t *testing.T) {
	bus, msgs, _ := wire(t)
	jid := domain.MustJID("5511999990003@s.whatsapp.net")
	m1, _ := domain.NewMessage("1", jid, jid, "oi", time.Now(), false, nil)
	m2, _ := domain.NewMessage("2", jid, jid, "tudo bem", time.Now(), true, nil)
	evt := domain.NewHistorySynced(nil, []domain.Message{m1, m2}, time.Now())

	bus.Publish(context.Background(), evt)

	if len(msgs.batch) != 1 || len(msgs.batch[0]) != 2 {
		t.Fatalf("lote de histórico não persistido: %+v", msgs.batch)
	}
}

func TestMessaging_failFastSemRepositorios(t *testing.T) {
	err := messaging.New().Register(ports.FeatureDeps{Log: logging.Noop{}, Bus: eventbus.New(logging.Noop{})})
	if err == nil {
		t.Fatal("esperava erro fail-fast sem Messages/Chats")
	}
}
