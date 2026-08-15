package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// LabelRepository persists labels and their associations with chats (messages.db).
type LabelRepository struct {
	db *sql.DB
}

var _ ports.LabelRepository = (*LabelRepository)(nil)

// NewLabelRepository creates the repository (fail-fast: store is required).
func NewLabelRepository(store *Store) *LabelRepository {
	if store == nil {
		panic("sqlite: LabelRepository requires a Store")
	}
	return &LabelRepository{db: store.db}
}

// SaveLabel writes/updates a label (deleted as 0/1, same as the legacy code).
func (r *LabelRepository) SaveLabel(ctx context.Context, label domain.Label) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO labels (label_id, name, color, deleted) VALUES (?, ?, ?, ?)",
		label.ID(), label.Name(), label.Color(), boolToInt(label.Deleted()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: save label: %w", err)
	}
	return nil
}

// SaveAssociation writes/updates the label↔chat link (labeled as 0/1).
func (r *LabelRepository) SaveAssociation(ctx context.Context, assoc domain.LabelAssociation) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO label_chats (label_id, chat_jid, labeled) VALUES (?, ?, ?)",
		assoc.LabelID(), assoc.Chat().String(), boolToInt(assoc.Labeled()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: save label association: %w", err)
	}
	return nil
}
