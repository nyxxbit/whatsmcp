package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// ChatRepository persists chats and looks up already-known names (messages.db).
type ChatRepository struct {
	db *sql.DB
}

var _ ports.ChatRepository = (*ChatRepository)(nil)

// NewChatRepository creates the repository (fail-fast: store is required).
func NewChatRepository(store *Store) *ChatRepository {
	if store == nil {
		panic("sqlite: ChatRepository requires a Store")
	}
	return &ChatRepository{db: store.db}
}

// Upsert writes/updates the chat (INSERT OR REPLACE on PK jid).
func (r *ChatRepository) Upsert(ctx context.Context, chat domain.Chat) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO chats (jid, name, last_message_time) VALUES (?, ?, ?)",
		chat.JID().String(), chat.Name(), chat.LastMessageTime(),
	)
	if err != nil {
		return fmt.Errorf("sqlite: save chat: %w", err)
	}
	return nil
}

// FindName returns the chat's saved name; domain.ErrChatNameUnknown if there is none.
func (r *ChatRepository) FindName(ctx context.Context, jid domain.JID) (string, error) {
	var name sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT name FROM chats WHERE jid = ?", jid.String()).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", domain.ErrChatNameUnknown
	}
	if err != nil {
		return "", fmt.Errorf("sqlite: query chat name: %w", err)
	}
	if name.String == "" {
		return "", domain.ErrChatNameUnknown
	}
	return name.String, nil
}
