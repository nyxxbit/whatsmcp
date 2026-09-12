package main

// Menus interativos do WhatsApp: ler as opcoes que chegam e RESPONDER clicando nelas.
//
// O problema que isto resolve, de 12/09/2026: a Localiza atende por um bot que manda o menu
// como *mensagem interativa*, nao como texto. O bridge nao sabia ler esse tipo, entao o
// `content` ficava vazio e do lado de ca' parecia que o bot tinha travado. Pior: responder com
// texto solto ("Assistencia 24h") faz o bot devolver "Desculpe, nao entendi", porque ele espera
// o ID da opcao e nao o rotulo dela.
//
// Sao quatro formatos, e cada um tem um tipo de RESPOSTA proprio:
//
//	ListMessage         -> ListResponseMessage         (rowID)
//	ButtonsMessage      -> ButtonsResponseMessage      (buttonID)
//	TemplateMessage     -> TemplateButtonReplyMessage  (id + indice)
//	InteractiveMessage  -> InteractiveResponseMessage  (name + paramsJSON)
//
// Em todos, a resposta tem que citar a mensagem original no ContextInfo (StanzaID e
// Participant). Sem isso o servidor aceita, mas o bot nao casa a resposta com a pergunta.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// MenuOption e' uma opcao clicavel de um menu recebido.
type MenuOption struct {
	Index       int    `json:"index"`       // 1-based, e' por aqui que se escolhe
	Kind        string `json:"kind"`        // list | button | template | nativeflow
	OptionID    string `json:"option_id"`   // rowID, buttonID, templateID ou nome do fluxo
	Title       string `json:"title"`       // o texto que o usuario ve'
	Description string `json:"description"` // subtitulo, quando existe
	Section     string `json:"section"`     // secao da lista, quando existe
	ParamsJSON  string `json:"params_json"` // so' no nativeflow
}

// extractMenuOptions devolve as opcoes clicaveis da mensagem, se ela for um menu.
func extractMenuOptions(msg *waProto.Message) []MenuOption {
	msg = unwrapMessage(msg)
	if msg == nil {
		return nil
	}
	var out []MenuOption
	add := func(kind, id, title, desc, section, params string) {
		out = append(out, MenuOption{
			Index: len(out) + 1, Kind: kind, OptionID: id,
			Title: title, Description: desc, Section: section, ParamsJSON: params,
		})
	}

	if lst := msg.GetListMessage(); lst != nil {
		for _, sec := range lst.GetSections() {
			for _, row := range sec.GetRows() {
				add("list", row.GetRowID(), row.GetTitle(), row.GetDescription(), sec.GetTitle(), "")
			}
		}
	}
	if btns := msg.GetButtonsMessage(); btns != nil {
		for _, b := range btns.GetButtons() {
			add("button", b.GetButtonID(), b.GetButtonText().GetDisplayText(), "", "", "")
		}
	}
	if tpl := msg.GetTemplateMessage(); tpl != nil {
		for _, b := range tpl.GetHydratedTemplate().GetHydratedButtons() {
			if q := b.GetQuickReplyButton(); q != nil {
				add("template", q.GetID(), q.GetDisplayText(), "", "", "")
			} else if u := b.GetUrlButton(); u != nil {
				add("template", u.GetURL(), u.GetDisplayText(), "url", "", "")
			} else if c := b.GetCallButton(); c != nil {
				add("template", c.GetPhoneNumber(), c.GetDisplayText(), "telefone", "", "")
			}
		}
	}
	if inter := msg.GetInteractiveMessage(); inter != nil {
		for _, b := range inter.GetNativeFlowMessage().GetButtons() {
			add("nativeflow", b.GetName(), rotuloNativeFlow(b.GetName(), b.GetButtonParamsJSON()),
				"", "", b.GetButtonParamsJSON())
		}
	}
	return out
}

// rotuloNativeFlow tira o texto legivel de dentro do JSON do botao, que e' onde ele vive.
// Sem isto a opcao apareceria como "single_select", que nao diz nada a quem le'.
func rotuloNativeFlow(nome, params string) string {
	var m map[string]any
	if json.Unmarshal([]byte(params), &m) == nil {
		for _, k := range []string{"display_text", "title", "text", "label"} {
			if v, ok := m[k].(string); ok && v != "" {
				return v
			}
		}
	}
	return nome
}

// renderMenu transforma o menu em texto legivel, para o `content` deixar de ficar vazio.
func renderMenu(msg *waProto.Message, opts []MenuOption) string {
	msg = unwrapMessage(msg)
	var cab []string
	if lst := msg.GetListMessage(); lst != nil {
		cab = append(cab, lst.GetTitle(), lst.GetDescription())
	}
	if b := msg.GetButtonsMessage(); b != nil {
		cab = append(cab, b.GetContentText())
	}
	if t := msg.GetTemplateMessage().GetHydratedTemplate(); t != nil {
		cab = append(cab, t.GetHydratedContentText())
	}
	if i := msg.GetInteractiveMessage(); i != nil {
		cab = append(cab, i.GetHeader().GetTitle(), i.GetBody().GetText())
	}

	var sb strings.Builder
	sb.WriteString("[menu]")
	for _, c := range cab {
		if c = strings.TrimSpace(c); c != "" {
			sb.WriteString(" " + c)
		}
	}
	secAtual := ""
	for _, o := range opts {
		if o.Section != "" && o.Section != secAtual {
			secAtual = o.Section
			sb.WriteString("\n-- " + secAtual)
		}
		sb.WriteString(fmt.Sprintf("\n%d. %s", o.Index, o.Title))
		if o.Description != "" {
			sb.WriteString(" (" + o.Description + ")")
		}
	}
	return sb.String()
}

// StoreMenuOptions grava as opcoes para que se possa escolher por indice depois.
func (store *MessageStore) StoreMenuOptions(id, chatJID string, opts []MenuOption) error {
	if len(opts) == 0 {
		return nil
	}
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM message_options WHERE message_id=? AND chat_jid=?",
		id, chatJID); err != nil {
		return err
	}
	for _, o := range opts {
		if _, err := tx.Exec(`INSERT INTO message_options
			(message_id, chat_jid, idx, kind, option_id, title, description, section, params_json)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			id, chatJID, o.Index, o.Kind, o.OptionID, o.Title, o.Description, o.Section,
			o.ParamsJSON); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// GetMenuOptions le' as opcoes de uma mensagem. Com id vazio, pega o menu MAIS RECENTE do chat,
// que e' o caso normal: o bot acabou de perguntar e eu quero responder.
func (store *MessageStore) GetMenuOptions(id, chatJID string) (string, []MenuOption, error) {
	if id == "" {
		err := store.db.QueryRow(`SELECT o.message_id FROM message_options o
			JOIN messages m ON m.id = o.message_id AND m.chat_jid = o.chat_jid
			WHERE o.chat_jid = ? AND m.is_from_me = 0
			ORDER BY m.timestamp DESC, o.idx ASC LIMIT 1`, chatJID).Scan(&id)
		if err == sql.ErrNoRows {
			return "", nil, fmt.Errorf("nenhum menu recebido neste chat")
		} else if err != nil {
			return "", nil, err
		}
	}
	rows, err := store.db.Query(`SELECT idx, kind, option_id, title, description, section,
		params_json FROM message_options WHERE message_id=? AND chat_jid=? ORDER BY idx`,
		id, chatJID)
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	var out []MenuOption
	for rows.Next() {
		var o MenuOption
		if err := rows.Scan(&o.Index, &o.Kind, &o.OptionID, &o.Title, &o.Description,
			&o.Section, &o.ParamsJSON); err != nil {
			return "", nil, err
		}
		out = append(out, o)
	}
	if len(out) == 0 {
		return "", nil, fmt.Errorf("mensagem %s nao tem opcoes guardadas", id)
	}
	return id, out, nil
}

// escolheOpcao acha a opcao por indice, por ID ou por titulo. O titulo casa sem diferenciar
// maiuscula e acento nao, de proposito: quem digita "revisao" quer "Revisão e Manutenção".
func escolheOpcao(opts []MenuOption, indice int, optID, titulo string) (*MenuOption, error) {
	if indice > 0 {
		for i := range opts {
			if opts[i].Index == indice {
				return &opts[i], nil
			}
		}
		return nil, fmt.Errorf("opcao %d nao existe, o menu tem %d", indice, len(opts))
	}
	if optID != "" {
		for i := range opts {
			if opts[i].OptionID == optID {
				return &opts[i], nil
			}
		}
		return nil, fmt.Errorf("nenhuma opcao com id %q", optID)
	}
	if titulo != "" {
		alvo := strings.ToLower(strings.TrimSpace(titulo))
		for i := range opts {
			if strings.ToLower(opts[i].Title) == alvo {
				return &opts[i], nil
			}
		}
		for i := range opts { // segunda passada, por pedaco
			if strings.Contains(strings.ToLower(opts[i].Title), alvo) {
				return &opts[i], nil
			}
		}
		return nil, fmt.Errorf("nenhuma opcao com titulo parecido com %q", titulo)
	}
	return nil, fmt.Errorf("informe option (indice), option_id ou title")
}

// montaResposta monta a mensagem de resposta do tipo certo para a opcao escolhida.
func montaResposta(o *MenuOption, stanzaID string, participant string) *waProto.Message {
	ctx := &waProto.ContextInfo{StanzaID: proto.String(stanzaID)}
	if participant != "" {
		ctx.Participant = proto.String(participant)
	}

	switch o.Kind {
	case "list":
		return &waProto.Message{ListResponseMessage: &waProto.ListResponseMessage{
			Title:    proto.String(o.Title),
			ListType: waProto.ListResponseMessage_SINGLE_SELECT.Enum(),
			SingleSelectReply: &waProto.ListResponseMessage_SingleSelectReply{
				SelectedRowID: proto.String(o.OptionID),
			},
			ContextInfo: ctx,
		}}
	case "button":
		return &waProto.Message{ButtonsResponseMessage: &waProto.ButtonsResponseMessage{
			SelectedButtonID: proto.String(o.OptionID),
			Response: &waProto.ButtonsResponseMessage_SelectedDisplayText{
				SelectedDisplayText: o.Title,
			},
			Type:        waProto.ButtonsResponseMessage_DISPLAY_TEXT.Enum(),
			ContextInfo: ctx,
		}}
	case "template":
		return &waProto.Message{TemplateButtonReplyMessage: &waProto.TemplateButtonReplyMessage{
			SelectedID:          proto.String(o.OptionID),
			SelectedDisplayText: proto.String(o.Title),
			SelectedIndex:       proto.Uint32(uint32(o.Index - 1)),
			ContextInfo:         ctx,
		}}
	case "nativeflow":
		return &waProto.Message{InteractiveResponseMessage: &waProto.InteractiveResponseMessage{
			Body: &waProto.InteractiveResponseMessage_Body{
				Text:   proto.String(o.Title),
				Format: waProto.InteractiveResponseMessage_Body_DEFAULT.Enum(),
			},
			InteractiveResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage_{
				NativeFlowResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage{
					Name:       proto.String(o.OptionID),
					ParamsJSON: proto.String(o.ParamsJSON),
					Version:    proto.Int32(2),
				},
			},
			ContextInfo: ctx,
		}}
	}
	return nil
}

// SelectOptionRequest e' o corpo de /api/select-option.
type SelectOptionRequest struct {
	ChatJID   string `json:"chat_jid"`
	MessageID string `json:"message_id"` // vazio = ultimo menu recebido no chat
	Option    int    `json:"option"`     // indice 1-based
	OptionID  string `json:"option_id"`  // alternativa ao indice
	Title     string `json:"title"`      // alternativa ao indice
}

// selecionaOpcao faz o clique: acha a opcao, monta a resposta certa e envia.
func selecionaOpcao(client *whatsmeow.Client, store *MessageStore,
	req SelectOptionRequest) (string, string, error) {
	if !client.IsConnected() {
		return "", "", fmt.Errorf("nao conectado ao WhatsApp")
	}
	if req.ChatJID == "" {
		return "", "", fmt.Errorf("chat_jid e' obrigatorio")
	}
	chat, err := types.ParseJID(req.ChatJID)
	if err != nil {
		return "", "", fmt.Errorf("chat_jid invalido: %v", err)
	}

	msgID, opts, err := store.GetMenuOptions(req.MessageID, req.ChatJID)
	if err != nil {
		return "", "", err
	}
	o, err := escolheOpcao(opts, req.Option, req.OptionID, req.Title)
	if err != nil {
		return "", "", err
	}

	// O participant so' existe em grupo; em conversa de um para um fica vazio.
	var participant string
	if chat.Server == types.GroupServer {
		_ = store.db.QueryRow("SELECT sender FROM messages WHERE id=? AND chat_jid=?",
			msgID, req.ChatJID).Scan(&participant)
		if participant != "" && !strings.Contains(participant, "@") {
			participant += "@s.whatsapp.net"
		}
	}

	msg := montaResposta(o, msgID, participant)
	if msg == nil {
		return "", "", fmt.Errorf("tipo de opcao desconhecido: %s", o.Kind)
	}
	resp, err := client.SendMessage(context.Background(), chat, msg)
	if err != nil {
		return "", "", fmt.Errorf("erro enviando: %v", err)
	}
	return resp.ID, o.Title, nil
}
