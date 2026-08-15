package main

import (
	"strings"
	"testing"
	"time"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// Media captions. Before this fix, a captioned photo reached the store with empty content
// and whatever was written in the caption was silently lost.
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

// Containers. WhatsApp wraps the real message when the chat has disappearing messages
// on, when the media is view-once, when a document carries a caption and when a message
// was edited. Without unwrapping, the bridge loses the ENTIRE message: text and media.
var testStamp = time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC)

func TestUnwrapMessage(t *testing.T) {
	inner := &waProto.Message{ImageMessage: &waProto.ImageMessage{
		Caption: proto.String("arrived at 08:02"),
		URL:     proto.String("https://exemplo/imagem.enc"),
	}}

	cases := map[string]*waProto.Message{
		"ephemeral":        {EphemeralMessage: &waProto.FutureProofMessage{Message: inner}},
		"view once":        {ViewOnceMessage: &waProto.FutureProofMessage{Message: inner}},
		"view once v2":     {ViewOnceMessageV2: &waProto.FutureProofMessage{Message: inner}},
		"doc with caption": {DocumentWithCaptionMessage: &waProto.FutureProofMessage{Message: inner}},
		"edited":           {EditedMessage: &waProto.FutureProofMessage{Message: inner}},
	}
	for label, msg := range cases {
		if got := extractTextContent(msg); got != "arrived at 08:02" {
			t.Errorf("%s: text was %q", label, got)
		}
		mediaType, name, url, _, _, _, _ := extractMediaInfo(msg, testStamp, "3EB0ABCD1234")
		if mediaType != "image" || url == "" {
			t.Errorf("%s: media lost (type=%q url=%q)", label, mediaType, url)
		}
		// the filename must come from the message, never from the wall clock:
		// a history sync processes hundreds of messages within the same second
		if !strings.Contains(name, "20260814_120000") || !strings.HasSuffix(name, "1234.jpg") {
			t.Errorf("%s: filename %q is not derived from the message", label, name)
		}
	}

	// ephemeral inside view-once: nesting must hold
	nested := &waProto.Message{EphemeralMessage: &waProto.FutureProofMessage{
		Message: &waProto.Message{ViewOnceMessageV2: &waProto.FutureProofMessage{Message: inner}}}}
	if got := extractTextContent(nested); got != "arrived at 08:02" {
		t.Errorf("nested: got %q", got)
	}
}

// Non-media types that still carry useful content.
func TestExtractTextContentNonMedia(t *testing.T) {
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

// Native WhatsApp events, the gap behind lharries/whatsapp-mcp#310: the event is created
// in the group but reads return nothing, so the user sees a stray plain-text summary.
func TestExtractTextContentEvent(t *testing.T) {
	ev := &waProto.Message{EventMessage: &waProto.EventMessage{
		Name:      proto.String("Quarterly site review"),
		StartTime: proto.Int64(1786000000),
	}}
	got := extractTextContent(ev)
	if !strings.HasPrefix(got, "[event] Quarterly site review") || !strings.Contains(got, "starts") {
		t.Errorf("event: got %q", got)
	}

	cancelled := &waProto.Message{EventMessage: &waProto.EventMessage{
		Name:       proto.String("Reuniao"),
		IsCanceled: proto.Bool(true),
	}}
	if got := extractTextContent(cancelled); !strings.Contains(got, "CANCELLED") {
		t.Errorf("cancelled event: got %q", got)
	}

	// must still surface from inside a container
	inner := &waProto.Message{EphemeralMessage: &waProto.FutureProofMessage{Message: ev}}
	if got := extractTextContent(inner); !strings.Contains(got, "Quarterly site review") {
		t.Errorf("event inside ephemeral: got %q", got)
	}
}
