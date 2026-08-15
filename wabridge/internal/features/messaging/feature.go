// Package messaging é a feature (bounded context) de mensagens: persiste o que
// chega (ao vivo e por history sync) e expõe o caso de uso de download de mídia.
package messaging

import (
	"context"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Feature assina os eventos de mensagem e os grava nos repositórios. Plugável:
// adicioná-la ao Registry não exige tocar em nenhuma outra parte (Open-Closed).
type Feature struct{}

var _ ports.Feature = Feature{}

// New cria a feature.
func New() Feature { return Feature{} }

// Name identifica a feature.
func (Feature) Name() string { return "messaging" }

// Register liga a feature ao núcleo (DIP: só ports). Fail-fast nas dependências.
func (Feature) Register(deps ports.FeatureDeps) error {
	if deps.Messages == nil || deps.Chats == nil {
		return fmt.Errorf("messaging: Messages e Chats são obrigatórios")
	}
	log := deps.Log.With("feature", "messaging")

	deps.Bus.Subscribe(domain.EventMessageReceived, func(ctx context.Context, evt domain.Event) error {
		received, ok := evt.(domain.MessageReceived)
		if !ok {
			return nil
		}
		if err := deps.Chats.Upsert(ctx, received.Chat()); err != nil {
			log.Warn("falha ao gravar conversa", "err", err)
		}
		return deps.Messages.Save(ctx, received.Message())
	})

	deps.Bus.Subscribe(domain.EventHistorySynced, func(ctx context.Context, evt domain.Event) error {
		synced, ok := evt.(domain.HistorySynced)
		if !ok {
			return nil
		}
		for _, chat := range synced.Chats() {
			if err := deps.Chats.Upsert(ctx, chat); err != nil {
				log.Warn("falha ao gravar conversa do histórico", "err", err)
			}
		}
		if err := deps.Messages.SaveBatch(ctx, synced.Messages()); err != nil {
			return err
		}
		log.Info("histórico persistido", "conversas", len(synced.Chats()), "mensagens", len(synced.Messages()))
		return nil
	})

	return nil
}
