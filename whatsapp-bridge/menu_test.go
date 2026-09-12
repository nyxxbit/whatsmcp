package main

import (
	"strings"
	"testing"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// Um menu de lista igual ao que bot de empresa manda, com duas secoes.
func listaExemplo() *waProto.Message {
	return &waProto.Message{ListMessage: &waProto.ListMessage{
		Title:       proto.String("Localiza"),
		Description: proto.String("Como posso ajudar?"),
		Sections: []*waProto.ListMessage_Section{
			{Title: proto.String("Atendimento"), Rows: []*waProto.ListMessage_Row{
				{Title: proto.String("Revisão e Manutenção"), RowID: proto.String("rev_man"),
					Description: proto.String("agendar")},
				{Title: proto.String("Assistência"), RowID: proto.String("assist")},
			}},
			{Title: proto.String("Outros"), Rows: []*waProto.ListMessage_Row{
				{Title: proto.String("Financeiro"), RowID: proto.String("fin")},
			}},
		},
	}}
}

func TestExtraiOpcoesDeLista(t *testing.T) {
	opts := extractMenuOptions(listaExemplo())
	if len(opts) != 3 {
		t.Fatalf("esperava 3 opcoes, veio %d", len(opts))
	}
	if opts[0].Index != 1 || opts[0].OptionID != "rev_man" || opts[0].Kind != "list" {
		t.Errorf("primeira opcao errada: %+v", opts[0])
	}
	if opts[0].Section != "Atendimento" || opts[2].Section != "Outros" {
		t.Errorf("secoes erradas: %q e %q", opts[0].Section, opts[2].Section)
	}
	if opts[2].Index != 3 {
		t.Errorf("indice nao e' continuo entre secoes: %d", opts[2].Index)
	}
}

// O sintoma que comecou tudo: menu chegava e o content ficava vazio.
func TestMenuNaoFicaMaisVazio(t *testing.T) {
	texto := extractTextContent(listaExemplo())
	if texto == "" {
		t.Fatal("content vazio, que e' exatamente o bug da Localiza")
	}
	for _, esperado := range []string{"[menu]", "Localiza", "1. Revisão e Manutenção",
		"2. Assistência", "3. Financeiro", "-- Outros"} {
		if !strings.Contains(texto, esperado) {
			t.Errorf("faltou %q em:\n%s", esperado, texto)
		}
	}
}

func TestExtraiBotoesENativeFlow(t *testing.T) {
	btn := &waProto.Message{ButtonsMessage: &waProto.ButtonsMessage{
		ContentText: proto.String("Confirma?"),
		Buttons: []*waProto.ButtonsMessage_Button{
			{ButtonID: proto.String("sim"), ButtonText: &waProto.ButtonsMessage_Button_ButtonText{
				DisplayText: proto.String("Sim")}},
		},
	}}
	o := extractMenuOptions(btn)
	if len(o) != 1 || o[0].Kind != "button" || o[0].OptionID != "sim" || o[0].Title != "Sim" {
		t.Errorf("botao mal lido: %+v", o)
	}

	// No native flow o rotulo vive dentro do JSON, nao no nome do botao.
	nf := &waProto.Message{InteractiveMessage: &waProto.InteractiveMessage{
		InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{
			NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
					{Name: proto.String("single_select"),
						ButtonParamsJSON: proto.String(`{"display_text":"Ver opções"}`)},
				},
			},
		},
	}}
	o = extractMenuOptions(nf)
	if len(o) != 1 || o[0].Kind != "nativeflow" || o[0].Title != "Ver opções" {
		t.Errorf("native flow mal lido: %+v", o)
	}
	if o[0].OptionID != "single_select" {
		t.Errorf("o name tem que virar option_id: %+v", o[0])
	}
}

func TestEscolheOpcao(t *testing.T) {
	opts := extractMenuOptions(listaExemplo())

	if o, err := escolheOpcao(opts, 2, "", ""); err != nil || o.OptionID != "assist" {
		t.Errorf("por indice falhou: %v %+v", err, o)
	}
	if o, err := escolheOpcao(opts, 0, "fin", ""); err != nil || o.Index != 3 {
		t.Errorf("por id falhou: %v %+v", err, o)
	}
	// titulo exato, sem diferenciar maiuscula
	if o, err := escolheOpcao(opts, 0, "", "revisão e manutenção"); err != nil ||
		o.OptionID != "rev_man" {
		t.Errorf("por titulo exato falhou: %v %+v", err, o)
	}
	// titulo por pedaco, que e' como eu digitaria na pratica
	if o, err := escolheOpcao(opts, 0, "", "Financ"); err != nil || o.OptionID != "fin" {
		t.Errorf("por pedaco do titulo falhou: %v %+v", err, o)
	}
	if _, err := escolheOpcao(opts, 9, "", ""); err == nil {
		t.Error("indice fora da faixa tinha que dar erro")
	}
	if _, err := escolheOpcao(opts, 0, "", ""); err == nil {
		t.Error("sem criterio nenhum tinha que dar erro")
	}
}

func TestMontaRespostaDoTipoCerto(t *testing.T) {
	const stanza = "3EB0ABC123"

	lista := &MenuOption{Index: 1, Kind: "list", OptionID: "rev_man", Title: "Revisão"}
	m := montaResposta(lista, stanza, "")
	lr := m.GetListResponseMessage()
	if lr == nil {
		t.Fatal("lista tinha que virar ListResponseMessage")
	}
	if lr.GetSingleSelectReply().GetSelectedRowID() != "rev_man" {
		t.Errorf("rowID errado: %q", lr.GetSingleSelectReply().GetSelectedRowID())
	}
	if lr.GetContextInfo().GetStanzaID() != stanza {
		t.Errorf("sem citar a mensagem original o bot nao casa a resposta: %q",
			lr.GetContextInfo().GetStanzaID())
	}
	if lr.GetListType() != waProto.ListResponseMessage_SINGLE_SELECT {
		t.Errorf("listType errado: %v", lr.GetListType())
	}

	btn := &MenuOption{Index: 1, Kind: "button", OptionID: "sim", Title: "Sim"}
	br := montaResposta(btn, stanza, "").GetButtonsResponseMessage()
	if br == nil || br.GetSelectedButtonID() != "sim" ||
		br.GetSelectedDisplayText() != "Sim" {
		t.Errorf("botao mal montado: %+v", br)
	}

	tpl := &MenuOption{Index: 3, Kind: "template", OptionID: "t3", Title: "Terceiro"}
	tr := montaResposta(tpl, stanza, "").GetTemplateButtonReplyMessage()
	if tr == nil || tr.GetSelectedID() != "t3" || tr.GetSelectedIndex() != 2 {
		t.Errorf("template mal montado, o indice e' 0-based: %+v", tr)
	}

	nf := &MenuOption{Index: 1, Kind: "nativeflow", OptionID: "single_select",
		Title: "Ver", ParamsJSON: `{"a":1}`}
	ir := montaResposta(nf, stanza, "").GetInteractiveResponseMessage()
	if ir == nil || ir.GetNativeFlowResponseMessage().GetName() != "single_select" ||
		ir.GetNativeFlowResponseMessage().GetParamsJSON() != `{"a":1}` {
		t.Errorf("native flow mal montado: %+v", ir)
	}

	if montaResposta(&MenuOption{Kind: "coisa_nova"}, stanza, "") != nil {
		t.Error("tipo desconhecido tem que devolver nil, nao mandar lixo")
	}
}

// Em grupo a resposta precisa do participant; em conversa de um para um, nao.
func TestParticipantSoQuandoTem(t *testing.T) {
	o := &MenuOption{Kind: "list", OptionID: "x", Title: "X"}
	semGrupo := montaResposta(o, "ID1", "").GetListResponseMessage().GetContextInfo()
	if semGrupo.Participant != nil {
		t.Error("em conversa direta o participant tem que ficar de fora")
	}
	comGrupo := montaResposta(o, "ID1", "5517999@s.whatsapp.net").
		GetListResponseMessage().GetContextInfo()
	if comGrupo.GetParticipant() != "5517999@s.whatsapp.net" {
		t.Errorf("participant nao foi: %q", comGrupo.GetParticipant())
	}
}
