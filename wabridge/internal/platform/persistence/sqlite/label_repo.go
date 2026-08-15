package sqlite

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// LabelRepository persiste etiquetas e suas associações com conversas (messages.db).
type LabelRepository struct {
	db *sql.DB
}

var _ ports.LabelRepository = (*LabelRepository)(nil)

// NewLabelRepository cria o repositório (fail-fast: store obrigatório).
func NewLabelRepository(store *Store) *LabelRepository {
	if store == nil {
		panic("sqlite: LabelRepository exige Store")
	}
	return &LabelRepository{db: store.db}
}

// SaveLabel grava/atualiza uma etiqueta (deleted como 0/1, igual ao legado).
func (r *LabelRepository) SaveLabel(ctx context.Context, label domain.Label) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO labels (label_id, name, color, deleted) VALUES (?, ?, ?, ?)",
		label.ID(), label.Name(), label.Color(), boolToInt(label.Deleted()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: gravar etiqueta: %w", err)
	}
	return nil
}

// SaveAssociation grava/atualiza o vínculo etiqueta↔conversa (labeled como 0/1).
func (r *LabelRepository) SaveAssociation(ctx context.Context, assoc domain.LabelAssociation) error {
	_, err := r.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO label_chats (label_id, chat_jid, labeled) VALUES (?, ?, ?)",
		assoc.LabelID(), assoc.Chat().String(), boolToInt(assoc.Labeled()),
	)
	if err != nil {
		return fmt.Errorf("sqlite: gravar associação de etiqueta: %w", err)
	}
	return nil
}
