// Package labels is the WhatsApp Business labels feature (bounded context)
// (e.g., "Work"): it persists labels and their associations with chats.
package labels

import (
	"context"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Feature subscribes to label events and stores them. Pluggable (Open-Closed).
type Feature struct{}

var _ ports.Feature = Feature{}

// New creates the feature.
func New() Feature { return Feature{} }

// Name identifies the feature.
func (Feature) Name() string { return "labels" }

// Register wires the feature into the core (DIP: ports only). Fail-fast on the dependency.
func (Feature) Register(deps ports.FeatureDeps) error {
	if deps.Labels == nil {
		return fmt.Errorf("labels: LabelRepository is required")
	}
	log := deps.Log.With("feature", "labels")

	deps.Bus.Subscribe(domain.EventLabelEdited, func(ctx context.Context, evt domain.Event) error {
		edited, ok := evt.(domain.LabelEdited)
		if !ok {
			return nil
		}
		log.Info("label updated", "id", edited.Label().ID(), "name", edited.Label().Name())
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
