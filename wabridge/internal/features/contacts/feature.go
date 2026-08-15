package contacts

import (
	"context"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Feature é a "arma" de contatos. Ao registrar, assina o evento de mensagem
// recebida e passa a logá-lo com a IDENTIDADE COMPLETA do remetente: nome, número
// E o JID cru (visão ampla). Adicionar isto ao sistema não exige alterar nenhuma
// outra parte (Open-Closed).
type Feature struct{}

var _ ports.Feature = Feature{}

// New cria a feature.
func New() Feature { return Feature{} }

// Name identifica a feature.
func (Feature) Name() string { return "contacts" }

// Register liga a feature ao núcleo usando apenas ports (DIP).
func (Feature) Register(deps ports.FeatureDeps) error {
	resolver := NewResolver(deps.Contacts)
	log := deps.Log.With("feature", "contacts")

	deps.Bus.Subscribe(domain.EventMessageReceived, func(ctx context.Context, evt domain.Event) error {
		msg, ok := evt.(domain.MessageReceived)
		if !ok {
			return nil // ignora silenciosamente eventos de outro formato
		}
		id := resolver.Identify(ctx, msg.From())
		log.Info("mensagem recebida",
			"nome", id.Name(), // nome legível ("" se desconhecido)
			"numero", id.Phone(), // número E.164 ("" se @lid sem mapa)
			"jid", id.JID().String(), // JID cru (sempre presente)
			"texto", msg.Body(),
		)
		return nil
	})
	return nil
}
