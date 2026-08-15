package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// MessageRepository persists messages and retrieves media metadata (messages.db).
type MessageRepository struct {
	db *sql.DB
}

var _ ports.MessageRepository = (*MessageRepository)(nil)

// NewMessageRepository creates the repository (fail-fast: store is required).
func NewMessageRepository(store *Store) *MessageRepository {
	if store == nil {
		panic("sqlite: MessageRepository requires a Store")
	}
	return &MessageRepository{db: store.db}
}

const insertMessageSQL = `
	INSERT OR REPLACE INTO messages
		(id, chat_jid, sender, content, timestamp, is_from_me,
		 media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length, direct_path)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

// Save writes a message (INSERT OR REPLACE on PK id+chat_jid).
func (r *MessageRepository) Save(ctx context.Context, msg domain.Message) error {
	return execInsertMessage(ctx, r.db, msg)
}

// SaveBatch writes a batch in a single transaction (history).
func (r *MessageRepository) SaveBatch(ctx context.Context, msgs []domain.Message) error {
	if len(msgs) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("sqlite: begin transaction: %w", err)
	}
	stmt, err := tx.PrepareContext(ctx, insertMessageSQL)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("sqlite: prepare insert: %w", err)
	}
	defer stmt.Close()

	for _, msg := range msgs {
		args := messageArgs(msg)
		if _, err := stmt.ExecContext(ctx, args...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("sqlite: save message in batch: %w", err)
		}
	}
	return tx.Commit()
}

func execInsertMessage(ctx context.Context, db *sql.DB, msg domain.Message) error {
	if _, err := db.ExecContext(ctx, insertMessageSQL, messageArgs(msg)...); err != nil {
		return fmt.Errorf("sqlite: save message: %w", err)
	}
	return nil
}

// messageArgs flattens a Message into the table's 14 columns (media → empty when absent).
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
		msg.Sender().User(), // bare user, as the legacy code writes it
		msg.Content(),
		msg.Timestamp(),
		msg.IsFromMe(),
		mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength, directPath,
	}
}

// FindMedia retrieves a message's media metadata for download.
// Returns domain.ErrMediaNotFound if the message doesn't exist or has no media.
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
		return domain.Media{}, fmt.Errorf("sqlite: query media: %w", err)
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
