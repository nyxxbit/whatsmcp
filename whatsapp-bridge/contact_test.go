package main

import (
	"testing"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"
)

func TestBuildContactName(t *testing.T) {
	pn := types.NewJID("5511999990003", types.DefaultUserServer)
	lid := types.NewJID("100000000000003", types.HiddenUserServer)
	p := buildContactName(pn, lid, "Alex Carter", true)
	if p.Type != appstate.WAPatchCriticalUnblockLow || len(p.Mutations) != 1 {
		t.Fatalf("patch type/mutations: %v %d", p.Type, len(p.Mutations))
	}
	m := p.Mutations[0]
	if len(m.Index) != 2 || m.Index[0] != "contact" || m.Index[1] != "5511999990003@s.whatsapp.net" || m.Version != 2 {
		t.Fatalf("index/version: %v %d", m.Index, m.Version)
	}
	a := m.Value.GetContactAction()
	if a.GetFullName() != "Alex Carter" || a.GetFirstName() != "Alex" || a.GetLidJID() != "100000000000003@lid" ||
		!a.GetSaveOnPrimaryAddressbook() {
		t.Fatalf("action: %+v", a)
	}
	if a := buildContactName(pn, types.EmptyJID, "Rose", false).Mutations[0].Value.GetContactAction(); a.LidJID != nil ||
		a.GetFirstName() != "Rose" || a.GetSaveOnPrimaryAddressbook() {
		t.Fatalf("no lid, one word, no save: %+v", a)
	}
}
