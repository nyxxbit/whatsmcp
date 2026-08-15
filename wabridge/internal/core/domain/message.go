package domain

import (
	"errors"
	"time"
)

// ErrInvalidMessage sinaliza uma mensagem sem o mínimo válido (fail-fast).
var ErrInvalidMessage = errors.New("domain: mensagem inválida")

// Message é a Entidade central do bounded context Messaging: uma mensagem de
// chat com identidade (id + chat). Pode carregar mídia opcional.
type Message struct {
	id        string
	chat      JID
	sender    JID
	content   string
	timestamp time.Time
	isFromMe  bool
	media     *Media
}

// NewMessage constrói uma mensagem válida. Fail-fast: exige um chat e ao menos
// conteúdo OU mídia (espelha o "skip if no content and no media" do legado).
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

// ID devolve o identificador da mensagem (pode ser vazio em itens de histórico).
func (m Message) ID() string { return m.id }

// Chat devolve o JID da conversa.
func (m Message) Chat() JID { return m.chat }

// Sender devolve o JID de quem enviou.
func (m Message) Sender() JID { return m.sender }

// Content devolve o texto (vazio para mídia sem legenda).
func (m Message) Content() string { return m.content }

// Timestamp devolve o instante da mensagem.
func (m Message) Timestamp() time.Time { return m.timestamp }

// IsFromMe indica se foi enviada pela própria conta.
func (m Message) IsFromMe() bool { return m.isFromMe }

// Media devolve a mídia anexa (nil se for só texto).
func (m Message) Media() *Media { return m.media }

// HasMedia indica se a mensagem carrega mídia.
func (m Message) HasMedia() bool { return m.media != nil }
