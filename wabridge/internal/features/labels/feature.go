// Package labels é a feature (bounded context) de etiquetas do WhatsApp Business
// (ex: "Work"): persiste etiquetas e suas associações com conversas.
package labels

import (
	"context"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Feature assina os eventos de etiqueta e os grava. Plugável (Open-Closed).
type Feature struct{}

var _ ports.Feature = Feature{}

// New cria a feature.
func New() Feature { return Feature{} }

// Name identifica a feature.
func (Feature) Name() string { return "labels" }

// Register liga a feature ao núcleo (DIP: só ports). Fail-fast na dependência.
func (Feature) Register(deps ports.FeatureDeps) error {
	if deps.Labels == nil {
		return fmt.Errorf("labels: LabelRepository é obrigatório")
	}
	log := deps.Log.With("feature", "labels")

	deps.Bus.Subscribe(domain.EventLabelEdited, func(ctx context.Context, evt domain.Event) error {
		edited, ok := evt.(domain.LabelEdited)
		if !ok {
			return nil
		}
		log.Info("etiqueta atualizada", "id", edited.Label().ID(), "nome", edited.Label().Name())
		return deps.Labels.SaveLabel(ctx, edited.Label())
	})

	deps.Bus.Subscribe(domain.EventChatLabeled, func(ctx context.Context, evt domain.Event) error {
		labeled, ok := evt.(domain.ChatLabeled)
		if !ok {
			return nil
		}
		return deps.Labels.SaveAssociation(ctx, labeled.Association())
	})

	return nil
}
