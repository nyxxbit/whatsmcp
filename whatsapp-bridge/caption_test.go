package main

import (
	"strings"
	"testing"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// Legenda de midia. Antes desta correcao, uma foto legendada chegava ao banco com content
// vazio e o dado escrito na legenda (horario de chegada da equipe, valor de nota, numero
// da filial) simplesmente sumia.
func TestExtractTextContentCaption(t *testing.T) {
	cases := []struct {
		name string
		msg  *waProto.Message
		want string
	}{
		{"image with caption",
			&waProto.Message{ImageMessage: &waProto.ImageMessage{
				Caption: proto.String("Arrived on site at 08:02")}},
			"Arrived on site at 08:02"},
		{"video with caption",
			&waProto.Message{VideoMessage: &waProto.VideoMessage{
				Caption: proto.String("video caption")}}, "video caption"},
		{"document with caption",
			&waProto.Message{DocumentMessage: &waProto.DocumentMessage{
				Caption: proto.String("August invoice")}}, "August invoice"},
		{"document without caption falls back to title",
			&waProto.Message{DocumentMessage: &waProto.DocumentMessage{
				Title: proto.String("report.pdf")}}, "report.pdf"},
		{"plain text still works",
			&waProto.Message{Conversation: proto.String("good morning")}, "good morning"},
		{"image without caption stays empty",
			&waProto.Message{ImageMessage: &waProto.ImageMessage{}}, ""},
	}
	for _, c := range cases {
		if got := extractTextContent(c.msg); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// Containers. O WhatsApp embrulha a mensagem real quando o chat tem mensagem temporaria
// ligada, quando e visualizacao unica, quando o documento vai com legenda e quando a
// mensagem foi edited. Sem desembrulhar, o bridge perde a MENSAGEM INTEIRA - texto e midia.
func TestUnwrapMessage(t *testing.T) {
	dentro := &waProto.Message{ImageMessage: &waProto.ImageMessage{
		Caption: proto.String("arrived at 08:02"),
		URL:     proto.String("https://exemplo/imagem.enc"),
	}}

	cases := map[string]*waProto.Message{
		"ephemeral":      {EphemeralMessage: &waProto.FutureProofMessage{Message: dentro}},
		"view once":      {ViewOnceMessage: &waProto.FutureProofMessage{Message: dentro}},
		"view once v2":   {ViewOnceMessageV2: &waProto.FutureProofMessage{Message: dentro}},
		"doc with caption": {DocumentWithCaptionMessage: &waProto.FutureProofMessage{Message: dentro}},
		"edited":        {EditedMessage: &waProto.FutureProofMessage{Message: dentro}},
	}
	for name, msg := range cases {
		if got := extractTextContent(msg); got != "arrived at 08:02" {
			t.Errorf("%s: text was %q", name, got)
		}
		mediaType, _, url, _, _, _, _ := extractMediaInfo(msg)
		if mediaType != "image" || url == "" {
			t.Errorf("%s: media lost (type=%q url=%q)", name, mediaType, url)
		}
	}

	// ephemeral inside view-once: nesting must hold
	duplo := &waProto.Message{EphemeralMessage: &waProto.FutureProofMessage{
		Message: &waProto.Message{ViewOnceMessageV2: &waProto.FutureProofMessage{Message: dentro}}}}
	if got := extractTextContent(duplo); got != "arrived at 08:02" {
		t.Errorf("nested: got %q", got)
	}
}

// Tipos sem midia que carregam informacao util na obra.
func TestExtractTextContentSemMidia(t *testing.T) {
	loc := &waProto.Message{LocationMessage: &waProto.LocationMessage{
		DegreesLatitude:  proto.Float64(37.4220),
		DegreesLongitude: proto.Float64(-122.0841),
		Name:             proto.String("Acme Warehouse"),
	}}
	got := extractTextContent(loc)
	if !strings.Contains(got, "location") || !strings.Contains(got, "Acme Warehouse") {
		t.Errorf("location: got %q", got)
	}

	ct := &waProto.Message{ContactMessage: &waProto.ContactMessage{
		DisplayName: proto.String("Jane Doe")}}
	if got := extractTextContent(ct); got != "[contact] Jane Doe" {
		t.Errorf("contact: got %q", got)
	}

	poll := &waProto.Message{PollCreationMessage: &waProto.PollCreationMessage{
		Name: proto.String("Which day works?")}}
	if got := extractTextContent(poll); got != "[poll] Which day works?" {
		t.Errorf("poll: got %q", got)
	}
}

// Evento nativo do WhatsApp - o buraco da issue #310 do lharries/whatsapp-mcp: o evento e
// criado no grupo mas a leitura nao devolve nada, e o usuario ve um texto solto no lugar.
func TestExtractTextContentEvento(t *testing.T) {
	ev := &waProto.Message{EventMessage: &waProto.EventMessage{
		Name:      proto.String("Quarterly site review"),
		StartTime: proto.Int64(1786000000),
	}}
	got := extractTextContent(ev)
	if !strings.HasPrefix(got, "[event] Quarterly site review") || !strings.Contains(got, "starts") {
		t.Errorf("event: got %q", got)
	}

	cancelado := &waProto.Message{EventMessage: &waProto.EventMessage{
		Name:       proto.String("Reuniao"),
		IsCanceled: proto.Bool(true),
	}}
	if got := extractTextContent(cancelado); !strings.Contains(got, "CANCELLED") {
		t.Errorf("cancelled event: got %q", got)
	}

	// must still surface from inside a container
	dentro := &waProto.Message{EphemeralMessage: &waProto.FutureProofMessage{Message: ev}}
	if got := extractTextContent(dentro); !strings.Contains(got, "Quarterly site review") {
		t.Errorf("event inside ephemeral: got %q", got)
	}
}
