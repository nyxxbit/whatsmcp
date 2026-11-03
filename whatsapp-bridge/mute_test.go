package main

import (
	"errors"
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestStartsChat(t *testing.T) {
	pn := types.NewJID("5511999990003", types.DefaultUserServer)
	lid := types.NewJID("100000000000003", types.HiddenUserServer)
	group := types.NewJID("120363000000000001", types.GroupServer)
	count := func(stored map[string]int) func(string) (int, error) {
		return func(j string) (int, error) { return stored[j], nil }
	}

	if !startsChat([]types.JID{pn}, count(nil)) {
		t.Error("a number never written to is a new chat")
	}
	if !startsChat([]types.JID{pn, lid}, count(nil)) {
		t.Error("number and @lid both empty is a new chat")
	}
	// the conversation lives under the @lid, as Márcio's does: sending to his number is not new
	if startsChat([]types.JID{pn, lid}, count(map[string]int{lid.String(): 12})) {
		t.Error("history under the @lid twin means an old chat")
	}
	if startsChat([]types.JID{group}, count(nil)) {
		t.Error("a group is never muted on its own")
	}
	if startsChat([]types.JID{pn}, func(string) (int, error) { return 0, errors.New("db locked") }) {
		t.Error("when the database cannot answer, do not mute")
	}
	if startsChat(nil, count(nil)) {
		t.Error("no JID, no new chat")
	}
}
