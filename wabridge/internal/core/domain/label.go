package domain

import "errors"

// ErrInvalidLabel sinaliza uma etiqueta sem identificador (fail-fast).
var ErrInvalidLabel = errors.New("domain: etiqueta inválida")

// Label é um Value Object imutável de etiqueta do WhatsApp Business (ex: "Work").
type Label struct {
	id      string
	name    string
	color   int
	deleted bool
}

// NewLabel constrói uma etiqueta (fail-fast: id obrigatório).
func NewLabel(id, name string, color int, deleted bool) (Label, error) {
	if id == "" {
		return Label{}, ErrInvalidLabel
	}
	return Label{id: id, name: name, color: color, deleted: deleted}, nil
}

// ID devolve o identificador da etiqueta.
func (l Label) ID() string { return l.id }

// Name devolve o nome da etiqueta.
func (l Label) Name() string { return l.name }

// Color devolve o índice de cor da etiqueta.
func (l Label) Color() int { return l.color }

// Deleted indica se a etiqueta foi removida.
func (l Label) Deleted() bool { return l.deleted }

// LabelAssociation é um Value Object imutável: o vínculo entre uma etiqueta e
// uma conversa (chat etiquetado ou desetiquetado).
type LabelAssociation struct {
	labelID string
	chat    JID
	labeled bool
}

// NewLabelAssociation constrói o vínculo (fail-fast: labelID e chat obrigatórios).
func NewLabelAssociation(labelID string, chat JID, labeled bool) (LabelAssociation, error) {
	if labelID == "" {
		return LabelAssociation{}, ErrInvalidLabel
	}
	if chat.IsZero() {
		return LabelAssociation{}, ErrInvalidJID
	}
	return LabelAssociation{labelID: labelID, chat: chat, labeled: labeled}, nil
}

// LabelID devolve o id da etiqueta.
func (a LabelAssociation) LabelID() string { return a.labelID }

// Chat devolve o JID da conversa associada.
func (a LabelAssociation) Chat() JID { return a.chat }

// Labeled indica se a conversa está etiquetada (true) ou foi desetiquetada (false).
func (a LabelAssociation) Labeled() bool { return a.labeled }
