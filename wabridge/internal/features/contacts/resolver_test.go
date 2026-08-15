package contacts_test

import (
	"context"
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/features/contacts"
)

// fakeRepo é um stub em memória de ContactRepository, testa o use case sem tocar
// em SQLite ou whatsmeow (a abstração por Port permite isso).
type fakeRepo struct {
	byJID map[string]domain.Identity
}

func (f fakeRepo) Identify(_ context.Context, jid domain.JID) (domain.Identity, error) {
	if id, ok := f.byJID[jid.String()]; ok {
		return id, nil
	}
	// Best-effort: sem nome/telefone conhecidos, devolve só o JID (não é erro).
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
		t.Fatalf("Display = %q (esperava nome + número juntos)", id.Display())
	}
}

func TestResolver_desconhecidoPreservaJID(t *testing.T) {
	resolver := contacts.NewResolver(fakeRepo{byJID: map[string]domain.Identity{}})
	jid := domain.MustJID("100000000000003@lid")

	id := resolver.Identify(context.Background(), jid)
	if id.HasName() {
		t.Fatalf("não deveria ter nome, veio %q", id.Name())
	}
	if id.JID() != jid {
		t.Fatalf("JID cru deveria ser preservado")
	}
	if id.Display() != "100000000000003" { // fallback para o user do JID
		t.Fatalf("Display = %q", id.Display())
	}
}
