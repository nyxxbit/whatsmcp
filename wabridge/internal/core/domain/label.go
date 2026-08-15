package domain

import "errors"

// ErrInvalidLabel signals a label without an identifier (fail-fast).
var ErrInvalidLabel = errors.New("domain: invalid label")

// Label is an immutable Value Object for a WhatsApp Business label (e.g. "Work").
type Label struct {
	id      string
	name    string
	color   int
	deleted bool
}

// NewLabel builds a label (fail-fast: id required).
func NewLabel(id, name string, color int, deleted bool) (Label, error) {
	if id == "" {
		return Label{}, ErrInvalidLabel
	}
	return Label{id: id, name: name, color: color, deleted: deleted}, nil
}

// ID returns the label's identifier.
func (l Label) ID() string { return l.id }

// Name returns the label's name.
func (l Label) Name() string { return l.name }

// Color returns the label's color index.
func (l Label) Color() int { return l.color }

// Deleted reports whether the label was removed.
func (l Label) Deleted() bool { return l.deleted }

// LabelAssociation is an immutable Value Object: the link between a label and
// a chat (chat labeled or unlabeled).
type LabelAssociation struct {
	labelID string
	chat    JID
	labeled bool
}

// NewLabelAssociation builds the link (fail-fast: labelID and chat required).
func NewLabelAssociation(labelID string, chat JID, labeled bool) (LabelAssociation, error) {
	if labelID == "" {
		return LabelAssociation{}, ErrInvalidLabel
	}
	if chat.IsZero() {
		return LabelAssociation{}, ErrInvalidJID
	}
	return LabelAssociation{labelID: labelID, chat: chat, labeled: labeled}, nil
}

// LabelID returns the label's id.
func (a LabelAssociation) LabelID() string { return a.labelID }

// Chat returns the JID of the associated chat.
func (a LabelAssociation) Chat() JID { return a.chat }

// Labeled reports whether the chat is labeled (true) or was unlabeled (false).
func (a LabelAssociation) Labeled() bool { return a.labeled }
