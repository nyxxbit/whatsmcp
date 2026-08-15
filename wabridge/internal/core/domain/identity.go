package domain

import "strings"

// Identity is a sender's resolved identity, "WIDE VIEW": it carries the
// NAME, the NUMBER and the raw JID all at once, never one instead of another. Name and
// phone may come back empty (unknown); the JID is always present.
//
// Immutable Value Object; equality by value.
type Identity struct {
	jid   JID
	name  string
	phone string
}

// NewIdentity creates the identity (fail-fast: JID required; name/phone optional).
func NewIdentity(jid JID, name, phone string) (Identity, error) {
	if jid.IsZero() {
		return Identity{}, ErrInvalidJID
	}
	return Identity{jid: jid, name: strings.TrimSpace(name), phone: strings.TrimSpace(phone)}, nil
}

// JID returns the raw identifier (always present).
func (i Identity) JID() JID { return i.jid }

// Name returns the readable name ("" if unknown).
func (i Identity) Name() string { return i.name }

// Phone returns the number ("" if not resolved, e.g. @lid without a mapping).
func (i Identity) Phone() string { return i.phone }

// HasName reports whether the name was resolved.
func (i Identity) HasName() bool { return i.name != "" }

// HasPhone reports whether the number was resolved.
func (i Identity) HasPhone() bool { return i.phone != "" }

// Display builds a single string with everything known: "Name (number)", or just
// whatever is available, falling back to the JID's user as a last resort. For one-line logs/reports.
func (i Identity) Display() string {
	switch {
	case i.name != "" && i.phone != "":
		return i.name + " (" + i.phone + ")"
	case i.name != "":
		return i.name
	case i.phone != "":
		return i.phone
	default:
		return i.jid.User()
	}
}
