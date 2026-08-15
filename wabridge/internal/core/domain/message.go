package domain

import (
	"errors"
	"time"
)

// ErrInvalidMessage signals a message without the minimum required validity (fail-fast).
var ErrInvalidMessage = errors.New("domain: invalid message")

// Message is the central Entity of the Messaging bounded context: a chat
// message with identity (id + chat). It may carry optional media.
type Message struct {
	id        string
	chat      JID
	sender    JID
	content   string
	timestamp time.Time
	isFromMe  bool
	media     *Media
}

// NewMessage builds a valid message. Fail-fast: requires a chat and at least
// content OR media (mirrors the legacy "skip if no content and no media").
func NewMessage(id string, chat, sender JID, content string, ts time.Time, isFromMe bool, media *Media) (Message, error) {
	if chat.IsZero() {
		return Message{}, ErrInvalidMessage
	}
	if content == "" && media == nil {
		return Message{}, ErrInvalidMessage
	}
	return Message{
		id:        id,
		chat:      chat,
		sender:    sender,
		content:   content,
		timestamp: ts,
		isFromMe:  isFromMe,
		media:     media,
	}, nil
}

// ID returns the message's identifier (may be empty for history items).
func (m Message) ID() string { return m.id }

// Chat returns the chat's JID.
func (m Message) Chat() JID { return m.chat }

// Sender returns the JID of whoever sent it.
func (m Message) Sender() JID { return m.sender }

// Content returns the text (empty for media without a caption).
func (m Message) Content() string { return m.content }

// Timestamp returns the message's timestamp.
func (m Message) Timestamp() time.Time { return m.timestamp }

// IsFromMe reports whether it was sent by the account's own user.
func (m Message) IsFromMe() bool { return m.isFromMe }

// Media returns the attached media (nil if text-only).
func (m Message) Media() *Media { return m.media }

// HasMedia reports whether the message carries media.
func (m Message) HasMedia() bool { return m.media != nil }
