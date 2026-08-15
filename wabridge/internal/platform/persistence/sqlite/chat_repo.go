package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// ChatRepository persiste conversas e consulta nomes já conhecidos (messages.db).
type ChatRepository struct {
	db *sql.DB
}

var _ ports.ChatRepository = (*ChatRepository)(nil)

// NewChatRepository cria o repositório (fail-fast: store obrigatório).
func NewChatRepository(store *Store) *ChatRepository {
	if store == nil {
		panic("sqlite: ChatRepository exige Store")
	}
	return &ChatRepository{db: store.db}
}

// Upsert grava/atualiza a conversa (INSERT OR REPLACE pela PK jid).
func (r *ChatRepository) Upsert(ctx context.Context, chat domain.Chat) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO chats (jid, name, last_message_time) VALUES (?, ?, ?)",
		chat.JID().String(), chat.Name(), chat.LastMessageTime(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: gravar conversa: %w", err)
	}
	return nil
}

// FindName devolve o nome salvo da conversa; domain.ErrChatNameUnknown se não houver.
func (r *ChatRepository) FindName(ctx context.Context, jid domain.JID) (string, error) {
	var name sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT name FROM chats WHERE jid = ?", jid.String()).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrChatNameUnknown
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: consultar nome da conversa: %w", err)
	}
	if name.String == "" {
		return "", domain.ErrChatNameUnknown
	}
	return name.String, nil
}
