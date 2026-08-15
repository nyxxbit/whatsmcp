// Package ports declares the core's contracts (interfaces). It is the boundary of the
// hexagonal architecture: the domain and features depend on these abstractions,
// never on concrete implementations (Dependency Inversion Principle).
//
// Interfaces are small and focused (Interface Segregation) and live together here
// since they are the shared "ammo kit". Each feature/adapter uses only what it needs.
package ports

import (
	"context"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// ── Cross-cutting infra ──────────────────────────────────────────────────────

// Logger is the structured logging contract. Implementations: slog (production),
// no-op (tests/silence). Swappable without touching whoever logs.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// ── Repositories (abstracted persistence, Repository pattern) ───────────────

// ContactRepository resolves the FULL identity of a JID (name + number + raw
// JID), cross-referencing lid_map and contacts. "Wide view": it returns everything it
// can; unknown fields come back empty. Error only on infrastructure failure. "No name"
// is NOT an error (positive programming: the absence of a name is data, not an exception).
type ContactRepository interface {
	Identify(ctx context.Context, jid domain.JID) (domain.Identity, error)
}

// MessageRepository persists messages and retrieves media metadata.
type MessageRepository interface {
	Save(ctx context.Context, msg domain.Message) error
	SaveBatch(ctx context.Context, msgs []domain.Message) error
	// FindMedia retrieves a message's media metadata (for download).
	// Returns domain.ErrMediaNotFound when the message has no media.
	FindMedia(ctx context.Context, messageID, chatJID string) (domain.Media, error)
}

// ChatRepository persists chats and looks up already-known names.
type ChatRepository interface {
	Upsert(ctx context.Context, chat domain.Chat) error
	// FindName returns the chat's saved name; error if there is no name yet.
	FindName(ctx context.Context, jid domain.JID) (string, error)
}

// LabelRepository persists labels and their associations with chats.
type LabelRepository interface {
	SaveLabel(ctx context.Context, label domain.Label) error
	SaveAssociation(ctx context.Context, assoc domain.LabelAssociation) error
}

// ── Output to WhatsApp (lib adapters) ─────────────────────────────────

// MessageSender is the port for sending messages (text and media).
type MessageSender interface {
	SendText(ctx context.Context, to domain.JID, body string) error
	// SendMedia sends the file at mediaPath with an optional caption; the type is
	// inferred from the extension (Strategy in the adapter).
	SendMedia(ctx context.Context, to domain.JID, mediaPath, caption string) error
}

// MediaFetcher downloads and decrypts a media's bytes from its
// metadata (low-level layer: only talks to WhatsApp's servers).
type MediaFetcher interface {
	Fetch(ctx context.Context, media domain.Media) ([]byte, error)
}

// MediaDownloader is the high-level use case: it resolves the metadata, uses the
// disk cache, downloads if needed and returns the already-saved result.
type MediaDownloader interface {
	Download(ctx context.Context, messageID, chatJID string) (domain.DownloadResult, error)
}

// SessionManager controls the connection lifecycle (Connect unifies
// reconnecting and pairing via QR, like the "Connect" button in the legacy bridge).
type SessionManager interface {
	Connect()
	Disconnect()
	Status() domain.SessionStatus
}

// LabelSyncer triggers label synchronization (fullSync of the app state).
type LabelSyncer interface {
	SyncLabels(ctx context.Context) error
}

// ── Events & Features (Observer + Open-Closed) ─────────────────────────────

// EventHandler reacts to a domain event.
type EventHandler func(ctx context.Context, evt domain.Event) error

// EventBus is the core's event channel (Observer/Mediator pattern). Features
// publish and subscribe to facts without knowing about each other.
type EventBus interface {
	Subscribe(eventName string, handler EventHandler)
	Publish(ctx context.Context, evt domain.Event)
}

// FeatureDeps is the dependency kit ("ammo") handed to each feature at
// registration, ports only. No feature sees concrete implementations.
// Fields not used by a feature are simply left nil (it only takes what it needs).
type FeatureDeps struct {
	Log      Logger
	Bus      EventBus
	Contacts ContactRepository
	Messages MessageRepository
	Chats    ChatRepository
	Labels   LabelRepository
	Sender   MessageSender
}

// Feature is a pluggable "weapon". Implementing this interface and registering it is enough
// to add behavior, the core remains closed for modification
// (Open-Closed Principle).
type Feature interface {
	// Name identifies the feature (logs and diagnostics).
	Name() string
	// Register wires the feature into the core using only the dependencies (ports).
	Register(deps FeatureDeps) error
}
