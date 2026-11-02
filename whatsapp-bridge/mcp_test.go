package main

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestGuardLocal(t *testing.T) {
	h := guardLocal(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for _, c := range []struct {
		host, fetchSite string
		want            int
	}{
		{"127.0.0.1:8080", "", http.StatusOK},
		{"localhost:8080", "", http.StatusOK},
		{"[::1]:8080", "", http.StatusOK},
		{"attacker.example:8080", "", http.StatusForbidden}, // DNS rebinding
		{"127.0.0.1:8080", "cross-site", http.StatusForbidden},
	} {
		r := httptest.NewRequest(http.MethodPost, "/api/send", nil)
		r.Host = c.host
		if c.fetchSite != "" {
			r.Header.Set("Sec-Fetch-Site", c.fetchSite)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != c.want {
			t.Errorf("Host %s, Sec-Fetch-Site %q: got %d, want %d", c.host, c.fetchSite, w.Code, c.want)
		}
	}
}

func TestListMessagesNeedsChatOrSender(t *testing.T) {
	if _, _, err := toolListMessages(context.Background(), nil, listMessagesIn{Query: "x"}); err == nil {
		t.Fatal("list_messages without chat_jid or sender_phone_number would read every chat")
	}
}

func TestSenderAloneReadsOnlyTheDirectChat(t *testing.T) {
	d, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	d.SetMaxOpenConns(1) // every connection to :memory: is a database of its own
	defer d.Close()
	if _, err := d.Exec(`CREATE TABLE chats (jid TEXT PRIMARY KEY, name TEXT, last_message_time TIMESTAMP);
		CREATE TABLE messages (id TEXT, chat_jid TEXT, sender TEXT, content TEXT, timestamp TIMESTAMP, is_from_me BOOLEAN, media_type TEXT);
		INSERT INTO messages VALUES ('1', '111@lid', '111', 'in the direct chat', '2026-11-02 10:00:00-03:00', 0, '');
		INSERT INTO messages VALUES ('2', '999@g.us', '111', 'in the family group', '2026-11-02 11:00:00-03:00', 0, '')`); err != nil {
		t.Fatal(err)
	}
	old := globalStore
	globalStore = &MessageStore{db: d}
	t.Cleanup(func() { globalStore = old })

	res, _, err := toolListMessages(context.Background(), nil, listMessagesIn{SenderPhone: "111@lid"})
	if err != nil {
		t.Fatal(err)
	}
	if got := res.Content[0].(*mcp.TextContent).Text; !strings.Contains(got, "direct chat") || strings.Contains(got, "family") {
		t.Fatalf("a sender alone must read only the direct chat, got:\n%s", got)
	}
}
