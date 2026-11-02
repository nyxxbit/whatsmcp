package main

// MCP server of the bridge, over Streamable HTTP on the REST port:
//
//	claude mcp add --transport http whatsapp http://127.0.0.1:8080/mcp
//
// It replaces the upstream Python server (whatsapp-mcp-server) with the same tool names and
// parameters, read straight from the bridge's store and client. Stateless: the bridge restarts often,
// and a session that died with it would leave the client broken until it reconnected.
//
// The store is the owner's whole WhatsApp, friends and family included, so every message read is bound
// to one chat: a sender alone means the direct chat with that person, never every group the person is
// in, and list_chats leaves the last message out unless asked. A conversation can be stored under its
// LID and under its phone number, so both are read.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"go.mau.fi/whatsmeow/types"
)

const mcpVersion = "1.0.0"

func init() {
	s := mcp.NewServer(&mcp.Implementation{Name: "whatsmcp", Version: mcpVersion}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "search_contacts", Description: "Search WhatsApp contacts by name or phone number."}, toolSearchContacts)
	mcp.AddTool(s, &mcp.Tool{Name: "list_messages", Description: "Get the WhatsApp messages of one chat (chat_jid), newest first, optionally " +
		"filtered by sender, date and text, each with the messages around it. sender_phone_number alone reads the direct chat with that person. " +
		"One of the two is required."}, toolListMessages)
	mcp.AddTool(s, &mcp.Tool{Name: "list_chats", Description: "List WhatsApp chats by name or JID. The last message is left out unless include_last_message is true."}, toolListChats)
	mcp.AddTool(s, &mcp.Tool{Name: "get_chat", Description: "Get WhatsApp chat metadata by JID."}, toolGetChat)
	mcp.AddTool(s, &mcp.Tool{Name: "get_direct_chat_by_contact", Description: "Get the direct WhatsApp chat with a phone number."}, toolGetDirectChat)
	mcp.AddTool(s, &mcp.Tool{Name: "get_contact_chats", Description: "Get the WhatsApp chats a contact takes part in, groups included."}, toolGetContactChats)
	mcp.AddTool(s, &mcp.Tool{Name: "get_last_interaction", Description: "Get the most recent message of the direct WhatsApp chat with a contact."}, toolGetLastInteraction)
	mcp.AddTool(s, &mcp.Tool{Name: "get_message_context", Description: "Get the messages around one WhatsApp message, in its chat."}, toolGetMessageContext)
	mcp.AddTool(s, &mcp.Tool{Name: "send_message", Description: "Send a WhatsApp text message to a person or group. For groups use the JID."}, toolSendMessage)
	mcp.AddTool(s, &mcp.Tool{Name: "send_file", Description: "Send a picture, video, document or audio via WhatsApp; an Opus .ogg goes as a voice message. " +
		"For groups use the JID."}, toolSendFile)
	mcp.AddTool(s, &mcp.Tool{Name: "download_media", Description: "Download the media of a WhatsApp message and get the local file path."}, toolDownloadMedia)
	mcp.AddTool(s, &mcp.Tool{Name: "connection_status", Description: "Tell whether the bridge is logged in and the WhatsApp server answers, " +
		"and when the last message was stored."}, toolConnectionStatus)
	http.Handle("/mcp", mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return s },
		&mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true}))
}

// --- inputs: the names and defaults of the upstream Python server

type queryIn struct {
	Query string `json:"query" jsonschema:"term matched against contact names and phone numbers"`
}

type listMessagesIn struct {
	After         string `json:"after,omitempty" jsonschema:"ISO-8601 date in local time: only messages after it"`
	Before        string `json:"before,omitempty" jsonschema:"ISO-8601 date in local time: only messages before it"`
	SenderPhone   string `json:"sender_phone_number,omitempty" jsonschema:"only messages from this sender, by phone number or JID"`
	ChatJID       string `json:"chat_jid,omitempty" jsonschema:"only messages of this chat"`
	Query         string `json:"query,omitempty" jsonschema:"only messages containing this text"`
	Limit         int    `json:"limit,omitempty" jsonschema:"maximum messages (default 20)"`
	Page          int    `json:"page,omitempty" jsonschema:"page, from 0"`
	IncludeCtx    *bool  `json:"include_context,omitempty" jsonschema:"include the messages around each match (default true)"`
	ContextBefore *int   `json:"context_before,omitempty" jsonschema:"messages before each match (default 1)"`
	ContextAfter  *int   `json:"context_after,omitempty" jsonschema:"messages after each match (default 1)"`
}

type listChatsIn struct {
	Query       string `json:"query,omitempty" jsonschema:"term matched against chat names and JIDs"`
	Limit       int    `json:"limit,omitempty" jsonschema:"maximum chats (default 20)"`
	Page        int    `json:"page,omitempty" jsonschema:"page, from 0"`
	LastMessage bool   `json:"include_last_message,omitempty" jsonschema:"include each chat's last message (default false)"`
	SortBy      string `json:"sort_by,omitempty" jsonschema:"last_active (default) or name"`
}

type chatIn struct {
	ChatJID     string `json:"chat_jid" jsonschema:"the JID of the chat"`
	LastMessage *bool  `json:"include_last_message,omitempty" jsonschema:"include the last message (default true)"`
}

type phoneIn struct {
	Phone string `json:"sender_phone_number" jsonschema:"the phone number"`
}

type contactChatsIn struct {
	JID   string `json:"jid" jsonschema:"the contact's JID or phone number"`
	Limit int    `json:"limit,omitempty" jsonschema:"maximum chats (default 20)"`
	Page  int    `json:"page,omitempty" jsonschema:"page, from 0"`
}

type jidIn struct {
	JID string `json:"jid" jsonschema:"the contact's JID or phone number"`
}

type contextIn struct {
	MessageID string `json:"message_id" jsonschema:"the ID of the message"`
	Before    *int   `json:"before,omitempty" jsonschema:"messages before it (default 5)"`
	After     *int   `json:"after,omitempty" jsonschema:"messages after it (default 5)"`
}

type sendMessageIn struct {
	Recipient string `json:"recipient" jsonschema:"phone number with country code, or a JID: 123@s.whatsapp.net, 123@lid or a group 123@g.us"`
	Message   string `json:"message" jsonschema:"the text to send"`
}

type sendFileIn struct {
	Recipient string `json:"recipient" jsonschema:"phone number with country code, or a JID: 123@s.whatsapp.net, 123@lid or a group 123@g.us"`
	MediaPath string `json:"media_path" jsonschema:"absolute path of the file"`
}

type downloadIn struct {
	MessageID string `json:"message_id" jsonschema:"the ID of the message with the media"`
	ChatJID   string `json:"chat_jid" jsonschema:"the JID of the chat, as list_messages shows it"`
}

// --- helpers

// text is a tool's answer: a string as is, anything else as indented JSON.
func text(v any) (*mcp.CallToolResult, any, error) {
	s, ok := v.(string)
	if !ok {
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, nil, err
		}
		s = string(b)
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: s}}}, nil, nil
}

func messagesDB() (*sql.DB, error) {
	if globalStore == nil || globalStore.db == nil {
		return nil, errors.New("the message store is not open yet")
	}
	return globalStore.db, nil
}

func orDefault(v, d int) int {
	if v <= 0 {
		return d
	}
	return v
}

func ptrOr(p *int, d int) int {
	if p == nil || *p < 0 {
		return d
	}
	return *p
}

// sides returns a chat's JID and its alternate (LID <-> phone number), as the bridge stores them.
func sides(ctx context.Context, raw string) []string {
	j, err := jidDe(raw)
	if err != nil {
		return []string{raw}
	}
	j = j.ToNonAD()
	out := []string{j.String()}
	if c := globalClient; c != nil && c.Store != nil {
		if alt, err := c.Store.GetAltJID(ctx, j); err == nil && !alt.IsEmpty() {
			out = append(out, alt.ToNonAD().String())
		}
	}
	return out
}

// senders returns the user parts a contact can appear as in messages.sender.
func senders(ctx context.Context, raw string) []string {
	out := sides(ctx, raw)
	for i, j := range out {
		out[i], _, _ = strings.Cut(j, "@")
	}
	return out
}

func marks(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func args(lists ...[]string) []any {
	var out []any
	for _, l := range lists {
		for _, v := range l {
			out = append(out, v)
		}
	}
	return out
}

type msgRow struct {
	ts, sender, chatName, content, chatJID, id, mediaType string
	fromMe                                                bool
}

// COALESCE keeps the timestamp as the stored text: go-sqlite3 turns a bare TIMESTAMP column into a
// time.Time, which scans back as RFC 3339 and no longer compares with what is stored.
const msgCols = `COALESCE(m.timestamp, ''), COALESCE(m.sender, ''), COALESCE(c.name, ''), COALESCE(m.content, ''),
	m.is_from_me, m.chat_jid, m.id, COALESCE(m.media_type, '')`

const msgFrom = ` FROM messages m LEFT JOIN chats c ON c.jid = m.chat_jid `

func queryMsgs(ctx context.Context, d *sql.DB, q string, params ...any) ([]msgRow, error) {
	rows, err := d.QueryContext(ctx, q, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []msgRow
	for rows.Next() {
		var r msgRow
		if err := rows.Scan(&r.ts, &r.sender, &r.chatName, &r.content, &r.fromMe, &r.chatJID, &r.id, &r.mediaType); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// displayName is the contact's name on the user's LID or phone number, else its chat's name. The
// bridge names a LID chat it could not resolve after the number itself, so that one does not count.
func displayName(ctx context.Context, d *sql.DB, user string) string {
	jids := append(sides(ctx, user+"@"+types.HiddenUserServer), sides(ctx, user+"@"+types.DefaultUserServer)...)
	if c := globalClient; c != nil && c.Store != nil && c.Store.Contacts != nil {
		for _, s := range jids {
			j, _ := types.ParseJID(s)
			if ci, err := c.Store.Contacts.GetContact(ctx, j); err == nil && ci.Found {
				if ci.FullName != "" {
					return ci.FullName
				}
				if ci.PushName != "" {
					return ci.PushName
				}
			}
		}
	}
	var name string
	if d.QueryRowContext(ctx, `SELECT name FROM chats WHERE jid IN (`+marks(len(jids))+`) AND COALESCE(name, '') NOT IN ('', ?) LIMIT 1`,
		append(args(jids), user)...).Scan(&name) == nil {
		return name
	}
	return user
}

func formatMsg(ctx context.Context, d *sql.DB, r msgRow) string {
	ts := r.ts
	if len(ts) > 19 {
		ts = ts[:19]
	}
	from := "Me"
	if !r.fromMe {
		from = displayName(ctx, d, r.sender)
	}
	chat, media := r.chatName, ""
	if user, server, _ := strings.Cut(r.chatJID, "@"); chat == user && server != types.GroupServer {
		chat = displayName(ctx, d, user)
	}
	if chat != "" {
		chat = "Chat: " + chat + " "
	}
	if r.mediaType != "" {
		media = fmt.Sprintf("[%s - Message ID: %s - Chat JID: %s] ", r.mediaType, r.id, r.chatJID)
	}
	return fmt.Sprintf("[%s] %sFrom: %s: %s%s", ts, chat, from, media, r.content)
}

func formatLines(ctx context.Context, d *sql.DB, rs []msgRow) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = formatMsg(ctx, d, r)
	}
	return out
}

// around returns the messages before and after r in its own chat, in time order.
func around(ctx context.Context, d *sql.DB, r msgRow, nBefore, nAfter int) ([]msgRow, []msgRow, error) {
	const q = `SELECT ` + msgCols + msgFrom + `WHERE m.chat_jid = ? AND m.timestamp %s ? ORDER BY m.timestamp %s LIMIT ?`
	before, err := queryMsgs(ctx, d, fmt.Sprintf(q, "<", "DESC"), r.chatJID, r.ts, nBefore)
	if err != nil {
		return nil, nil, err
	}
	slices.Reverse(before)
	after, err := queryMsgs(ctx, d, fmt.Sprintf(q, ">", "ASC"), r.chatJID, r.ts, nAfter)
	return before, after, err
}

type chatOut struct {
	JID             string `json:"jid"`
	Name            string `json:"name"`
	LastMessageTime string `json:"last_message_time"`
	IsGroup         bool   `json:"is_group"`
	LastMessage     string `json:"last_message,omitempty"`
	LastSender      string `json:"last_sender,omitempty"`
	LastIsFromMe    *bool  `json:"last_is_from_me,omitempty"`
}

func queryChats(ctx context.Context, d *sql.DB, withLast bool, where, order string, params ...any) ([]chatOut, error) {
	cols, join := "'', '', NULL", ""
	if withLast {
		cols = "COALESCE(m.content, ''), COALESCE(m.sender, ''), m.is_from_me"
		join = "LEFT JOIN messages m ON m.chat_jid = c.jid AND m.timestamp = c.last_message_time"
	}
	rows, err := d.QueryContext(ctx, "SELECT c.jid, COALESCE(c.name, ''), COALESCE(c.last_message_time, ''), "+cols+
		" FROM chats c "+join+" "+where+" "+order, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []chatOut{}
	seen := map[string]bool{}
	for rows.Next() {
		var c chatOut
		var fromMe sql.NullBool
		if err := rows.Scan(&c.JID, &c.Name, &c.LastMessageTime, &c.LastMessage, &c.LastSender, &fromMe); err != nil {
			return nil, err
		}
		if seen[c.JID] { // two messages share the chat's last timestamp
			continue
		}
		seen[c.JID] = true
		c.IsGroup = strings.HasSuffix(c.JID, "@g.us")
		if fromMe.Valid {
			c.LastIsFromMe = &fromMe.Bool
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// --- tools

func toolSearchContacts(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	p := "%" + in.Query + "%"
	rows, err := d.QueryContext(ctx, `SELECT DISTINCT jid, COALESCE(name, '') FROM chats
		WHERE (LOWER(name) LIKE LOWER(?) OR LOWER(jid) LIKE LOWER(?)) AND jid NOT LIKE '%@g.us' ORDER BY name, jid LIMIT 50`, p, p)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	out := []map[string]string{}
	for rows.Next() {
		var jid, name string
		if err := rows.Scan(&jid, &name); err != nil {
			return nil, nil, err
		}
		user, _, _ := strings.Cut(jid, "@")
		out = append(out, map[string]string{"phone_number": user, "name": name, "jid": jid})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return text(out)
}

func toolListMessages(ctx context.Context, _ *mcp.CallToolRequest, in listMessagesIn) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.ChatJID) == "" && strings.TrimSpace(in.SenderPhone) == "" {
		return nil, nil, errors.New("chat_jid or sender_phone_number is required: the store holds the owner's whole WhatsApp, " +
			"and messages are only read inside the chat or the sender asked for")
	}
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	chat := in.ChatJID
	if strings.TrimSpace(chat) == "" {
		chat = in.SenderPhone // a sender alone reads the direct chat, never every group the person is in
	}
	cs := sides(ctx, chat)
	where := []string{"m.chat_jid IN (" + marks(len(cs)) + ")"}
	params := args(cs)
	if in.SenderPhone != "" {
		ss := senders(ctx, in.SenderPhone)
		where = append(where, "m.sender IN ("+marks(len(ss))+")")
		params = append(params, args(ss)...)
	}
	// stored as "2006-01-02 15:04:05-07:00", so an ISO date compares as text once the T is a space
	if in.After != "" {
		where = append(where, "m.timestamp > ?")
		params = append(params, strings.Replace(strings.TrimSpace(in.After), "T", " ", 1))
	}
	if in.Before != "" {
		where = append(where, "m.timestamp < ?")
		params = append(params, strings.Replace(strings.TrimSpace(in.Before), "T", " ", 1))
	}
	if in.Query != "" {
		where = append(where, "LOWER(m.content) LIKE LOWER(?)")
		params = append(params, "%"+in.Query+"%")
	}
	limit := orDefault(in.Limit, 20)
	found, err := queryMsgs(ctx, d, `SELECT `+msgCols+msgFrom+`WHERE `+strings.Join(where, " AND ")+
		` ORDER BY m.timestamp DESC LIMIT ? OFFSET ?`, append(params, limit, in.Page*limit)...)
	if err != nil {
		return nil, nil, err
	}
	all := found
	if in.IncludeCtx == nil || *in.IncludeCtx {
		all = nil
		for _, r := range found {
			before, after, err := around(ctx, d, r, ptrOr(in.ContextBefore, 1), ptrOr(in.ContextAfter, 1))
			if err != nil {
				return nil, nil, err
			}
			all = append(append(append(all, before...), r), after...)
		}
	}
	if len(all) == 0 {
		return text("No messages to display.")
	}
	return text(strings.Join(formatLines(ctx, d, all), "\n"))
}

func toolListChats(ctx context.Context, _ *mcp.CallToolRequest, in listChatsIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	where, params := "", []any{}
	if in.Query != "" {
		where = "WHERE (LOWER(c.name) LIKE LOWER(?) OR c.jid LIKE ?)"
		params = append(params, "%"+in.Query+"%", "%"+in.Query+"%")
	}
	order := "ORDER BY c.last_message_time DESC"
	if in.SortBy == "name" {
		order = "ORDER BY c.name"
	}
	limit := orDefault(in.Limit, 20)
	out, err := queryChats(ctx, d, in.LastMessage, where, order+" LIMIT ? OFFSET ?", append(params, limit, in.Page*limit)...)
	if err != nil {
		return nil, nil, err
	}
	return text(out)
}

func toolGetChat(ctx context.Context, _ *mcp.CallToolRequest, in chatIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	out, err := queryChats(ctx, d, in.LastMessage == nil || *in.LastMessage, "WHERE c.jid = ?", "", in.ChatJID)
	if err != nil {
		return nil, nil, err
	}
	if len(out) == 0 {
		return text("Chat not found: " + in.ChatJID)
	}
	return text(out[0])
}

func toolGetDirectChat(ctx context.Context, _ *mcp.CallToolRequest, in phoneIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	cs := sides(ctx, in.Phone)
	out, err := queryChats(ctx, d, true, "WHERE c.jid IN ("+marks(len(cs))+")", "ORDER BY c.last_message_time DESC", args(cs)...)
	if err != nil {
		return nil, nil, err
	}
	if len(out) == 0 {
		return text("No direct chat with " + in.Phone)
	}
	return text(out[0])
}

func toolGetContactChats(ctx context.Context, _ *mcp.CallToolRequest, in contactChatsIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	cs, ss := sides(ctx, in.JID), senders(ctx, in.JID)
	limit := orDefault(in.Limit, 20)
	out, err := queryChats(ctx, d, false,
		"WHERE c.jid IN ("+marks(len(cs))+") OR c.jid IN (SELECT chat_jid FROM messages WHERE sender IN ("+marks(len(ss))+"))",
		"ORDER BY c.last_message_time DESC LIMIT ? OFFSET ?", append(args(cs, ss), limit, in.Page*limit)...)
	if err != nil {
		return nil, nil, err
	}
	return text(out)
}

func toolGetLastInteraction(ctx context.Context, _ *mcp.CallToolRequest, in jidIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	cs := sides(ctx, in.JID)
	found, err := queryMsgs(ctx, d, `SELECT `+msgCols+msgFrom+`WHERE m.chat_jid IN (`+marks(len(cs))+`) ORDER BY m.timestamp DESC LIMIT 1`,
		args(cs)...)
	if err != nil {
		return nil, nil, err
	}
	if len(found) == 0 {
		return text("No messages with " + in.JID)
	}
	return text(formatMsg(ctx, d, found[0]))
}

func toolGetMessageContext(ctx context.Context, _ *mcp.CallToolRequest, in contextIn) (*mcp.CallToolResult, any, error) {
	d, err := messagesDB()
	if err != nil {
		return nil, nil, err
	}
	found, err := queryMsgs(ctx, d, `SELECT `+msgCols+msgFrom+`WHERE m.id = ? LIMIT 1`, in.MessageID)
	if err != nil {
		return nil, nil, err
	}
	if len(found) == 0 {
		return nil, nil, fmt.Errorf("message %s not found", in.MessageID)
	}
	before, after, err := around(ctx, d, found[0], ptrOr(in.Before, 5), ptrOr(in.After, 5))
	if err != nil {
		return nil, nil, err
	}
	return text(map[string]any{
		"message": formatMsg(ctx, d, found[0]),
		"before":  formatLines(ctx, d, before),
		"after":   formatLines(ctx, d, after),
	})
}

func send(recipient, message, mediaPath string) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(recipient) == "" {
		return nil, nil, errors.New("recipient must be provided")
	}
	if globalClient == nil {
		return nil, nil, errors.New("the WhatsApp client is not running")
	}
	ok, msg := sendWhatsAppMessage(globalClient, recipient, message, mediaPath)
	if !ok {
		return nil, nil, fmt.Errorf("not sent: %s", msg)
	}
	return text(map[string]any{"success": true, "message": "Message sent to " + recipient, "message_id": msg})
}

func toolSendMessage(_ context.Context, _ *mcp.CallToolRequest, in sendMessageIn) (*mcp.CallToolResult, any, error) {
	if in.Message == "" {
		return nil, nil, errors.New("message must be provided")
	}
	return send(in.Recipient, in.Message, "")
}

func toolSendFile(_ context.Context, _ *mcp.CallToolRequest, in sendFileIn) (*mcp.CallToolResult, any, error) {
	if in.MediaPath == "" {
		return nil, nil, errors.New("media_path must be provided")
	}
	return send(in.Recipient, "", in.MediaPath)
}

func toolDownloadMedia(_ context.Context, _ *mcp.CallToolRequest, in downloadIn) (*mcp.CallToolResult, any, error) {
	if globalClient == nil || globalStore == nil {
		return nil, nil, errors.New("the bridge is not ready")
	}
	ok, mediaType, _, path, err := downloadMedia(globalClient, globalStore, in.MessageID, in.ChatJID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to download media: %w", err)
	}
	if !ok {
		return nil, nil, errors.New("failed to download media")
	}
	return text(map[string]any{"success": true, "message": "Successfully downloaded " + mediaType + " media", "file_path": path})
}

// toolConnectionStatus asks the server about the account's own number, like the watcher does:
// IsConnected alone has said yes with the socket dead.
func toolConnectionStatus(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, any, error) {
	out := map[string]any{"version": mcpVersion, "logged_in": false, "server": "not asked"}
	if c := globalClient; c != nil && c.Store != nil && c.Store.ID != nil {
		out["logged_in"] = true
		out["account"] = c.Store.ID.ToNonAD().String()
		qctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		t0 := time.Now()
		res, err := c.IsOnWhatsApp(qctx, []string{"+" + c.Store.ID.User})
		switch {
		case err != nil:
			out["server"] = "no answer: " + err.Error()
		case len(res) == 0 || !res[0].IsIn:
			out["server"] = "answered without the account's own number"
		default:
			out["server"] = fmt.Sprintf("answered in %d ms", time.Since(t0).Milliseconds())
		}
	}
	if d, err := messagesDB(); err == nil {
		var last sql.NullString
		if d.QueryRowContext(ctx, `SELECT MAX(timestamp) FROM messages`).Scan(&last) == nil && last.Valid {
			out["last_message_stored"] = last.String
		}
	}
	return text(out)
}
