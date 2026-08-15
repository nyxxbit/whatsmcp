// Package ports declara os contratos (interfaces) do núcleo. É a fronteira da
// arquitetura hexagonal: o domínio e as features dependem destas abstrações,
// nunca de implementações concretas (Dependency Inversion Principle).
//
// Interfaces são pequenas e focadas (Interface Segregation) e ficam juntas aqui
// por serem o "kit de munição" compartilhado. Cada feature/adapter usa só o que precisa.
package ports

import (
	"context"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// ── Infra transversal ──────────────────────────────────────────────────────

// Logger é o contrato de log estruturado. Implementações: slog (produção),
// no-op (testes/silêncio). Substituível sem tocar em quem loga.
type Logger interface {
	Info(msg string, args ...any)
	Warn(msg string, args ...any)
	Error(msg string, args ...any)
	With(args ...any) Logger
}

// ── Repositórios (persistência abstraída, padrão Repository) ───────────────

// ContactRepository resolve a identidade COMPLETA de um JID (nome + número + JID
// cru), cruzando lid_map e contacts. "Visão ampla": devolve tudo o que conseguir;
// campos desconhecidos vêm vazios. Erro só em falha de infraestrutura - "sem nome"
// NÃO é erro (programação positiva: a ausência de nome é um dado, não uma exceção).
type ContactRepository interface {
	Identify(ctx context.Context, jid domain.JID) (domain.Identity, error)
}

// MessageRepository persiste mensagens e recupera metadados de mídia.
type MessageRepository interface {
	Save(ctx context.Context, msg domain.Message) error
	SaveBatch(ctx context.Context, msgs []domain.Message) error
	// FindMedia recupera os metadados de mídia de uma mensagem (para download).
	// Devolve domain.ErrMediaNotFound quando a mensagem não tem mídia.
	FindMedia(ctx context.Context, messageID, chatJID string) (domain.Media, error)
}

// ChatRepository persiste conversas e consulta nomes já conhecidos.
type ChatRepository interface {
	Upsert(ctx context.Context, chat domain.Chat) error
	// FindName devolve o nome salvo da conversa; erro se ainda não houver nome.
	FindName(ctx context.Context, jid domain.JID) (string, error)
}

// LabelRepository persiste etiquetas e suas associações com conversas.
type LabelRepository interface {
	SaveLabel(ctx context.Context, label domain.Label) error
	SaveAssociation(ctx context.Context, assoc domain.LabelAssociation) error
}

// ── Saída para o WhatsApp (adapters da lib) ─────────────────────────────────

// MessageSender é a porta de envio de mensagens (texto e mídia).
type MessageSender interface {
	SendText(ctx context.Context, to domain.JID, body string) error
	// SendMedia envia o arquivo em mediaPath com legenda opcional; o tipo é
	// inferido pela extensão (Strategy no adapter).
	SendMedia(ctx context.Context, to domain.JID, mediaPath, caption string) error
}

// MediaFetcher baixa e descriptografa os bytes de uma mídia a partir dos seus
// metadados (camada baixa: só fala com os servidores do WhatsApp).
type MediaFetcher interface {
	Fetch(ctx context.Context, media domain.Media) ([]byte, error)
}

// MediaDownloader é o caso de uso de alto nível: resolve os metadados, usa o
// cache em disco, baixa se preciso e devolve o resultado já salvo.
type MediaDownloader interface {
	Download(ctx context.Context, messageID, chatJID string) (domain.DownloadResult, error)
}

// SessionManager controla o ciclo de vida da conexão (Connect unifica
// reconectar e parear via QR, como o botão "Conectar" do bridge legado).
type SessionManager interface {
	Connect()
	Disconnect()
	Status() domain.SessionStatus
}

// LabelSyncer dispara a sincronização de etiquetas (fullSync do app state).
type LabelSyncer interface {
	SyncLabels(ctx context.Context) error
}

// ── Eventos & Features (Observer + Open-Closed) ─────────────────────────────

// EventHandler reage a um evento de domínio.
type EventHandler func(ctx context.Context, evt domain.Event) error

// EventBus é o canal de eventos do núcleo (padrão Observer/Mediator). Features
// publicam e assinam fatos sem conhecer umas às outras.
type EventBus interface {
	Subscribe(eventName string, handler EventHandler)
	Publish(ctx context.Context, evt domain.Event)
}

// FeatureDeps é o kit de dependências ("munição") entregue a cada feature no
// registro, apenas ports. Nenhuma feature enxerga implementações concretas.
// Campos não usados por uma feature ficam simplesmente nil (ela só pega o que precisa).
type FeatureDeps struct {
	Log      Logger
	Bus      EventBus
	Contacts ContactRepository
	Messages MessageRepository
	Chats    ChatRepository
	Labels   LabelRepository
	Sender   MessageSender
}

// Feature é uma "arma" plugável. Implementar esta interface e registrá-la basta
// para adicionar comportamento, o núcleo permanece fechado para modificação
// (Open-Closed Principle).
type Feature interface {
	// Name identifica a feature (logs e diagnóstico).
	Name() string
	// Register liga a feature ao núcleo usando apenas as dependências (ports).
	Register(deps FeatureDeps) error
}
