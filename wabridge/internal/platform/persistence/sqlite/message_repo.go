package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// MessageRepository persiste mensagens e recupera metadados de mídia (messages.db).
type MessageRepository struct {
	db *sql.DB
}

var _ ports.MessageRepository = (*MessageRepository)(nil)

// NewMessageRepository cria o repositório (fail-fast: store obrigatório).
func NewMessageRepository(store *Store) *MessageRepository {
	if store == nil {
		panic("sqlite: MessageRepository exige Store")
	}
	return &MessageRepository{db: store.db}
}

const insertMessageSQL = `
	INSERT OR REPLACE INTO messages
		(id, chat_jid, sender, content, timestamp, is_from_me,
		 media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length, direct_path)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// Save grava uma mensagem (INSERT OR REPLACE pela PK id+chat_jid).
func (r *MessageRepository) Save(ctx context.Context, msg domain.Message) error {
	return execInsertMessage(ctx, r.db, msg)
}

// SaveBatch grava um lote em uma única transação (histórico).
func (r *MessageRepository) SaveBatch(ctx context.Context, msgs []domain.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: iniciar transação: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertMessageSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: preparar insert: %w", err)
	}
	defer stmt.Close()

	for _, msg := range msgs {
		args := messageArgs(msg)
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: gravar mensagem em lote: %w", err)
		}
	}
	return tx.Commit()
}

func execInsertMessage(ctx context.Context, db *sql.DB, msg domain.Message) error {
	if _, err := db.ExecContext(ctx, insertMessageSQL, messageArgs(msg)...); err != nil {
		return fmt.Errorf("sqlite: gravar mensagem: %w", err)
	}
	return nil
}

// messageArgs achata uma Message nas 14 colunas da tabela (mídia → vazios quando ausente).
func messageArgs(msg domain.Message) []any {
	var (
		mediaType, filename, url, directPath string
		mediaKey, fileSHA256, fileEncSHA256  []byte
		fileLength                           uint64
	)
	if m := msg.Media(); m != nil {
		mediaType = string(m.Kind())
		filename = m.Filename()
		url = m.URL()
		directPath = m.DirectPath()
		mediaKey = m.MediaKey()
		fileSHA256 = m.FileSHA256()
		fileEncSHA256 = m.FileEncSHA256()
		fileLength = m.FileLength()
	}
	return []any{
		msg.ID(),
		msg.Chat().String(),
		msg.Sender().User(), // bare user, como o legado grava
		msg.Content(),
		msg.Timestamp(),
		msg.IsFromMe(),
		mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength, directPath,
	}
}

// FindMedia recupera os metadados de mídia de uma mensagem para download.
// Devolve domain.ErrMediaNotFound se a mensagem não existir ou não tiver mídia.
func (r *MessageRepository) FindMedia(ctx context.Context, messageID, chatJID string) (domain.Media, error) {
	var (
		mediaType, filename, url, directPath sql.NullString
		mediaKey, fileSHA256, fileEncSHA256  []byte
		fileLength                           sql.NullInt64
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length, direct_path
		 FROM messages WHERE id = ? AND chat_jid = ?`,
		messageID, chatJID,
	).Scan(&mediaType, &filename, &url, &mediaKey, &fileSHA256, &fileEncSHA256, &fileLength, &directPath)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Media{}, domain.ErrMediaNotFound
	}
	if err != nil {
		return domain.Media{}, fmt.Errorf("sqlite: consultar mídia: %w", err)
	}
	if mediaType.String == "" {
		return domain.Media{}, domain.ErrMediaNotFound
	}
	return domain.NewMedia(domain.MediaSpec{
		Kind:          domain.MediaKind(mediaType.String),
		Filename:      filename.String,
		URL:           url.String,
		DirectPath:    directPath.String,
		MediaKey:      mediaKey,
		FileSHA256:    fileSHA256,
		FileEncSHA256: fileEncSHA256,
		FileLength:    uint64(fileLength.Int64),
	})
}
