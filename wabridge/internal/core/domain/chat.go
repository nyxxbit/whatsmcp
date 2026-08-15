package domain

import (
	"errors"
	"time"
)

// ErrChatNameUnknown é retornado quando ainda não há nome salvo para a conversa.
var ErrChatNameUnknown = errors.New("domain: nome da conversa desconhecido")

// Chat é um Value Object imutável que representa uma conversa: identidade (JID),
// nome legível e o instante da última mensagem (usado para ordenar conversas).
type Chat struct {
	jid             JID
	name            string
	lastMessageTime time.Time
}

// NewChat constrói uma conversa (fail-fast: JID obrigatório).
func NewChat(jid JID, name string, lastMessageTime time.Time) (Chat, error) {
	if jid.IsZero() {
		return Chat{}, ErrInvalidJID
	}
	return Chat{jid: jid, name: name, lastMessageTime: lastMessageTime}, nil
}

// JID devolve o identificador da conversa.
func (c Chat) JID() JID { return c.jid }

// Name devolve o nome legível da conversa.
func (c Chat) Name() string { return c.name }

// LastMessageTime devolve o instante da última mensagem.
func (c Chat) LastMessageTime() time.Time { return c.lastMessageTime }
