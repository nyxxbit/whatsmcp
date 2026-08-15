package contacts

import (
	"context"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Feature is the contacts plugin. When registered, it subscribes to the
// message received event and logs it with the sender's FULL IDENTITY: name,
// number, AND the raw JID (broad view). Adding this to the system requires no
// changes to any other part (Open-Closed).
type Feature struct{}

var _ ports.Feature = Feature{}

// New creates the feature.
func New() Feature { return Feature{} }

// Name identifies the feature.
func (Feature) Name() string { return "contacts" }

// Register wires the feature into the core using only ports (DIP).
func (Feature) Register(deps ports.FeatureDeps) error {
	resolver := NewResolver(deps.Contacts)
	log := deps.Log.With("feature", "contacts")

	deps.Bus.Subscribe(domain.EventMessageReceived, func(ctx context.Context, evt domain.Event) error {
		msg, ok := evt.(domain.MessageReceived)
		if !ok {
			return nil // silently ignore events of another format
		}
		id := resolver.Identify(ctx, msg.From())
		log.Info("message received",
			"name", id.Name(), // readable name ("" if unknown)
			"number", id.Phone(), // E.164 number ("" if @lid with no mapping)
			"jid", id.JID().String(), // raw JID (always present)
			"text", msg.Body(),
		)
		return nil
	})
	return nil
}
