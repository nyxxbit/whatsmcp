// Package domain contains the bridge's pure domain model: entities,
// value objects and events. It imports no infrastructure, network libs or
// database. It is the center of the Clean Architecture.
package domain

import (
	"errors"
	"fmt"
	"strings"
)

// ServerType identifies the server of a WhatsApp JID.
type ServerType string

const (
	ServerPN        ServerType = "s.whatsapp.net" // phone number
	ServerLID       ServerType = "lid"            // hidden identifier (Linked ID)
	ServerGroup     ServerType = "g.us"           // group
	ServerBroadcast ServerType = "broadcast"      // broadcast list
)

// ErrInvalidJID signals a malformed JID (used in constructors' fail-fast checks).
var ErrInvalidJID = errors.New("domain: invalid jid")

// JID is an IMMUTABLE Value Object that identifies an entity in WhatsApp
// (user, group, list). Equality is by value; there are no setters, any
// "change" produces a new instance.
type JID struct {
	user   string
	server ServerType
}

// NewJID builds a JID from "user@server", validating it right away (fail-fast).
// Positive programming: the function only returns a JID if it is valid.
func NewJID(raw string) (JID, error) {
	raw = strings.TrimSpace(raw)
	at := strings.LastIndex(raw, "@")
	if at <= 0 || at >= len(raw)-1 {
		return JID{}, fmt.Errorf("%w: %q", ErrInvalidJID, raw)
	}
	return JID{user: raw[:at], server: ServerType(raw[at+1:])}, nil
}

// MustJID is a helper for tests and constants: it panics if invalid.
func MustJID(raw string) JID {
	jid, err := NewJID(raw)
	if err != nil {
		panic(err)
	}
	return jid
}

// NewJIDFromParts builds a JID from user and server already split apart (fail-fast).
func NewJIDFromParts(user string, server ServerType) (JID, error) {
	user = strings.TrimSpace(user)
	if user == "" || server == "" {
		return JID{}, fmt.Errorf("%w: user=%q server=%q", ErrInvalidJID, user, server)
	}
	return JID{user: user, server: server}, nil
}

// ParseRecipient parses a recipient coming from the API: it accepts a full JID
// ("user@server") or a raw number (becomes @s.whatsapp.net, like in the legacy bridge).
// Positive programming: it only returns an error when even that is not possible.
func ParseRecipient(raw string) (JID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return JID{}, fmt.Errorf("%w: empty recipient", ErrInvalidJID)
	}
	if strings.Contains(raw, "@") {
		return NewJID(raw)
	}
	return JID{user: raw, server: ServerPN}, nil
}

// User returns the user part (before the @).
func (j JID) User() string { return j.user }

// Server returns the server type (after the @).
func (j JID) Server() ServerType { return j.server }

// IsLID reports whether this is a hidden identifier (@lid), used by contacts that hide their phone number.
func (j JID) IsLID() bool { return j.server == ServerLID }

// IsPN reports whether this is a phone number (@s.whatsapp.net).
func (j JID) IsPN() bool { return j.server == ServerPN }

// IsGroup reports whether this is a group (@g.us).
func (j JID) IsGroup() bool { return j.server == ServerGroup }

// IsZero reports the empty JID (zero value), useful for guard clauses.
func (j JID) IsZero() bool { return j.user == "" && j.server == "" }

// String rebuilds the canonical form "user@server".
func (j JID) String() string {
	if j.IsZero() {
		return ""
	}
	return j.user + "@" + string(j.server)
}
