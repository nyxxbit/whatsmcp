package domain

import (
	"errors"
	"time"
)

// ErrChatNameUnknown is returned when there is no saved name for the chat yet.
var ErrChatNameUnknown = errors.New("domain: unknown chat name")

// Chat is an immutable Value Object representing a chat: identity (JID),
// readable name and the timestamp of the last message (used to order chats).
type Chat struct {
	jid             JID
	name            string
	lastMessageTime time.Time
}

// NewChat builds a chat (fail-fast: JID required).
func NewChat(jid JID, name string, lastMessageTime time.Time) (Chat, error) {
	if jid.IsZero() {
		return Chat{}, ErrInvalidJID
	}
	return Chat{jid: jid, name: name, lastMessageTime: lastMessageTime}, nil
}

// JID returns the chat's identifier.
func (c Chat) JID() JID { return c.jid }

// Name returns the chat's readable name.
func (c Chat) Name() string { return c.name }

// LastMessageTime returns the timestamp of the last message.
func (c Chat) LastMessageTime() time.Time { return c.lastMessageTime }
