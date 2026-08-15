package domain

import "time"

// Event is an IMMUTABLE domain event: it carries a fact that occurred, never
// behavior. Events are published on the EventBus and consumed by features,
// enabling low coupling between bounded contexts.
type Event interface {
	// EventName identifies the event type (routing key on the bus).
	EventName() string
	// OccurredAt is the instant the fact happened.
	OccurredAt() time.Time
}

// Event names (routing keys), centralized to avoid typos.
const (
	EventMessageReceived  = "messaging.message_received"
	EventHistorySynced    = "messaging.history_synced"
	EventLabelEdited      = "labels.label_edited"
	EventChatLabeled      = "labels.chat_labeled"
	EventSessionConnected = "session.connected"
	EventSessionLoggedOut = "session.logged_out"
	EventQRCodeReady      = "session.qr_ready"
)

// ── Messaging ──────────────────────────────────────────────────────────────

// MessageReceived is emitted when a message arrives at the bridge (live). It carries
// the Message and the Chat already resolved (name), so the messaging feature can persist
// both without needing to re-query anything.
type MessageReceived struct {
	msg  Message
	chat Chat
}

// NewMessageReceived builds the event (fail-fast: message sender required).
func NewMessageReceived(msg Message, chat Chat) (MessageReceived, error) {
	if msg.Chat().IsZero() {
		return MessageReceived{}, ErrInvalidMessage
	}
	return MessageReceived{msg: msg, chat: chat}, nil
}

// EventName implements Event.
func (MessageReceived) EventName() string { return EventMessageReceived }

// OccurredAt implements Event (message timestamp).
func (e MessageReceived) OccurredAt() time.Time { return e.msg.Timestamp() }

// Message returns the received message.
func (e MessageReceived) Message() Message { return e.msg }

// Chat returns the chat (with resolved name).
func (e MessageReceived) Chat() Chat { return e.chat }

// From returns the sender (convenience for the contacts feature).
func (e MessageReceived) From() JID { return e.msg.Sender() }

// Body returns the message text (convenience).
func (e MessageReceived) Body() string { return e.msg.Content() }

// IsFromMe reports whether the message is from the account's own user.
func (e MessageReceived) IsFromMe() bool { return e.msg.IsFromMe() }

// HistorySynced is emitted when the server sends a history batch. It carries
// the chats and messages already translated into the domain (batch persistence).
type HistorySynced struct {
	chats    []Chat
	messages []Message
	at       time.Time
}

// NewHistorySynced builds the history sync event.
func NewHistorySynced(chats []Chat, messages []Message, at time.Time) HistorySynced {
	return HistorySynced{chats: chats, messages: messages, at: at}
}

// EventName implements Event.
func (HistorySynced) EventName() string { return EventHistorySynced }

// OccurredAt implements Event.
func (e HistorySynced) OccurredAt() time.Time { return e.at }

// Chats returns the synced chats.
func (e HistorySynced) Chats() []Chat { return e.chats }

// Messages returns the synced messages.
func (e HistorySynced) Messages() []Message { return e.messages }

// ── Labels ─────────────────────────────────────────────────────────────────

// LabelEdited is emitted when a label is created/renamed/removed.
type LabelEdited struct {
	label Label
	at    time.Time
}

// NewLabelEdited builds the event.
func NewLabelEdited(label Label, at time.Time) LabelEdited {
	return LabelEdited{label: label, at: at}
}

// EventName implements Event.
func (LabelEdited) EventName() string { return EventLabelEdited }

// OccurredAt implements Event.
func (e LabelEdited) OccurredAt() time.Time { return e.at }

// Label returns the edited label.
func (e LabelEdited) Label() Label { return e.label }

// ChatLabeled is emitted when a chat is labeled/unlabeled.
type ChatLabeled struct {
	assoc LabelAssociation
	at    time.Time
}

// NewChatLabeled builds the event.
func NewChatLabeled(assoc LabelAssociation, at time.Time) ChatLabeled {
	return ChatLabeled{assoc: assoc, at: at}
}

// EventName implements Event.
func (ChatLabeled) EventName() string { return EventChatLabeled }

// OccurredAt implements Event.
func (e ChatLabeled) OccurredAt() time.Time { return e.at }

// Association returns the label-to-chat link.
func (e ChatLabeled) Association() LabelAssociation { return e.assoc }

// ── Session ────────────────────────────────────────────────────────────────

// SessionConnected is emitted when the session comes online.
type SessionConnected struct {
	account string
	at      time.Time
}

// NewSessionConnected builds the event.
func NewSessionConnected(account string, at time.Time) SessionConnected {
	return SessionConnected{account: account, at: at}
}

// EventName implements Event.
func (SessionConnected) EventName() string { return EventSessionConnected }

// OccurredAt implements Event.
func (e SessionConnected) OccurredAt() time.Time { return e.at }

// Account returns the connected account.
func (e SessionConnected) Account() string { return e.account }

// SessionLoggedOut is emitted when the device is logged out (needs a new QR).
type SessionLoggedOut struct {
	at time.Time
}

// NewSessionLoggedOut builds the event.
func NewSessionLoggedOut(at time.Time) SessionLoggedOut {
	return SessionLoggedOut{at: at}
}

// EventName implements Event.
func (SessionLoggedOut) EventName() string { return EventSessionLoggedOut }

// OccurredAt implements Event.
func (e SessionLoggedOut) OccurredAt() time.Time { return e.at }

// QRCodeReady is emitted when a new pairing QR is generated and saved to disk.
// The delivery layer (tray) reacts by opening the image; the domain knows nothing about UI.
type QRCodeReady struct {
	path string
	at   time.Time
}

// NewQRCodeReady builds the event.
func NewQRCodeReady(path string, at time.Time) QRCodeReady {
	return QRCodeReady{path: path, at: at}
}

// EventName implements Event.
func (QRCodeReady) EventName() string { return EventQRCodeReady }

// OccurredAt implements Event.
func (e QRCodeReady) OccurredAt() time.Time { return e.at }

// Path returns the path to the saved QR PNG.
func (e QRCodeReady) Path() string { return e.path }
