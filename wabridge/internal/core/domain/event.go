package domain

import "time"

// Event é um evento de domínio IMUTÁVEL: carrega um fato ocorrido, nunca
// comportamento. Eventos são publicados no EventBus e consumidos pelas features,
// permitindo baixo acoplamento entre bounded contexts.
type Event interface {
	// EventName identifica o tipo do evento (chave de roteamento no bus).
	EventName() string
	// OccurredAt é o instante em que o fato aconteceu.
	OccurredAt() time.Time
}

// Nomes de evento (chaves de roteamento), centralizados para evitar typos.
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

// MessageReceived é emitido quando uma mensagem chega ao bridge (ao vivo). Carrega
// a Mensagem e a Conversa já resolvida (nome), para a feature messaging persistir
// ambas sem precisar reconsultar nada.
type MessageReceived struct {
	msg  Message
	chat Chat
}

// NewMessageReceived constrói o evento (fail-fast: remetente da mensagem obrigatório).
func NewMessageReceived(msg Message, chat Chat) (MessageReceived, error) {
	if msg.Chat().IsZero() {
		return MessageReceived{}, ErrInvalidMessage
	}
	return MessageReceived{msg: msg, chat: chat}, nil
}

// EventName implementa Event.
func (MessageReceived) EventName() string { return EventMessageReceived }

// OccurredAt implementa Event (instante da mensagem).
func (e MessageReceived) OccurredAt() time.Time { return e.msg.Timestamp() }

// Message devolve a mensagem recebida.
func (e MessageReceived) Message() Message { return e.msg }

// Chat devolve a conversa (com nome resolvido).
func (e MessageReceived) Chat() Chat { return e.chat }

// From devolve o remetente (conveniência para a feature de contatos).
func (e MessageReceived) From() JID { return e.msg.Sender() }

// Body devolve o texto da mensagem (conveniência).
func (e MessageReceived) Body() string { return e.msg.Content() }

// IsFromMe indica se a mensagem é da própria conta.
func (e MessageReceived) IsFromMe() bool { return e.msg.IsFromMe() }

// HistorySynced é emitido quando o servidor envia um lote de histórico. Carrega
// as conversas e mensagens já traduzidas para o domínio (persistência em lote).
type HistorySynced struct {
	chats    []Chat
	messages []Message
	at       time.Time
}

// NewHistorySynced constrói o evento de sincronização de histórico.
func NewHistorySynced(chats []Chat, messages []Message, at time.Time) HistorySynced {
	return HistorySynced{chats: chats, messages: messages, at: at}
}

// EventName implementa Event.
func (HistorySynced) EventName() string { return EventHistorySynced }

// OccurredAt implementa Event.
func (e HistorySynced) OccurredAt() time.Time { return e.at }

// Chats devolve as conversas sincronizadas.
func (e HistorySynced) Chats() []Chat { return e.chats }

// Messages devolve as mensagens sincronizadas.
func (e HistorySynced) Messages() []Message { return e.messages }

// ── Labels ─────────────────────────────────────────────────────────────────

// LabelEdited é emitido quando uma etiqueta é criada/renomeada/removida.
type LabelEdited struct {
	label Label
	at    time.Time
}

// NewLabelEdited constrói o evento.
func NewLabelEdited(label Label, at time.Time) LabelEdited {
	return LabelEdited{label: label, at: at}
}

// EventName implementa Event.
func (LabelEdited) EventName() string { return EventLabelEdited }

// OccurredAt implementa Event.
func (e LabelEdited) OccurredAt() time.Time { return e.at }

// Label devolve a etiqueta editada.
func (e LabelEdited) Label() Label { return e.label }

// ChatLabeled é emitido quando uma conversa é etiquetada/desetiquetada.
type ChatLabeled struct {
	assoc LabelAssociation
	at    time.Time
}

// NewChatLabeled constrói o evento.
func NewChatLabeled(assoc LabelAssociation, at time.Time) ChatLabeled {
	return ChatLabeled{assoc: assoc, at: at}
}

// EventName implementa Event.
func (ChatLabeled) EventName() string { return EventChatLabeled }

// OccurredAt implementa Event.
func (e ChatLabeled) OccurredAt() time.Time { return e.at }

// Association devolve o vínculo etiqueta↔conversa.
func (e ChatLabeled) Association() LabelAssociation { return e.assoc }

// ── Session ────────────────────────────────────────────────────────────────

// SessionConnected é emitido quando a sessão fica online.
type SessionConnected struct {
	account string
	at      time.Time
}

// NewSessionConnected constrói o evento.
func NewSessionConnected(account string, at time.Time) SessionConnected {
	return SessionConnected{account: account, at: at}
}

// EventName implementa Event.
func (SessionConnected) EventName() string { return EventSessionConnected }

// OccurredAt implementa Event.
func (e SessionConnected) OccurredAt() time.Time { return e.at }

// Account devolve a conta conectada.
func (e SessionConnected) Account() string { return e.account }

// SessionLoggedOut é emitido quando o aparelho é deslogado (precisa de novo QR).
type SessionLoggedOut struct {
	at time.Time
}

// NewSessionLoggedOut constrói o evento.
func NewSessionLoggedOut(at time.Time) SessionLoggedOut {
	return SessionLoggedOut{at: at}
}

// EventName implementa Event.
func (SessionLoggedOut) EventName() string { return EventSessionLoggedOut }

// OccurredAt implementa Event.
func (e SessionLoggedOut) OccurredAt() time.Time { return e.at }

// QRCodeReady é emitido quando um novo QR de pareamento é gerado e salvo em disco.
// A camada de entrega (tray) reage abrindo a imagem, o domínio não conhece UI.
type QRCodeReady struct {
	path string
	at   time.Time
}

// NewQRCodeReady constrói o evento.
func NewQRCodeReady(path string, at time.Time) QRCodeReady {
	return QRCodeReady{path: path, at: at}
}

// EventName implementa Event.
func (QRCodeReady) EventName() string { return EventQRCodeReady }

// OccurredAt implementa Event.
func (e QRCodeReady) OccurredAt() time.Time { return e.at }

// Path devolve o caminho do PNG do QR salvo.
func (e QRCodeReady) Path() string { return e.path }
