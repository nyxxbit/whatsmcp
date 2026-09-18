package main

import (
	"context"
	"database/sql"
	_ "embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/mdp/qrterminal"
	"rsc.io/qr"

	"bytes"

	"fyne.io/systray"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

//go:embed icon.ico
var iconData []byte

// globals so the tray can shut the bridge down cleanly (onExit)
var (
	globalClient *whatsmeow.Client
	globalStore  *MessageStore
	mStatus      *systray.MenuItem // tray Status item (kept live)
	connectMu    sync.Mutex        // serialises (re)connection and guards qrActive
	qrActive     bool              // a QR pairing flow is already running
)

// Message represents a chat message for our client
type Message struct {
	Time      time.Time
	Sender    string
	Content   string
	IsFromMe  bool
	MediaType string
	Filename  string
}

// Database handler for storing message history
type MessageStore struct {
	db *sql.DB
}

// Initialize message store
func NewMessageStore() (*MessageStore, error) {
	// Create directory for database if it doesn't exist
	if err := os.MkdirAll("store", 0755); err != nil {
		return nil, fmt.Errorf("failed to create store directory: %v", err)
	}

	// Open SQLite database for messages
	db, err := sql.Open("sqlite3", "file:store/messages.db?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open message database: %v", err)
	}

	// Create tables if they don't exist
	_, err = db.Exec(`
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
			quoted_id TEXT,
			quoted_text TEXT,
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
		);

		-- O protobuf cru do menu recebido. Parece redundante, mas nao e': a resposta a uma
		-- opcao tem que CITAR a mensagem original inteira dentro do ContextInfo. Sem o
		-- QuotedMessage o servidor recusa o stanza com erro 479, "smax-invalid". Guardar so' o
		-- texto nao basta, porque o que vai no ContextInfo e' o proto.
		CREATE TABLE IF NOT EXISTS message_raw (
			message_id TEXT,
			chat_jid TEXT,
			proto BLOB,
			PRIMARY KEY (message_id, chat_jid)
		);

		-- Opcoes clicaveis dos menus interativos (lista, botao, template, native flow).
		-- Sem isto nao da' para RESPONDER um bot: ele espera o ID da opcao, nao o rotulo.
		CREATE TABLE IF NOT EXISTS message_options (
			message_id TEXT,
			chat_jid TEXT,
			idx INTEGER,
			kind TEXT,
			option_id TEXT,
			title TEXT,
			description TEXT,
			section TEXT,
			params_json TEXT,
			PRIMARY KEY (message_id, chat_jid, idx)
		);
	`)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to create tables: %v", err)
	}

	// Migracao para bancos que ja existiam antes das colunas de citacao. CREATE TABLE IF NOT
	// EXISTS nao altera tabela ja criada, entao sem isto o banco antigo continuaria sem elas.
	// O erro de "duplicate column name" e o caso normal em quem ja migrou, e por isso e ignorado.
	for _, col := range []string{"quoted_id TEXT", "quoted_text TEXT"} {
		if _, e := db.Exec("ALTER TABLE messages ADD COLUMN " + col); e != nil &&
			!strings.Contains(e.Error(), "duplicate column") {
			fmt.Printf("aviso: nao consegui adicionar %s: %v\n", col, e)
		}
	}

	return &MessageStore{db: db}, nil
}

// Close the database connection
func (store *MessageStore) Close() error {
	return store.db.Close()
}

// Store a chat in the database
func (store *MessageStore) StoreChat(jid, name string, lastMessageTime time.Time) error {
	_, err := store.db.Exec(
		"INSERT OR REPLACE INTO chats (jid, name, last_message_time) VALUES (?, ?, ?)",
		jid, name, lastMessageTime,
	)
	return err
}

// EnsureChat cria a linha do chat SO se ela ainda nao existir.
//
// Diferente do StoreChat, que e INSERT OR REPLACE e sobrescreveria o nome por vazio,
// este aqui preserva o que ja esta la. Serve para o caminho de ENVIO: sem a linha em
// chats, o INSERT em messages morre com FOREIGN KEY constraint failed e a mensagem
// enviada nao fica registrada em lugar nenhum.
func (store *MessageStore) EnsureChat(jid string, lastMessageTime time.Time) error {
	_, err := store.db.Exec(
		"INSERT INTO chats (jid, name, last_message_time) VALUES (?, '', ?) "+
			"ON CONFLICT(jid) DO NOTHING",
		jid, lastMessageTime,
	)
	return err
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// StoreLabel upserts a WhatsApp Business label (e.g. "Work")
func (store *MessageStore) StoreLabel(labelID, name string, color int, deleted bool) error {
	_, err := store.db.Exec(
		"INSERT OR REPLACE INTO labels (label_id, name, color, deleted) VALUES (?, ?, ?, ?)",
		labelID, name, color, boolToInt(deleted),
	)
	return err
}

// StoreLabelChat upserts the label <-> chat association
func (store *MessageStore) StoreLabelChat(labelID, chatJID string, labeled bool) error {
	_, err := store.db.Exec(
		"INSERT OR REPLACE INTO label_chats (label_id, chat_jid, labeled) VALUES (?, ?, ?)",
		labelID, chatJID, boolToInt(labeled),
	)
	return err
}

// Store a message in the database
func (store *MessageStore) StoreMessage(id, chatJID, sender, content string, timestamp time.Time, isFromMe bool,
	mediaType, filename, url string, mediaKey, fileSHA256, fileEncSHA256 []byte, fileLength uint64,
	quotedID, quotedText string) error {
	// Only store if there's actual content or media
	if content == "" && mediaType == "" {
		return nil
	}

	_, err := store.db.Exec(
		`INSERT OR REPLACE INTO messages
		(id, chat_jid, sender, content, timestamp, is_from_me, media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length, quoted_id, quoted_text)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, chatJID, sender, content, timestamp, isFromMe, mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength, quotedID, quotedText,
	)
	return err
}

// Store the real direct_path (from proto) for a media message
func (store *MessageStore) StoreDirectPath(id, chatJID, directPath string) error {
	if directPath == "" {
		return nil
	}
	_, err := store.db.Exec(
		"UPDATE messages SET direct_path = ? WHERE id = ? AND chat_jid = ?",
		directPath, id, chatJID,
	)
	return err
}

// Get stored direct_path for a media message
func (store *MessageStore) GetDirectPath(id, chatJID string) string {
	var dp sql.NullString
	store.db.QueryRow("SELECT direct_path FROM messages WHERE id = ? AND chat_jid = ?", id, chatJID).Scan(&dp)
	if dp.Valid {
		return dp.String
	}
	return ""
}

// telefonesDoVcard tira os numeros de um vCard de contato compartilhado.
//
// Antes de 02/09/2026 o bridge guardava so' "[contact] Fulano" e o TELEFONE SE PERDIA. Quando
// o Marcio mandou o contato do locador de plataforma de Piumhi, o numero nao estava em lugar
// nenhum do banco e teve que ser pedido de novo. O nome sozinho nao serve para nada: o que se
// faz com um contato compartilhado e' ligar para ele.
//
// O vCard traz o telefone em linhas do tipo:
//
//	TEL;type=CELL;waid=553799830144:+55 37 9983-0144
//
// O waid e' o que interessa, porque ja' vem no formato do WhatsApp. Quando nao ha' waid, cai
// para o numero cru depois dos dois-pontos.
func telefonesDoVcard(vcard string) []string {
	vistos := map[string]bool{}
	nums := []string{}
	for _, linha := range strings.Split(vcard, "\n") {
		linha = strings.TrimSpace(linha)
		if !strings.HasPrefix(strings.ToUpper(linha), "TEL") {
			continue
		}
		n := ""
		if i := strings.Index(strings.ToLower(linha), "waid="); i >= 0 {
			resto := linha[i+len("waid="):]
			if j := strings.IndexAny(resto, ":;"); j >= 0 {
				n = resto[:j]
			} else {
				n = resto
			}
		}
		if n == "" {
			if i := strings.LastIndex(linha, ":"); i >= 0 {
				n = linha[i+1:]
			}
		}
		// deixa so' digito e o + inicial, que e' o que serve para discar ou montar JID
		limpo := strings.Builder{}
		for k, r := range strings.TrimSpace(n) {
			if r >= '0' && r <= '9' || (k == 0 && r == '+') {
				limpo.WriteRune(r)
			}
		}
		if s := limpo.String(); len(s) >= 8 && !vistos[s] {
			vistos[s] = true
			nums = append(nums, s)
		}
	}
	return nums
}

// formataContato junta o nome exibido com os telefones achados no vCard.
func formataContato(nome, vcard string) string {
	s := "[contact] " + nome
	if tels := telefonesDoVcard(vcard); len(tels) > 0 {
		s += " - " + strings.Join(tels, ", ")
	}
	return s
}

// Extract the real direct path from a message proto (any media type)
func extractDirectPath(msg *waProto.Message) string {
	msg = unwrapMessage(msg)
	if msg == nil {
		return ""
	}
	if m := msg.GetAudioMessage(); m != nil {
		return m.GetDirectPath()
	}
	if m := msg.GetImageMessage(); m != nil {
		return m.GetDirectPath()
	}
	if m := msg.GetVideoMessage(); m != nil {
		return m.GetDirectPath()
	}
	if m := msg.GetDocumentMessage(); m != nil {
		return m.GetDirectPath()
	}
	if m := msg.GetStickerMessage(); m != nil {
		return m.GetDirectPath()
	}
	if m := msg.GetPtvMessage(); m != nil {
		return m.GetDirectPath()
	}
	return ""
}

// Get messages from a chat
func (store *MessageStore) GetMessages(chatJID string, limit int) ([]Message, error) {
	rows, err := store.db.Query(
		"SELECT sender, content, timestamp, is_from_me, media_type, filename FROM messages WHERE chat_jid = ? ORDER BY timestamp DESC LIMIT ?",
		chatJID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var msg Message
		var timestamp time.Time
		err := rows.Scan(&msg.Sender, &msg.Content, &timestamp, &msg.IsFromMe, &msg.MediaType, &msg.Filename)
		if err != nil {
			return nil, err
		}
		msg.Time = timestamp
		messages = append(messages, msg)
	}

	return messages, nil
}

// Get all chats
func (store *MessageStore) GetChats() (map[string]time.Time, error) {
	rows, err := store.db.Query("SELECT jid, last_message_time FROM chats ORDER BY last_message_time DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	chats := make(map[string]time.Time)
	for rows.Next() {
		var jid string
		var lastMessageTime time.Time
		err := rows.Scan(&jid, &lastMessageTime)
		if err != nil {
			return nil, err
		}
		chats[jid] = lastMessageTime
	}

	return chats, nil
}

// Extract text content from a message
// WhatsApp wraps the real message inside another one in several cases: documents sent with
// a caption, disappearing (ephemeral) messages, view-once media and edited messages. Without
// unwrapping, the bridge sees nothing at all: the whole message is lost, not just its text.
func unwrapMessage(msg *waProto.Message) *waProto.Message {
	for i := 0; i < 5 && msg != nil; i++ {
		switch {
		case msg.GetEphemeralMessage().GetMessage() != nil:
			msg = msg.GetEphemeralMessage().GetMessage()
		case msg.GetViewOnceMessage().GetMessage() != nil:
			msg = msg.GetViewOnceMessage().GetMessage()
		case msg.GetViewOnceMessageV2().GetMessage() != nil:
			msg = msg.GetViewOnceMessageV2().GetMessage()
		case msg.GetViewOnceMessageV2Extension().GetMessage() != nil:
			msg = msg.GetViewOnceMessageV2Extension().GetMessage()
		case msg.GetDocumentWithCaptionMessage().GetMessage() != nil:
			msg = msg.GetDocumentWithCaptionMessage().GetMessage()
		case msg.GetEditedMessage().GetMessage() != nil:
			msg = msg.GetEditedMessage().GetMessage()
		default:
			return msg
		}
	}
	return msg
}

// extractQuoted devolve o ID e o texto da mensagem CITADA, quando o remetente respondeu a uma
// mensagem especifica em vez de escrever solto.
//
// Sem isto o "Aqui" do peao chega sem contexto: ele respondeu a uma pergunta minha e eu nao sei
// a qual. Gabriel apontou em 04/09/2026: *"no caso ele marcou uma mensagem, deveria ter isso no
// sistema"*. Quem trabalha em obra responde citando, porque recebe varias perguntas de uma vez;
// perder a citacao e perder metade do sentido.
//
// O ContextInfo vive em cada tipo de mensagem, nao num campo comum, entao tem que ser procurado
// um por um. O texto citado sai do proprio extractTextContent, que ja sabe ler legenda de midia.
func extractQuoted(msg *waProto.Message) (string, string) {
	msg = unwrapMessage(msg)
	if msg == nil {
		return "", ""
	}
	var ctx *waProto.ContextInfo
	switch {
	case msg.GetExtendedTextMessage().GetContextInfo() != nil:
		ctx = msg.GetExtendedTextMessage().GetContextInfo()
	case msg.GetImageMessage().GetContextInfo() != nil:
		ctx = msg.GetImageMessage().GetContextInfo()
	case msg.GetVideoMessage().GetContextInfo() != nil:
		ctx = msg.GetVideoMessage().GetContextInfo()
	case msg.GetAudioMessage().GetContextInfo() != nil:
		ctx = msg.GetAudioMessage().GetContextInfo()
	case msg.GetDocumentMessage().GetContextInfo() != nil:
		ctx = msg.GetDocumentMessage().GetContextInfo()
	case msg.GetStickerMessage().GetContextInfo() != nil:
		ctx = msg.GetStickerMessage().GetContextInfo()
	}
	if ctx == nil || ctx.GetStanzaID() == "" {
		return "", ""
	}
	return ctx.GetStanzaID(), extractTextContent(ctx.GetQuotedMessage())
}

func extractTextContent(msg *waProto.Message) string {
	msg = unwrapMessage(msg)
	if msg == nil {
		return ""
	}

	// Try to get text content
	if text := msg.GetConversation(); text != "" {
		return text
	} else if extendedText := msg.GetExtendedTextMessage(); extendedText != nil {
		return extendedText.GetText()
	}

	// Media captions. Without this, a captioned photo is stored with empty content and
	// whatever was written in the caption is lost.
	if img := msg.GetImageMessage(); img != nil {
		return img.GetCaption()
	} else if vid := msg.GetVideoMessage(); vid != nil {
		return vid.GetCaption()
	} else if doc := msg.GetDocumentMessage(); doc != nil {
		if c := doc.GetCaption(); c != "" {
			return c
		}
		return doc.GetTitle()
	}

	// Non-media message types that still carry meaningful content
	if loc := msg.GetLocationMessage(); loc != nil {
		s := fmt.Sprintf("[location] %f, %f", loc.GetDegreesLatitude(), loc.GetDegreesLongitude())
		if n := loc.GetName(); n != "" {
			s += " - " + n
		}
		if a := loc.GetAddress(); a != "" {
			s += " (" + a + ")"
		}
		return s
	} else if live := msg.GetLiveLocationMessage(); live != nil {
		return fmt.Sprintf("[live location] %f, %f %s",
			live.GetDegreesLatitude(), live.GetDegreesLongitude(), live.GetCaption())
	} else if ct := msg.GetContactMessage(); ct != nil {
		return formataContato(ct.GetDisplayName(), ct.GetVcard())
	} else if cts := msg.GetContactsArrayMessage(); cts != nil {
		partes := make([]string, 0, len(cts.GetContacts()))
		for _, c := range cts.GetContacts() {
			partes = append(partes, formataContato(c.GetDisplayName(), c.GetVcard()))
		}
		return fmt.Sprintf("[%d contacts] %s | %s",
			len(cts.GetContacts()), cts.GetDisplayName(), strings.Join(partes, " ; "))
	} else if poll := msg.GetPollCreationMessage(); poll != nil {
		return "[poll] " + poll.GetName()
	} else if p3 := msg.GetPollCreationMessageV3(); p3 != nil {
		return "[poll] " + p3.GetName()
	} else if btn := msg.GetButtonsResponseMessage(); btn != nil {
		return btn.GetSelectedDisplayText()
	} else if lst := msg.GetListResponseMessage(); lst != nil {
		return lst.GetTitle()
	} else if tpl := msg.GetTemplateButtonReplyMessage(); tpl != nil {
		return tpl.GetSelectedDisplayText()
	}

	// MENU INTERATIVO. Lista, botoes, template e native flow chegam sem texto nenhum, e ate'
	// 12/09/2026 o bridge gravava content vazio: do lado de ca' o bot parecia ter travado, e
	// foi exatamente o que aconteceu com o atendimento da Localiza. Aqui o menu vira texto
	// numerado, e as opcoes com seus IDs sao gravadas em message_options por handleMessage.
	if opts := extractMenuOptions(msg); len(opts) > 0 {
		return renderMenu(msg, opts)
	}

	// Native WhatsApp events. This is the gap behind lharries/whatsapp-mcp#310: the event is
	// created in the group but reads return nothing, so clients fall back to a plain-text
	// summary. Without handling EventMessage, the event does not exist for the bridge.
	if ev := msg.GetEventMessage(); ev != nil {
		s := "[event] " + ev.GetName()
		if ev.GetIsCanceled() {
			s = "[event CANCELLED] " + ev.GetName()
		}
		if t := ev.GetStartTime(); t != 0 {
			s += " starts " + time.Unix(t, 0).Format("02/01/2006 15:04")
		}
		if t := ev.GetEndTime(); t != 0 {
			s += " ends " + time.Unix(t, 0).Format("02/01/2006 15:04")
		}
		if loc := ev.GetLocation().GetName(); loc != "" {
			s += " - " + loc
		}
		if d := ev.GetDescription(); d != "" {
			s += " | " + d
		}
		return s
	} else if inv := msg.GetEventInviteMessage(); inv != nil {
		return "[event invite]"
	}

	// Faithful replica: record the types that carry no text of their own, so the conversation
	// has no gaps when someone reacts, deletes a message or sends a sticker.
	if al := msg.GetAlbumMessage(); al != nil {
		return fmt.Sprintf("[album] %d images, %d videos",
			al.GetExpectedImageCount(), al.GetExpectedVideoCount())
	} else if ord := msg.GetOrderMessage(); ord != nil {
		return fmt.Sprintf("[order] %s (%d items)", ord.GetOrderTitle(), ord.GetItemCount())
	} else if pin := msg.GetPinInChatMessage(); pin != nil {
		return "[pinned message]"
	} else if st := msg.GetStickerMessage(); st != nil {
		return "[sticker]"
	} else if r := msg.GetReactionMessage(); r != nil {
		target := ""
		if k := r.GetKey(); k != nil {
			target = k.GetID()
		}
		return fmt.Sprintf("[reaction %s to %s]", r.GetText(), target)
	} else if ptv := msg.GetPtvMessage(); ptv != nil {
		return "[round video] " + ptv.GetCaption()
	} else if gi := msg.GetGroupInviteMessage(); gi != nil {
		return "[group invite] " + gi.GetGroupName()
	} else if pu := msg.GetPollUpdateMessage(); pu != nil {
		return "[poll vote]"
	} else if p := msg.GetProtocolMessage(); p != nil {
		if p.GetType() == waProto.ProtocolMessage_REVOKE {
			target := ""
			if k := p.GetKey(); k != nil {
				target = k.GetID()
			}
			return "[deleted message] " + target
		}
	}

	return ""
}

// SendMessageResponse represents the response for the send message API
type SendMessageResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	// ID da mensagem que acabou de sair. Sem ele nao da' para apagar nem editar depois,
	// que era o buraco de antes de 02/09/2026.
	MessageID string `json:"message_id,omitempty"`
}

// RevokeRequest apaga (ou edita) uma mensagem que a gente mesmo mandou.
// Com NewText preenchido vira edicao; vazio, apaga para todo mundo.
type RevokeRequest struct {
	MessageID string `json:"message_id"`
	ChatJID   string `json:"chat_jid"`
	NewText   string `json:"new_text,omitempty"`
}

// SendMessageRequest represents the request body for the send message API
type SendMessageRequest struct {
	Recipient string `json:"recipient"`
	Message   string `json:"message"`
	MediaPath string `json:"media_path,omitempty"`
}

// Function to send a WhatsApp message
func sendWhatsAppMessage(client *whatsmeow.Client, recipient string, message string, mediaPath string) (bool, string) {
	if !client.IsConnected() {
		return false, "Not connected to WhatsApp"
	}

	// Create JID for recipient
	var recipientJID types.JID
	var err error

	// Check if recipient is a JID
	isJID := strings.Contains(recipient, "@")

	if isJID {
		// Parse the JID string
		recipientJID, err = types.ParseJID(recipient)
		if err != nil {
			return false, fmt.Sprintf("Error parsing JID: %v", err)
		}
	} else {
		// Create JID from phone number
		recipientJID = types.JID{
			User:   recipient,
			Server: "s.whatsapp.net", // For personal chats
		}
	}

	msg := &waProto.Message{}

	// Check if we have media to send
	if mediaPath != "" {
		// Read media file
		mediaData, err := os.ReadFile(mediaPath)
		if err != nil {
			return false, fmt.Sprintf("Error reading media file: %v", err)
		}

		// Determine media type and mime type based on file extension
		fileExt := strings.ToLower(mediaPath[strings.LastIndex(mediaPath, ".")+1:])
		var mediaType whatsmeow.MediaType
		var mimeType string

		// Handle different media types
		switch fileExt {
		// Image types
		case "jpg", "jpeg":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/jpeg"
		case "png":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/png"
		case "gif":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/gif"
		case "webp":
			mediaType = whatsmeow.MediaImage
			mimeType = "image/webp"

		// Audio types
		case "ogg":
			mediaType = whatsmeow.MediaAudio
			mimeType = "audio/ogg; codecs=opus"

		// Video types
		case "mp4":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/mp4"
		case "avi":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/avi"
		case "mov":
			mediaType = whatsmeow.MediaVideo
			mimeType = "video/quicktime"

		// Document types (correct mimetype so WhatsApp can render a preview)
		case "pdf":
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/pdf"
		case "xlsx":
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
		case "xls":
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/vnd.ms-excel"
		case "docx":
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
		case "doc":
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/msword"

		// any other file type
		default:
			mediaType = whatsmeow.MediaDocument
			mimeType = "application/octet-stream"
		}

		// Upload media to WhatsApp servers
		resp, err := client.Upload(context.Background(), mediaData, mediaType)
		if err != nil {
			return false, fmt.Sprintf("Error uploading media: %v", err)
		}

		fmt.Println("Media uploaded", resp)

		// Create the appropriate message type based on media type
		switch mediaType {
		case whatsmeow.MediaImage:
			msg.ImageMessage = &waProto.ImageMessage{
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case whatsmeow.MediaAudio:
			// Handle ogg audio files
			var seconds uint32 = 30 // Default fallback
			var waveform []byte = nil

			// Try to analyze the ogg file
			if strings.Contains(mimeType, "ogg") {
				analyzedSeconds, analyzedWaveform, err := analyzeOggOpus(mediaData)
				if err == nil {
					seconds = analyzedSeconds
					waveform = analyzedWaveform
				} else {
					return false, fmt.Sprintf("Failed to analyze Ogg Opus file: %v", err)
				}
			} else {
				fmt.Printf("Not an Ogg Opus file: %s\n", mimeType)
			}

			msg.AudioMessage = &waProto.AudioMessage{
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
				Seconds:       proto.Uint32(seconds),
				PTT:           proto.Bool(true),
				Waveform:      waveform,
			}
		case whatsmeow.MediaVideo:
			msg.VideoMessage = &waProto.VideoMessage{
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		case whatsmeow.MediaDocument:
			fileName := filepath.Base(mediaPath) // handles both / and \ (Windows); avoids "Untitled"
			msg.DocumentMessage = &waProto.DocumentMessage{
				Title:         proto.String(fileName),
				FileName:      proto.String(fileName),
				Caption:       proto.String(message),
				Mimetype:      proto.String(mimeType),
				URL:           &resp.URL,
				DirectPath:    &resp.DirectPath,
				MediaKey:      resp.MediaKey,
				FileEncSHA256: resp.FileEncSHA256,
				FileSHA256:    resp.FileSHA256,
				FileLength:    &resp.FileLength,
			}
		}
	} else {
		msg.Conversation = proto.String(message)
	}

	// Send message
	resp, err := client.SendMessage(context.Background(), recipientJID, msg)

	if err != nil {
		return false, fmt.Sprintf("Error sending message: %v", err)
	}

	// Guarda o que a gente mesmo mandou, com o ID que o servidor devolveu.
	//
	// Ate' 02/09/2026 o bridge so' gravava mensagem RECEBIDA, porque o StoreMessage vive nos
	// handlers de evento. O envio proprio nao entrava, e isso custava duas coisas: nao dava
	// para conferir se algo saiu, e principalmente **nao dava para APAGAR nem EDITAR**, porque
	// as duas operacoes exigem o ID. Foi o que travou quando o Gabriel pediu para apagar uma
	// pergunta ja' enviada ao locador de plataforma.
	//
	// Falha aqui NAO derruba o envio: a mensagem ja' foi, o banco e' so' registro.
	if globalStore != nil && resp.ID != "" {
		conteudo := message
		if conteudo == "" && mediaPath != "" {
			conteudo = "[enviado] " + filepath.Base(mediaPath)
		}
		// Contato novo ainda nao tem linha em chats, e sem ela a FK derruba o INSERT.
		if e := globalStore.EnsureChat(recipientJID.String(), resp.Timestamp); e != nil {
			fmt.Printf("[bridge] nao criou o chat %s: %v\n", recipientJID.String(), e)
		}
		if e := globalStore.StoreMessage(resp.ID, recipientJID.String(), "", conteudo,
			resp.Timestamp, true, "", "", "", nil, nil, nil, 0, "", ""); e != nil {
			fmt.Printf("[bridge] enviou mas nao gravou no banco: %v\n", e)
		}
	}

	return true, resp.ID
}

// shortID trims a WhatsApp message ID to a filename-safe suffix.
func shortID(id string) string {
	id = strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			return r
		}
		return -1
	}, id)
	if len(id) > 8 {
		return id[len(id)-8:]
	}
	if id == "" {
		return "0"
	}
	return id
}

// uniqueName binds a stored media filename to its message, leaving names that already carry
// the ID untouched so previously downloaded files keep their path.
func uniqueName(filename, messageID string) string {
	short := shortID(messageID)
	if short == "" || strings.Contains(filename, short) {
		return filename
	}
	ext := filepath.Ext(filename)
	return strings.TrimSuffix(filename, ext) + "_" + short + ext
}

// Media metadata needed to fetch the file later: WhatsApp serves media by reference
// (URL plus decryption key), never inline.
//
// The filename is derived from the message itself, not from the wall clock. Naming by
// time.Now() with second resolution collides during a history sync, where hundreds of
// messages are processed in the same second: two different photos would land on the same
// path, and downloadMedia returns an existing file as a success, so the second message
// would silently serve the first one's bytes.
func extractMediaInfo(msg *waProto.Message, stamp time.Time, id string) (mediaType string, filename string, url string, mediaKey []byte, fileSHA256 []byte, fileEncSHA256 []byte, fileLength uint64) {
	msg = unwrapMessage(msg)
	if msg == nil {
		return "", "", "", nil, nil, nil, 0
	}
	// message ID keeps the name unique even for messages sharing a timestamp
	base := stamp.Format("20060102_150405") + "_" + shortID(id)

	// Check for image message
	if img := msg.GetImageMessage(); img != nil {
		return "image", "image_" + base + ".jpg",
			img.GetURL(), img.GetMediaKey(), img.GetFileSHA256(), img.GetFileEncSHA256(), img.GetFileLength()
	}

	// Check for video message
	if vid := msg.GetVideoMessage(); vid != nil {
		return "video", "video_" + base + ".mp4",
			vid.GetURL(), vid.GetMediaKey(), vid.GetFileSHA256(), vid.GetFileEncSHA256(), vid.GetFileLength()
	}

	// Check for audio message
	if aud := msg.GetAudioMessage(); aud != nil {
		return "audio", "audio_" + base + ".ogg",
			aud.GetURL(), aud.GetMediaKey(), aud.GetFileSHA256(), aud.GetFileEncSHA256(), aud.GetFileLength()
	}

	// Check for document message
	if doc := msg.GetDocumentMessage(); doc != nil {
		filename := doc.GetFileName()
		if filename == "" {
			filename = "document_" + base
		}
		return "document", filename,
			doc.GetURL(), doc.GetMediaKey(), doc.GetFileSHA256(), doc.GetFileEncSHA256(), doc.GetFileLength()
	}

	// Stickers and round videos are downloadable media too
	if st := msg.GetStickerMessage(); st != nil {
		return "sticker", "sticker_" + base + ".webp",
			st.GetURL(), st.GetMediaKey(), st.GetFileSHA256(), st.GetFileEncSHA256(), st.GetFileLength()
	}
	if ptv := msg.GetPtvMessage(); ptv != nil {
		return "video", "ptv_" + base + ".mp4",
			ptv.GetURL(), ptv.GetMediaKey(), ptv.GetFileSHA256(), ptv.GetFileEncSHA256(), ptv.GetFileLength()
	}

	return "", "", "", nil, nil, nil, 0
}

// Handle regular incoming messages with media support
func handleMessage(client *whatsmeow.Client, messageStore *MessageStore, msg *events.Message, logger waLog.Logger) {
	// Save message to database
	chatJID := msg.Info.Chat.String()
	sender := msg.Info.Sender.User

	// Get appropriate chat name (pass nil for conversation since we don't have one for regular messages)
	name := GetChatName(client, messageStore, msg.Info.Chat, chatJID, nil, sender, logger)

	// Update chat in database with the message timestamp (keeps last message time updated)
	err := messageStore.StoreChat(chatJID, name, msg.Info.Timestamp)
	if err != nil {
		logger.Warnf("Failed to store chat: %v", err)
	}

	// Extract text content
	content := extractTextContent(msg.Message)

	// Extract media info
	mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength := extractMediaInfo(msg.Message, msg.Info.Timestamp, msg.Info.ID)

	// Quem responde citando esta dizendo A QUAL pergunta responde; sem isto o "Aqui" chega solto
	quotedID, quotedText := extractQuoted(msg.Message)

	// Skip if there's no content and no media
	if content == "" && mediaType == "" {
		return
	}

	// Store message in database
	err = messageStore.StoreMessage(
		msg.Info.ID,
		chatJID,
		sender,
		content,
		msg.Info.Timestamp,
		msg.Info.IsFromMe,
		mediaType,
		filename,
		url,
		mediaKey,
		fileSHA256,
		fileEncSHA256,
		fileLength,
		quotedID,
		quotedText,
	)

	if err != nil {
		logger.Warnf("Failed to store message: %v", err)
	} else {
		// Guarda as opcoes clicaveis do menu, com os IDs que o bot espera de volta.
		// Falha aqui nao derruba nada: a mensagem ja' foi gravada, isto e' so' o indice das
		// opcoes para o /api/select-option poder escolher por numero depois.
		if opts := extractMenuOptions(msg.Message); len(opts) > 0 {
			if e := messageStore.StoreMenuOptions(msg.Info.ID, chatJID, opts); e != nil {
				logger.Warnf("Failed to store menu options: %v", e)
			} else {
				logger.Infof("Menu com %d opcoes guardado (%s)", len(opts), msg.Info.ID)
			}
			// O proto cru tambem, porque a resposta precisa CITAR a mensagem original
			// inteira. Sem isso o servidor recusa com 479, "smax-invalid".
			if e := messageStore.StoreRawMessage(msg.Info.ID, chatJID, msg.Message); e != nil {
				logger.Warnf("Failed to store raw menu proto: %v", e)
			}
		}
		// Store the real direct_path from the proto (needed for reliable media download)
		if mediaType != "" {
			if dp := extractDirectPath(msg.Message); dp != "" {
				if derr := messageStore.StoreDirectPath(msg.Info.ID, chatJID, dp); derr != nil {
					logger.Warnf("Failed to store direct_path: %v", derr)
				}
			}
		}
		// (per-message logging removed: it was the main cause of a runaway bridge.log)
	}
}

// DownloadMediaRequest represents the request body for the download media API
type DownloadMediaRequest struct {
	MessageID string `json:"message_id"`
	ChatJID   string `json:"chat_jid"`
}

// DownloadMediaResponse represents the response for the download media API
type DownloadMediaResponse struct {
	Success  bool   `json:"success"`
	Message  string `json:"message"`
	Filename string `json:"filename,omitempty"`
	Path     string `json:"path,omitempty"`
}

// Store additional media info in the database
func (store *MessageStore) StoreMediaInfo(id, chatJID, url string, mediaKey, fileSHA256, fileEncSHA256 []byte, fileLength uint64) error {
	_, err := store.db.Exec(
		"UPDATE messages SET url = ?, media_key = ?, file_sha256 = ?, file_enc_sha256 = ?, file_length = ? WHERE id = ? AND chat_jid = ?",
		url, mediaKey, fileSHA256, fileEncSHA256, fileLength, id, chatJID,
	)
	return err
}

// Get media info from the database
func (store *MessageStore) GetMediaInfo(id, chatJID string) (string, string, string, []byte, []byte, []byte, uint64, error) {
	var mediaType, filename, url string
	var mediaKey, fileSHA256, fileEncSHA256 []byte
	var fileLength uint64

	err := store.db.QueryRow(
		"SELECT media_type, filename, url, media_key, file_sha256, file_enc_sha256, file_length FROM messages WHERE id = ? AND chat_jid = ?",
		id, chatJID,
	).Scan(&mediaType, &filename, &url, &mediaKey, &fileSHA256, &fileEncSHA256, &fileLength)

	return mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength, err
}

// MediaDownloader implements the whatsmeow.DownloadableMessage interface
type MediaDownloader struct {
	URL           string
	DirectPath    string
	MediaKey      []byte
	FileLength    uint64
	FileSHA256    []byte
	FileEncSHA256 []byte
	MediaType     whatsmeow.MediaType
}

// GetDirectPath implements the DownloadableMessage interface
func (d *MediaDownloader) GetDirectPath() string {
	return d.DirectPath
}

// GetURL implements the DownloadableMessage interface
func (d *MediaDownloader) GetURL() string {
	return d.URL
}

// GetMediaKey implements the DownloadableMessage interface
func (d *MediaDownloader) GetMediaKey() []byte {
	return d.MediaKey
}

// GetFileLength implements the DownloadableMessage interface
func (d *MediaDownloader) GetFileLength() uint64 {
	return d.FileLength
}

// GetFileSHA256 implements the DownloadableMessage interface
func (d *MediaDownloader) GetFileSHA256() []byte {
	return d.FileSHA256
}

// GetFileEncSHA256 implements the DownloadableMessage interface
func (d *MediaDownloader) GetFileEncSHA256() []byte {
	return d.FileEncSHA256
}

// GetMediaType implements the DownloadableMessage interface
func (d *MediaDownloader) GetMediaType() whatsmeow.MediaType {
	return d.MediaType
}

// Function to download media from a message
func downloadMedia(client *whatsmeow.Client, messageStore *MessageStore, messageID, chatJID string) (bool, string, string, string, error) {
	// Query the database for the message
	var mediaType, filename, url string
	var mediaKey, fileSHA256, fileEncSHA256 []byte
	var fileLength uint64
	var err error

	// First, check if we already have this file
	chatDir := fmt.Sprintf("store/%s", strings.ReplaceAll(chatJID, ":", "_"))
	localPath := ""

	// Get media info from the database
	mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength, err = messageStore.GetMediaInfo(messageID, chatJID)

	if err != nil {
		// Try to get basic info if extended info isn't available
		err = messageStore.db.QueryRow(
			"SELECT media_type, filename FROM messages WHERE id = ? AND chat_jid = ?",
			messageID, chatJID,
		).Scan(&mediaType, &filename)

		if err != nil {
			return false, "", "", "", fmt.Errorf("failed to find message: %v", err)
		}
	}

	// Check if this is a media message
	if mediaType == "" {
		return false, "", "", "", fmt.Errorf("not a media message")
	}

	// Create directory for the chat if it doesn't exist
	if err := os.MkdirAll(chatDir, 0755); err != nil {
		return false, "", "", "", fmt.Errorf("failed to create chat directory: %v", err)
	}

	// Generate a local path for the file.
	//
	// The stored filename is only unique for messages saved after extractMediaInfo started
	// appending the message ID. Older rows still hold a name built from a second-resolution
	// timestamp, so a burst of photos sent in the same second all map to one path: the first
	// download wins and every later message is handed those same bytes, reported as success.
	// Binding the path to the message ID makes the cache hit below mean "this message was
	// already fetched" instead of "some message in that second was".
	localPath = fmt.Sprintf("%s/%s", chatDir, uniqueName(filename, messageID))

	// Get absolute path
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return false, "", "", "", fmt.Errorf("failed to get absolute path: %v", err)
	}

	// Check if file already exists
	if _, err := os.Stat(localPath); err == nil {
		// File exists, return it
		return true, mediaType, filename, absPath, nil
	}

	// If we don't have all the media info we need, we can't download
	if url == "" || len(mediaKey) == 0 || len(fileSHA256) == 0 || len(fileEncSHA256) == 0 || fileLength == 0 {
		return false, "", "", "", fmt.Errorf("incomplete media information for download")
	}

	fmt.Printf("Attempting to download media for message %s in chat %s...\n", messageID, chatJID)

	// Prefer the real direct_path stored from the proto; fall back to URL-derived hack
	directPath := messageStore.GetDirectPath(messageID, chatJID)
	if directPath == "" {
		directPath = extractDirectPathFromURL(url)
	}

	// Create a downloader that implements DownloadableMessage
	var waMediaType whatsmeow.MediaType
	switch mediaType {
	case "image":
		waMediaType = whatsmeow.MediaImage
	case "video":
		waMediaType = whatsmeow.MediaVideo
	case "audio":
		waMediaType = whatsmeow.MediaAudio
	case "document":
		waMediaType = whatsmeow.MediaDocument
	default:
		return false, "", "", "", fmt.Errorf("unsupported media type: %s", mediaType)
	}

	downloader := &MediaDownloader{
		URL:           url,
		DirectPath:    directPath,
		MediaKey:      mediaKey,
		FileLength:    fileLength,
		FileSHA256:    fileSHA256,
		FileEncSHA256: fileEncSHA256,
		MediaType:     waMediaType,
	}

	// Download the media using whatsmeow client
	mediaData, err := client.Download(context.Background(), downloader)
	if err != nil {
		return false, "", "", "", fmt.Errorf("failed to download media: %v", err)
	}

	// Save the downloaded media to file
	if err := os.WriteFile(localPath, mediaData, 0644); err != nil {
		return false, "", "", "", fmt.Errorf("failed to save media file: %v", err)
	}

	fmt.Printf("Successfully downloaded %s media to %s (%d bytes)\n", mediaType, absPath, len(mediaData))
	return true, mediaType, filename, absPath, nil
}

// Extract direct path from a WhatsApp media URL
func extractDirectPathFromURL(url string) string {
	// The direct path is typically in the URL, we need to extract it
	// Example URL: https://mmg.whatsapp.net/v/t62.7118-24/13812002_698058036224062_3424455886509161511_n.enc?ccb=11-4&oh=...

	// Find the path part after the domain
	parts := strings.SplitN(url, ".net/", 2)
	if len(parts) < 2 {
		return url // Return original URL if parsing fails
	}

	pathPart := parts[1]

	// Remove query parameters
	pathPart = strings.SplitN(pathPart, "?", 2)[0]

	// Create proper direct path format
	return "/" + pathPart
}

// Start a REST API server to expose the WhatsApp client functionality
func startRESTServer(client *whatsmeow.Client, messageStore *MessageStore, port int) {
	// grupo, participante e checagem de numero; ficam em grupos.go
	registrarRotasGrupo()

	// Handler for sending messages
	http.HandleFunc("/api/sync-labels", func(w http.ResponseWriter, r *http.Request) {
		c := globalClient
		if c == nil || c.Store.ID == nil {
			http.Error(w, "not connected", http.StatusServiceUnavailable)
			return
		}
		c.EmitAppStateEventsOnFullSync = true // else fullSync re-applies but does not re-emit events (label_edit/label_jid)
		err := c.FetchAppState(context.Background(), appstate.WAPatchRegular, true, false)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"success": true, "message": "label sync triggered (regular fullSync)"})
	})

	// Pareamento POR CODIGO, para quando nao da para escanear o QR (o dono esta fora).
	// Devolve os 8 caracteres que se digita no celular em Aparelhos conectados >
	// Conectar com numero de telefone. O codigo vale ~160s; se expirar, chame de novo.
	http.HandleFunc("/api/pair-phone", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Phone string `json:"phone"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if globalClient == nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"success": false, "message": "client is nil"})
			return
		}
		if globalClient.Store.ID != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"success": false,
				"message": "already paired; no code needed"})
			return
		}
		codigo, err := globalClient.PairPhone(context.Background(), req.Phone, true,
			whatsmeow.PairClientChrome, "Chrome (Windows)")
		if err != nil {
			fmt.Printf("[bridge] pair-phone falhou: %v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]any{"success": false, "message": err.Error()})
			return
		}
		fmt.Printf("[bridge] codigo de pareamento: %s\n", codigo)
		json.NewEncoder(w).Encode(map[string]any{"success": true, "code": codigo})
	})

	http.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		// Only allow POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the request body
		var req SendMessageRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}

		// Validate request
		if req.Recipient == "" {
			http.Error(w, "Recipient is required", http.StatusBadRequest)
			return
		}

		if req.Message == "" && req.MediaPath == "" {
			http.Error(w, "Message or media path is required", http.StatusBadRequest)
			return
		}

		fmt.Println("Received request to send message", req.Message, req.MediaPath)

		// Send the message. Em caso de sucesso o segundo retorno e' o ID da mensagem.
		success, message := sendWhatsAppMessage(client, req.Recipient, req.Message, req.MediaPath)
		fmt.Println("Message sent", success, message)
		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Set appropriate status code
		if !success {
			w.WriteHeader(http.StatusInternalServerError)
		}

		resp := SendMessageResponse{Success: success, Message: message}
		if success {
			// mantem a frase antiga para nao quebrar quem le' o campo message, e devolve o
			// ID em campo proprio, que e' o que serve para apagar ou editar depois
			resp.MessageID = message
			resp.Message = fmt.Sprintf("Message sent to %s", req.Recipient)
		}
		json.NewEncoder(w).Encode(resp)
	})

	// Clica numa opcao de menu interativo, que e' como bot de empresa conversa.
	//
	// Nasceu em 12/09/2026 no atendimento da Localiza: o menu deles chega como mensagem
	// interativa, o bridge gravava content vazio, e responder com o texto da opcao devolvia
	// "Desculpe, nao entendi". O bot espera o ID da opcao num tipo de mensagem proprio.
	//
	// Corpo: {"chat_jid": "...", "option": 2}. O message_id e' opcional, e sem ele vale o
	// ULTIMO menu recebido no chat, que e' o caso normal. Em vez do indice tambem aceita
	// "option_id" ou "title".
	http.HandleFunc("/api/select-option", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req SelectOptionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		id, titulo, err := selecionaOpcao(client, messageStore, req)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(SendMessageResponse{Success: false, Message: err.Error()})
			return
		}
		fmt.Printf("Opcao de menu escolhida: %q em %s\n", titulo, req.ChatJID)
		json.NewEncoder(w).Encode(SendMessageResponse{
			Success: true, MessageID: id,
			Message: fmt.Sprintf("Escolhido %q em %s", titulo, req.ChatJID),
		})
	})

	// Lista as opcoes do menu recebido, para saber o que da' para clicar antes de clicar.
	// GET /api/menu-options?chat_jid=...&message_id=... (message_id opcional)
	http.HandleFunc("/api/menu-options", func(w http.ResponseWriter, r *http.Request) {
		chatJID := r.URL.Query().Get("chat_jid")
		w.Header().Set("Content-Type", "application/json")
		if chatJID == "" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{"success": false,
				"message": "chat_jid e' obrigatorio"})
			return
		}
		id, opts, err := messageStore.GetMenuOptions(r.URL.Query().Get("message_id"), chatJID)
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]any{"success": false, "message": err.Error()})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"success": true,
			"message_id": id, "options": opts})
	})

	// Apaga para todos, ou edita, uma mensagem que a gente mesmo mandou.
	//
	// Gabriel, 02/09/2026: *"o bot precisa ter o id das suas proprias mensagens pra editar /
	// deletar"*. Sem o /api/send gravar o ID isto aqui nao teria como existir.
	http.HandleFunc("/api/revoke", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req RevokeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}
		if req.MessageID == "" || req.ChatJID == "" {
			http.Error(w, "message_id and chat_jid are required", http.StatusBadRequest)
			return
		}
		chat, err := types.ParseJID(req.ChatJID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error parsing chat JID: %v", err), http.StatusBadRequest)
			return
		}

		var acao string
		var msg *waProto.Message
		if req.NewText == "" {
			acao = "apagada"
			msg = client.BuildRevoke(chat, types.EmptyJID, req.MessageID)
		} else {
			acao = "editada"
			msg = client.BuildEdit(chat, req.MessageID,
				&waProto.Message{Conversation: proto.String(req.NewText)})
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := client.SendMessage(context.Background(), chat, msg); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(SendMessageResponse{
				Success: false, Message: fmt.Sprintf("Error: %v", err)})
			return
		}

		// espelha no banco o que aconteceu no WhatsApp, senao o registro mente
		if globalStore != nil {
			if req.NewText == "" {
				_, _ = globalStore.db.Exec(
					"DELETE FROM messages WHERE id = ? AND chat_jid = ?", req.MessageID, req.ChatJID)
			} else {
				_, _ = globalStore.db.Exec(
					"UPDATE messages SET content = ? WHERE id = ? AND chat_jid = ?",
					req.NewText, req.MessageID, req.ChatJID)
			}
		}
		json.NewEncoder(w).Encode(SendMessageResponse{
			Success: true, Message: fmt.Sprintf("Mensagem %s", acao), MessageID: req.MessageID})
	})

	// Handler for downloading media
	http.HandleFunc("/api/download", func(w http.ResponseWriter, r *http.Request) {
		// Only allow POST requests
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Parse the request body
		var req DownloadMediaRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request format", http.StatusBadRequest)
			return
		}

		// Validate request
		if req.MessageID == "" || req.ChatJID == "" {
			http.Error(w, "Message ID and Chat JID are required", http.StatusBadRequest)
			return
		}

		// Download the media
		success, mediaType, filename, path, err := downloadMedia(client, messageStore, req.MessageID, req.ChatJID)

		// Set response headers
		w.Header().Set("Content-Type", "application/json")

		// Handle download result
		if !success || err != nil {
			errMsg := "Unknown error"
			if err != nil {
				errMsg = err.Error()
			}

			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(DownloadMediaResponse{
				Success: false,
				Message: fmt.Sprintf("Failed to download media: %s", errMsg),
			})
			return
		}

		// Send successful response
		json.NewEncoder(w).Encode(DownloadMediaResponse{
			Success:  true,
			Message:  fmt.Sprintf("Successfully downloaded %s media", mediaType),
			Filename: filename,
			Path:     path,
		})
	})

	// Start the server
	serverAddr := fmt.Sprintf(":%d", port)
	fmt.Printf("Starting REST API server on %s...\n", serverAddr)

	// Run server in a goroutine so it doesn't block
	go func() {
		if err := http.ListenAndServe(serverAddr, nil); err != nil {
			fmt.Printf("REST API server error: %v\n", err)
		}
	}()
}

func main() {
	// cwd = the exe folder, since store/, bridge.log and the icon use relative paths
	if exe, err := os.Executable(); err == nil {
		os.Chdir(filepath.Dir(exe))
	}
	// a -H=windowsgui build has no console: stdout/stderr go to bridge.log
	rotateLog() // trim the log if a previous session left it oversized
	if f, ferr := os.OpenFile("bridge.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); ferr == nil {
		os.Stdout = f
		os.Stderr = f
	}
	// single instance: if port 8080 already answers, another bridge is running -> exit
	if c, derr := net.DialTimeout("tcp", "127.0.0.1:8080", time.Second); derr == nil {
		c.Close()
		return
	}
	// periodic trim: every 5 min, cut the log if it grew past the cap
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			rotateLog()
		}
	}()
	// systray.Run holds the main thread, so the bridge connects in the background (onReady)
	systray.Run(onReady, onExit)
}

func onReady() {
	systray.SetIcon(iconData)
	systray.SetTooltip("WhatsApp Bridge")
	mTitle := systray.AddMenuItem("WhatsApp Bridge", "")
	mTitle.Disable()
	mStatus = systray.AddMenuItem("Status: starting...", "WhatsApp connection state")
	mStatus.Disable()
	systray.AddSeparator()
	mConnect := systray.AddMenuItem("Connect / Reconnect", "Detects the current state and connects, showing a QR when logged out")
	mLog := systray.AddMenuItem("View log", "Open the bridge log")
	mFolder := systray.AddMenuItem("Open folder", "Open the bridge folder")
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit (stops the bridge)", "Stop the bridge")

	go runBridge() // connect in the background; the tray shows up immediately

	// graceful shutdown on a system signal (shutdown / taskkill)
	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		systray.Quit()
	}()

	// menu clicks
	go func() {
		for {
			select {
			case <-mConnect.ClickedCh:
				go triggerConnect() // detect the state and connect or reconnect, pairing by QR when needed
			case <-mLog.ClickedCh:
				showLogTail() // open only the tail (never hangs, even on a large log)
			case <-mFolder.ClickedCh:
				exec.Command("explorer", ".").Start()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}

// openPath opens a file or folder with the default Windows handler
func openPath(p string) {
	exec.Command("rundll32", "url.dll,FileProtocolHandler", p).Start()
}

const logCapBytes = 1 << 20 // 1 MB: past this the log is trimmed, previous contents kept in .1

// rotateLog keeps bridge.log small: past the cap it copies the current contents to
// bridge.log.1 and truncates the active one. Since the log is opened with O_APPEND, later
// writes resume from the start, safe even with the fd open (Windows included).
func rotateLog() {
	fi, err := os.Stat("bridge.log")
	if err != nil || fi.Size() < logCapBytes {
		return
	}
	if data, rerr := os.ReadFile("bridge.log"); rerr == nil {
		_ = os.WriteFile("bridge.log.1", data, 0644)
	}
	_ = os.Truncate("bridge.log", 0)
}

// showLogTail opens just the tail of the log (fast, never stalls the viewer).
func showLogTail() {
	const tailBytes = 64 * 1024
	data, err := os.ReadFile("bridge.log")
	if err != nil {
		openPath("bridge.log")
		return
	}
	if len(data) > tailBytes {
		data = data[len(data)-tailBytes:]
	}
	if werr := os.WriteFile("bridge-tail.log", data, 0644); werr != nil {
		openPath("bridge.log")
		return
	}
	openPath("bridge-tail.log")
}

// refreshStatus mirrors the real client state onto the tray Status item.
func refreshStatus() {
	if mStatus == nil {
		return
	}
	c := globalClient
	switch {
	case c != nil && c.Store.ID != nil && c.IsConnected():
		mStatus.SetTitle("Status: CONNECTED (" + c.Store.ID.User + ")")
		systray.SetTooltip("WhatsApp Bridge - connected")
	case c != nil && c.Store.ID != nil:
		mStatus.SetTitle("Status: disconnected, click Connect")
		systray.SetTooltip("WhatsApp Bridge - disconnected (session still valid)")
	default:
		mStatus.SetTitle("Status: LOGGED OUT, click Connect to pair")
		systray.SetTooltip("WhatsApp Bridge - logged out, scan the QR to pair")
	}
}

// statusLoop re-evaluates every 5s, so drops and recoveries are picked up on their own.
func statusLoop() {
	for {
		refreshStatus()
		time.Sleep(5 * time.Second)
	}
}

// reconnectLoop mantem a conexao de pe' SEM LIMITE de tentativas.
//
// Em 29/08/2026 as 03:20 o cabo de rede rompeu. O reconnect embutido tentou SEIS vezes,
// todas falhando com "lookup web.whatsapp.com: no such host", e parou de tentar. O bridge
// ficou 61 horas fora: as duas cobrancas automaticas do dia sairam com [FALHOU] em todos os
// peoes, tres dias de diaria nao chegaram e, como passou dias sem conectar, o WhatsApp
// derrubou o vinculo do aparelho. Foi preciso parear o QR de novo, e a midia daqueles dias
// se perdeu junto com as chaves.
//
// Uma queda de rede de madrugada nao pode custar isso. Aqui a tentativa nao tem teto: espera
// 30 s no comeco e vai dobrando ate 5 minutos, que segura o martelo no servidor e ainda assim
// volta sozinho poucos minutos depois de a rede normalizar.
//
// So' tenta quando a sessao ainda e' valida (Store.ID != nil). Deslogado precisa de QR, e QR
// depende do celular do Gabriel; nesse caso o loop fica quieto para nao gastar tentativa a toa.
func reconnectLoop(c *whatsmeow.Client) {
	const esperaMin, esperaMax = 30 * time.Second, 5 * time.Minute
	espera := esperaMin
	for {
		time.Sleep(espera)
		if c == nil || c.Store.ID == nil || c.IsConnected() {
			espera = esperaMin
			continue
		}
		agora := time.Now().Format("2006-01-02 15:04:05")
		fmt.Println(agora, "[bridge] fora do ar, tentando reconectar")
		if err := c.Connect(); err != nil {
			fmt.Printf("%s [bridge] reconexao falhou: %v (proxima tentativa em %s)\n",
				agora, err, espera)
			if espera < esperaMax {
				espera *= 2
				if espera > esperaMax {
					espera = esperaMax
				}
			}
			continue
		}
		fmt.Println(agora, "[bridge] reconectado")
		espera = esperaMin
	}
}

// doQRPair runs QR pairing: writes qr.png, opens it and waits for the scan. Used both at
// startup (when logged out) and by the tray's Connect button.
func doQRPair(client *whatsmeow.Client) {
	qrChan, err := client.GetQRChannel(context.Background())
	if err != nil {
		fmt.Printf("Failed to open the QR channel: %v\n", err)
		return
	}
	if err := client.Connect(); err != nil {
		fmt.Printf("Failed to connect for QR pairing: %v\n", err)
		return
	}
	opened := false // open the viewer only for the first code (the QR rotates every ~30s)
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			fmt.Println("\nScan this QR in WhatsApp > Linked devices:")
			qrterminal.GenerateHalfBlock(evt.Code, qrterminal.L, os.Stdout)
			if code, qerr := qr.Encode(evt.Code, qr.M); qerr == nil {
				if werr := os.WriteFile("qr.png", code.PNG(), 0644); werr == nil {
					if !opened {
						fmt.Println(">>> QR written to qr.png, opening it in the default viewer <<<")
						openPath("qr.png")
						opened = true
					}
				}
			}
		case "success":
			fmt.Println("\nConnected and authenticated.")
			os.Remove("qr.png")
			refreshStatus()
			return
		case "timeout":
			fmt.Println("QR expired. Click Connect again to generate a new one.")
			return
		case "error":
			fmt.Printf("QR pairing failed: %v\n", evt.Error)
			return
		}
	}
}

// triggerConnect is the single entry point for (re)connection, used by startup and by the
// Connect button. It inspects the state and acts: already connected -> nothing; valid session
// that dropped -> reconnect; logged out -> QR pairing (guarded by qrActive).
func triggerConnect() {
	connectMu.Lock()
	c := globalClient
	if c == nil {
		connectMu.Unlock()
		fmt.Println("Bridge is still starting up, try again in a few seconds.")
		return
	}

	// already connected and logged in
	if c.Store.ID != nil && c.IsConnected() {
		connectMu.Unlock()
		fmt.Println("WhatsApp is already connected.")
		refreshStatus()
		return
	}

	// valid session, just reconnect
	if c.Store.ID != nil {
		connectMu.Unlock()
		fmt.Println("Session still valid, reconnecting...")
		c.Disconnect()
		time.Sleep(300 * time.Millisecond)
		if err := c.Connect(); err != nil {
			fmt.Printf("Reconnect failed: %v\n", err)
		}
		time.Sleep(2 * time.Second)
		refreshStatus()
		return
	}

	// logged out: needs QR pairing
	if qrActive {
		connectMu.Unlock()
		fmt.Println("Pairing already in progress, see qr.png.")
		openPath("qr.png")
		return
	}
	qrActive = true
	connectMu.Unlock()

	go func() {
		c.Disconnect()
		time.Sleep(500 * time.Millisecond)
		doQRPair(c)
		connectMu.Lock()
		qrActive = false
		connectMu.Unlock()
		refreshStatus()
	}()
}

func onExit() {
	if globalClient != nil {
		globalClient.Disconnect()
	}
	if globalStore != nil {
		globalStore.Close()
	}
	os.Exit(0)
}

func runBridge() {
	// Set up logger - WARN silences the INFO spam ("Using existing chat name", per-message
	// for every message, which made bridge.log explode. WARN/ERROR are kept.
	logger := waLog.Stdout("Client", "WARN", true)
	logger.Infof("Starting WhatsApp client...")

	// Create database connection for storing session data
	dbLog := waLog.Stdout("Database", "WARN", true)

	// Create directory for database if it doesn't exist
	if err := os.MkdirAll("store", 0755); err != nil {
		logger.Errorf("Failed to create store directory: %v", err)
		return
	}

	container, err := sqlstore.New(context.Background(), "sqlite3", "file:store/whatsapp.db?_foreign_keys=on", dbLog)
	if err != nil {
		logger.Errorf("Failed to connect to database: %v", err)
		return
	}

	// Get device store - This contains session information
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		if err == sql.ErrNoRows {
			// No device exists, create one
			deviceStore = container.NewDevice()
			logger.Infof("Created new device")
		} else {
			logger.Errorf("Failed to get device: %v", err)
			return
		}
	}

	// Create client instance
	client := whatsmeow.NewClient(deviceStore, logger)
	if client == nil {
		logger.Errorf("Failed to create WhatsApp client")
		return
	}

	// Initialize message store
	messageStore, err := NewMessageStore()
	if err != nil {
		logger.Errorf("Failed to initialize message store: %v", err)
		return
	}
	// Close is handled by onExit (systray); runBridge returns early, so no defer here

	// Asynchronous processing: the whatsmeow handler only enqueues (cheap) and a single
	// worker drains the queue in order. This keeps the dispatcher from stalling on a large
	// history sync, which used to produce minutes-long "node handling taking long" warnings
	// and delay live messages. One worker also means no concurrent SQLite writers.
	eventQueue := make(chan interface{}, 4096)
	go func() {
		for evt := range eventQueue {
			switch v := evt.(type) {
			case *events.Message:
				handleMessage(client, messageStore, v, logger)
			case *events.HistorySync:
				handleHistorySync(client, messageStore, v, logger)
			}
		}
	}()

	client.AddEventHandler(func(evt interface{}) {
		switch v := evt.(type) {
		case *events.Message, *events.HistorySync:
			select {
			case eventQueue <- evt:
			default:
				logger.Warnf("Event queue full, dropping event")
			}
		case *events.LabelEdit:
			messageStore.StoreLabel(v.LabelID, v.Action.GetName(), int(v.Action.GetColor()), v.Action.GetDeleted())
		case *events.LabelAssociationChat:
			messageStore.StoreLabelChat(v.LabelID, v.JID.String(), v.Action.GetLabeled())
		case *events.Connected:
			logger.Infof("Connected to WhatsApp")
			fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "[bridge] CONNECTED")
			refreshStatus()
		case *events.Disconnected:
			logger.Warnf("Disconnected from WhatsApp")
			fmt.Println(time.Now().Format("2006-01-02 15:04:05"),
				"[bridge] DISCONNECTED, o reconnectLoop vai tentar de novo")
			refreshStatus()
		case *events.LoggedOut:
			logger.Warnf("Device logged out, please scan QR code to log in again")
			fmt.Println(time.Now().Format("2006-01-02 15:04:05"), "[bridge] LOGGED OUT, scan the QR to reconnect")
			refreshStatus()
		}
	})

	// set the globals early: the tray needs the client even while logged out
	globalClient = client
	globalStore = messageStore

	// Always start the REST server, even when logged out, so the tray and the API stay alive
	// if the connection drops. Handlers check IsConnected() on every call.
	startRESTServer(client, messageStore, 8080)

	// Refresh the tray Status item periodically.
	go statusLoop()

	// Vigia a conexao para sempre: sem isso, uma queda de rede longa derruba o bridge de vez.
	go reconnectLoop(client)

	// Unified connect path: valid session -> reconnect; logged out -> QR pairing. Same route
	// as the tray Connect button, guarded by qrActive.
	triggerConnect()

	fmt.Println("REST server is running (tray mode).")
}

// GetChatName determines the appropriate name for a chat based on JID and other info
func GetChatName(client *whatsmeow.Client, messageStore *MessageStore, jid types.JID, chatJID string, conversation interface{}, sender string, logger waLog.Logger) string {
	// First, check if chat already exists in database with a name
	var existingName string
	err := messageStore.db.QueryRow("SELECT name FROM chats WHERE jid = ?", chatJID).Scan(&existingName)
	if err == nil && existingName != "" {
		// Chat exists with a name, use that
		logger.Infof("Using existing chat name for %s: %s", chatJID, existingName)
		return existingName
	}

	// Need to determine chat name
	var name string

	if jid.Server == "g.us" {
		// This is a group chat
		logger.Infof("Getting name for group: %s", chatJID)

		// Use conversation data if provided (from history sync)
		if conversation != nil {
			// Extract name from conversation if available
			// This uses type assertions to handle different possible types
			var displayName, convName *string
			// Try to extract the fields we care about regardless of the exact type
			v := reflect.ValueOf(conversation)
			if v.Kind() == reflect.Ptr && !v.IsNil() {
				v = v.Elem()

				// Try to find DisplayName field
				if displayNameField := v.FieldByName("DisplayName"); displayNameField.IsValid() && displayNameField.Kind() == reflect.Ptr && !displayNameField.IsNil() {
					dn := displayNameField.Elem().String()
					displayName = &dn
				}

				// Try to find Name field
				if nameField := v.FieldByName("Name"); nameField.IsValid() && nameField.Kind() == reflect.Ptr && !nameField.IsNil() {
					n := nameField.Elem().String()
					convName = &n
				}
			}

			// Use the name we found
			if displayName != nil && *displayName != "" {
				name = *displayName
			} else if convName != nil && *convName != "" {
				name = *convName
			}
		}

		// If we didn't get a name, try group info
		if name == "" {
			groupInfo, err := client.GetGroupInfo(context.Background(), jid)
			if err == nil && groupInfo.Name != "" {
				name = groupInfo.Name
			} else {
				// Fallback name for groups
				name = fmt.Sprintf("Group %s", jid.User)
			}
		}

		logger.Infof("Using group name: %s", name)
	} else {
		// This is an individual contact
		logger.Infof("Getting name for contact: %s", chatJID)

		// Just use contact info (full name)
		contact, err := client.Store.Contacts.GetContact(context.Background(), jid)
		if err == nil && contact.FullName != "" {
			name = contact.FullName
		} else if sender != "" {
			// Fallback to sender
			name = sender
		} else {
			// Last fallback to JID
			name = jid.User
		}

		logger.Infof("Using contact name: %s", name)
	}

	return name
}

// Handle history sync events
func handleHistorySync(client *whatsmeow.Client, messageStore *MessageStore, historySync *events.HistorySync, logger waLog.Logger) {
	fmt.Printf("Received history sync event with %d conversations\n", len(historySync.Data.Conversations))

	syncedCount := 0
	for _, conversation := range historySync.Data.Conversations {
		// Parse JID from the conversation
		if conversation.ID == nil {
			continue
		}

		chatJID := *conversation.ID

		// Try to parse the JID
		jid, err := types.ParseJID(chatJID)
		if err != nil {
			logger.Warnf("Failed to parse JID %s: %v", chatJID, err)
			continue
		}

		// Get appropriate chat name by passing the history sync conversation directly
		name := GetChatName(client, messageStore, jid, chatJID, conversation, "", logger)

		// Process messages
		messages := conversation.Messages
		if len(messages) > 0 {
			// Update chat with latest message timestamp
			latestMsg := messages[0]
			if latestMsg == nil || latestMsg.Message == nil {
				continue
			}

			// Get timestamp from message info
			timestamp := time.Time{}
			if ts := latestMsg.Message.GetMessageTimestamp(); ts != 0 {
				timestamp = time.Unix(int64(ts), 0)
			} else {
				continue
			}

			messageStore.StoreChat(chatJID, name, timestamp)

			// Store messages
			for _, msg := range messages {
				if msg == nil || msg.Message == nil {
					continue
				}

				// Extract text content
				var content string
				if msg.Message.Message != nil {
					if conv := msg.Message.Message.GetConversation(); conv != "" {
						content = conv
					} else if ext := msg.Message.Message.GetExtendedTextMessage(); ext != nil {
						content = ext.GetText()
					}
				}

				// Extract media info
				var mediaType, filename, url string
				var mediaKey, fileSHA256, fileEncSHA256 []byte
				var fileLength uint64

				var quotedID, quotedText string
				if msg.Message.Message != nil {
					mediaType, filename, url, mediaKey, fileSHA256, fileEncSHA256, fileLength = extractMediaInfo(
						msg.Message.Message, time.Unix(int64(msg.Message.GetMessageTimestamp()), 0), msg.Message.Key.GetID())
					quotedID, quotedText = extractQuoted(msg.Message.Message)
				}

				// Log the message content for debugging
				logger.Infof("Message content: %v, Media Type: %v", content, mediaType)

				// Skip messages with no content and no media
				if content == "" && mediaType == "" {
					continue
				}

				// Determine sender
				var sender string
				isFromMe := false
				if msg.Message.Key != nil {
					if msg.Message.Key.FromMe != nil {
						isFromMe = *msg.Message.Key.FromMe
					}
					if !isFromMe && msg.Message.Key.Participant != nil && *msg.Message.Key.Participant != "" {
						sender = *msg.Message.Key.Participant
					} else if isFromMe {
						sender = client.Store.ID.User
					} else {
						sender = jid.User
					}
				} else {
					sender = jid.User
				}

				// Store message
				msgID := ""
				if msg.Message.Key != nil && msg.Message.Key.ID != nil {
					msgID = *msg.Message.Key.ID
				}

				// Get message timestamp
				timestamp := time.Time{}
				if ts := msg.Message.GetMessageTimestamp(); ts != 0 {
					timestamp = time.Unix(int64(ts), 0)
				} else {
					continue
				}

				err = messageStore.StoreMessage(
					msgID,
					chatJID,
					sender,
					content,
					timestamp,
					isFromMe,
					mediaType,
					filename,
					url,
					mediaKey,
					fileSHA256,
					fileEncSHA256,
					fileLength,
					quotedID,
					quotedText,
				)
				if err != nil {
					logger.Warnf("Failed to store history message: %v", err)
				} else {
					syncedCount++
					// Log successful message storage
					if mediaType != "" {
						logger.Infof("Stored message: [%s] %s -> %s: [%s: %s] %s",
							timestamp.Format("2006-01-02 15:04:05"), sender, chatJID, mediaType, filename, content)
					} else {
						logger.Infof("Stored message: [%s] %s -> %s: %s",
							timestamp.Format("2006-01-02 15:04:05"), sender, chatJID, content)
					}
				}
			}
		}
	}

	fmt.Printf("History sync complete. Stored %d messages.\n", syncedCount)
}

// Request history sync from the server
func requestHistorySync(client *whatsmeow.Client) {
	if client == nil {
		fmt.Println("Client is not initialized. Cannot request history sync.")
		return
	}

	if !client.IsConnected() {
		fmt.Println("Client is not connected. Please ensure you are connected to WhatsApp first.")
		return
	}

	if client.Store.ID == nil {
		fmt.Println("Client is not logged in. Please scan the QR code first.")
		return
	}

	// Build and send a history sync request
	historyMsg := client.BuildHistorySyncRequest(nil, 100)
	if historyMsg == nil {
		fmt.Println("Failed to build history sync request.")
		return
	}

	_, err := client.SendMessage(context.Background(), types.JID{
		Server: "s.whatsapp.net",
		User:   "status",
	}, historyMsg)

	if err != nil {
		fmt.Printf("Failed to request history sync: %v\n", err)
	} else {
		fmt.Println("History sync requested. Waiting for server response...")
	}
}

// analyzeOggOpus tries to extract duration and generate a simple waveform from an Ogg Opus file
func analyzeOggOpus(data []byte) (duration uint32, waveform []byte, err error) {
	// Try to detect if this is a valid Ogg file by checking for the "OggS" signature
	// at the beginning of the file
	if len(data) < 4 || string(data[0:4]) != "OggS" {
		return 0, nil, fmt.Errorf("not a valid Ogg file (missing OggS signature)")
	}

	// Parse Ogg pages to find the last page with a valid granule position
	var lastGranule uint64
	var sampleRate uint32 = 48000 // Default Opus sample rate
	var preSkip uint16 = 0
	var foundOpusHead bool

	// Scan through the file looking for Ogg pages
	for i := 0; i < len(data); {
		// Check if we have enough data to read Ogg page header
		if i+27 >= len(data) {
			break
		}

		// Verify Ogg page signature
		if string(data[i:i+4]) != "OggS" {
			// Skip until next potential page
			i++
			continue
		}

		// Extract header fields
		granulePos := binary.LittleEndian.Uint64(data[i+6 : i+14])
		pageSeqNum := binary.LittleEndian.Uint32(data[i+18 : i+22])
		numSegments := int(data[i+26])

		// Extract segment table
		if i+27+numSegments >= len(data) {
			break
		}
		segmentTable := data[i+27 : i+27+numSegments]

		// Calculate page size
		pageSize := 27 + numSegments
		for _, segLen := range segmentTable {
			pageSize += int(segLen)
		}

		// Check if we're looking at an OpusHead packet (should be in first few pages)
		if !foundOpusHead && pageSeqNum <= 1 {
			// Look for "OpusHead" marker in this page
			pageData := data[i : i+pageSize]
			headPos := bytes.Index(pageData, []byte("OpusHead"))
			if headPos >= 0 && headPos+12 < len(pageData) {
				// Found OpusHead, extract sample rate and pre-skip
				// OpusHead format: Magic(8) + Version(1) + Channels(1) + PreSkip(2) + SampleRate(4) + ...
				headPos += 8 // Skip "OpusHead" marker
				// PreSkip is 2 bytes at offset 10
				if headPos+12 <= len(pageData) {
					preSkip = binary.LittleEndian.Uint16(pageData[headPos+10 : headPos+12])
					sampleRate = binary.LittleEndian.Uint32(pageData[headPos+12 : headPos+16])
					foundOpusHead = true
					fmt.Printf("Found OpusHead: sampleRate=%d, preSkip=%d\n", sampleRate, preSkip)
				}
			}
		}

		// Keep track of last valid granule position
		if granulePos != 0 {
			lastGranule = granulePos
		}

		// Move to next page
		i += pageSize
	}

	if !foundOpusHead {
		fmt.Println("Warning: OpusHead not found, using default values")
	}

	// Calculate duration based on granule position
	if lastGranule > 0 {
		// Formula for duration: (lastGranule - preSkip) / sampleRate
		durationSeconds := float64(lastGranule-uint64(preSkip)) / float64(sampleRate)
		duration = uint32(math.Ceil(durationSeconds))
		fmt.Printf("Calculated Opus duration from granule: %f seconds (lastGranule=%d)\n",
			durationSeconds, lastGranule)
	} else {
		// Fallback to rough estimation if granule position not found
		fmt.Println("Warning: No valid granule position found, using estimation")
		durationEstimate := float64(len(data)) / 2000.0 // Very rough approximation
		duration = uint32(durationEstimate)
	}

	// Make sure we have a reasonable duration (at least 1 second, at most 300 seconds)
	if duration < 1 {
		duration = 1
	} else if duration > 300 {
		duration = 300
	}

	// Generate waveform
	waveform = placeholderWaveform(duration)

	fmt.Printf("Ogg Opus analysis: size=%d bytes, calculated duration=%d sec, waveform=%d bytes\n",
		len(data), duration, len(waveform))

	return duration, waveform, nil
}

// min returns the smaller of x or y
func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

// placeholderWaveform generates a synthetic waveform for WhatsApp voice messages
// that appears natural with some variability based on the duration
func placeholderWaveform(duration uint32) []byte {
	// WhatsApp expects a 64-byte waveform for voice messages
	const waveformLength = 64
	waveform := make([]byte, waveformLength)

	// Seed the random number generator for consistent results with the same duration
	rand.Seed(int64(duration))

	// Create a more natural looking waveform with some patterns and variability
	// rather than completely random values

	// Base amplitude and frequency - longer messages get faster frequency
	baseAmplitude := 35.0
	frequencyFactor := float64(min(int(duration), 120)) / 30.0

	for i := range waveform {
		// Position in the waveform (normalized 0-1)
		pos := float64(i) / float64(waveformLength)

		// Create a wave pattern with some randomness
		// Use multiple sine waves of different frequencies for more natural look
		val := baseAmplitude * math.Sin(pos*math.Pi*frequencyFactor*8)
		val += (baseAmplitude / 2) * math.Sin(pos*math.Pi*frequencyFactor*16)

		// Add some randomness to make it look more natural
		val += (rand.Float64() - 0.5) * 15

		// Add some fade-in and fade-out effects
		fadeInOut := math.Sin(pos * math.Pi)
		val = val * (0.7 + 0.3*fadeInOut)

		// Center around 50 (typical voice baseline)
		val = val + 50

		// Ensure values stay within WhatsApp's expected range (0-100)
		if val < 0 {
			val = 0
		} else if val > 100 {
			val = 100
		}

		waveform[i] = byte(val)
	}

	return waveform
}
