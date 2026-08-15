// Package sqlite implements the repositories on top of SQLite, the
// persistence adapter of the hexagonal architecture. The schema is identical
// to the legacy bridge (messages.db), so existing consumers and the MCP
// server keep reading the same database without any migration.
package sqlite

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3" // CGO "sqlite3" driver
)

// Store holds the connection to messages.db and ensures the schema. It's
// shared by the message, chat, and label repositories (all on the same
// database).
type Store struct {
	db *sql.DB
}

// Open opens (creating if needed) the messages database, its directory, and
// the schema. Fail-fast: any I/O/SQL error aborts immediately.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite: empty database path")
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("sqlite: create directory %q: %w", dir, err)
		}
	}
	db, err := sql.Open("sqlite3", "file:"+path+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open %q: %w", path, err)
	}
	if err := createSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// createSchema creates the tables if they don't exist (identical to the legacy schema).
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
		return fmt.Errorf("sqlite: create schema: %w", err)
	}
	return nil
}

// DB exposes the connection (used by this package's repositories).
func (s *Store) DB() *sql.DB { return s.db }

// Close closes the connection.
func (s *Store) Close() error { return s.db.Close() }

// boolToInt converts a bool to 0/1 (the format the legacy code wrote for deleted/labeled).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
