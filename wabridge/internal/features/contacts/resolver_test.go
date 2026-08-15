package contacts_test

import (
	"context"
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/features/contacts"
)

// fakeRepo is an in-memory stub of ContactRepository; it tests the use case
// without touching SQLite or whatsmeow (the Port abstraction allows this).
type fakeRepo struct {
	byJID map[string]domain.Identity
}

func (f fakeRepo) Identify(_ context.Context, jid domain.JID) (domain.Identity, error) {
	if id, ok := f.byJID[jid.String()]; ok {
		return id, nil
	}
	// Best-effort: with no known name/phone, returns just the JID (not an error).
	return domain.NewIdentity(jid, "", "")
}

func TestResolver_identifyTrazNomeENumero(t *testing.T) {
	jid := domain.MustJID("5511999990003@s.whatsapp.net")
	identity, err := domain.NewIdentity(jid, "Alex Carter - Springfield", "5511999990003")
	if err != nil {
		t.Fatal(err)
	}
	resolver := contacts.NewResolver(fakeRepo{byJID: map[string]domain.Identity{jid.String(): identity}})

	id := resolver.Identify(context.Background(), jid)
	if id.Name() != "Alex Carter - Springfield" {
		t.Fatalf("Name = %q", id.Name())
	}
	if id.Phone() != "5511999990003" {
		t.Fatalf("Phone = %q", id.Phone())
	}
	if id.Display() != "Alex Carter - Springfield (5511999990003)" {
		t.Fatalf("Display = %q (expected name + number together)", id.Display())
	}
}

func TestResolver_desconhecidoPreservaJID(t *testing.T) {
	resolver := contacts.NewResolver(fakeRepo{byJID: map[string]domain.Identity{}})
	jid := domain.MustJID("100000000000003@lid")

	id := resolver.Identify(context.Background(), jid)
	if id.HasName() {
		t.Fatalf("should not have a name, got %q", id.Name())
	}
	if id.JID() != jid {
		t.Fatalf("raw JID should be preserved")
	}
	if id.Display() != "100000000000003" { // fallback to the JID's user portion
		t.Fatalf("Display = %q", id.Display())
	}
}
