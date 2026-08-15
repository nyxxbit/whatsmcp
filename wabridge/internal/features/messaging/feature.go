// Package messaging is the messages feature (bounded context): it persists
// what arrives (live and via history sync) and exposes the media download use
// case.
package messaging

import (
	"context"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Feature subscribes to message events and stores them in the repositories.
// Pluggable: adding it to the Registry requires no changes to any other part
// (Open-Closed).
type Feature struct{}

var _ ports.Feature = Feature{}

// New creates the feature.
func New() Feature { return Feature{} }

// Name identifies the feature.
func (Feature) Name() string { return "messaging" }

// Register wires the feature into the core (DIP: ports only). Fail-fast on dependencies.
func (Feature) Register(deps ports.FeatureDeps) error {
	if deps.Messages == nil || deps.Chats == nil {
		return fmt.Errorf("messaging: Messages and Chats are required")
	}
	log := deps.Log.With("feature", "messaging")

	deps.Bus.Subscribe(domain.EventMessageReceived, func(ctx context.Context, evt domain.Event) error {
		received, ok := evt.(domain.MessageReceived)
		if !ok {
			return nil
		}
		if err := deps.Chats.Upsert(ctx, received.Chat()); err != nil {
			log.Warn("failed to save chat", "err", err)
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
				log.Warn("failed to save chat from history", "err", err)
			}
		}
		if err := deps.Messages.SaveBatch(ctx, synced.Messages()); err != nil {
			return err
		}
		log.Info("history persisted", "chats", len(synced.Chats()), "messages", len(synced.Messages()))
		return nil
	})

	return nil
}
