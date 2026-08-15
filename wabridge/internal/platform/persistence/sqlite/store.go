// Package sqlite implementa os repositórios sobre SQLite, adapter de
// persistência da arquitetura hexagonal. O schema é idêntico ao do bridge legado
// (messages.db), so existing consumers and the MCP server keep reading the
// mesma base sem qualquer migração.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // driver CGO "sqlite3"
)

// Store detém a conexão com messages.db e garante o schema. É compartilhado pelos
// repositórios de mensagem, conversa e etiqueta (todos na mesma base).
type Store struct {
	db *sql.DB
}

// Open abre (criando se preciso) o banco de mensagens, o diretório e o schema.
// Fail-fast: qualquer erro de IO/SQL aborta na hora.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite: caminho do banco vazio")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: criar diretório %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("sqlite: abrir %q: %w", path, err)
	}
	if err := createSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// createSchema cria as tabelas se não existirem (idêntico ao legado).
func createSchema(db *sql.DB) error {
	const ddl = `
		CREATE TABLE IF NOT EXISTS chats (
			jid TEXT PRIMARY KEY,
			name TEXT,
			last_message_time TIMESTAMP
		);

		CREATE TABLE IF NOT EXISTS messages (
			id TEXT,
			chat_jid TEXT,
			sender TEXT,
			content TEXT,
			timestamp TIMESTAMP,
			is_from_me BOOLEAN,
			media_type TEXT,
			filename TEXT,
			url TEXT,
			media_key BLOB,
			file_sha256 BLOB,
			file_enc_sha256 BLOB,
			file_length INTEGER,
			direct_path TEXT,
			PRIMARY KEY (id, chat_jid),
			FOREIGN KEY (chat_jid) REFERENCES chats(jid)
		);

		CREATE TABLE IF NOT EXISTS labels (
			label_id TEXT PRIMARY KEY,
			name TEXT,
			color INTEGER,
			deleted INTEGER
		);

		CREATE TABLE IF NOT EXISTS label_chats (
			label_id TEXT,
			chat_jid TEXT,
			labeled INTEGER,
			PRIMARY KEY (label_id, chat_jid)
		);`
	if _, err := db.Exec(ddl); err != nil {
		return fmt.Errorf("sqlite: criar schema: %w", err)
	}
	return nil
}

// DB expõe a conexão (usada pelos repositórios deste pacote).
func (s *Store) DB() *sql.DB { return s.db }

// Close fecha a conexão.
func (s *Store) Close() error { return s.db.Close() }

// boolToInt converte bool para 0/1 (formato gravado pelo legado em deleted/labeled).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
