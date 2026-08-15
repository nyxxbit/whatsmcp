package domain_test

import (
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

func TestNewJID_valido(t *testing.T) {
	cases := map[string]struct {
		raw    string
		user   string
		server domain.ServerType
		isLID  bool
	}{
		"lid":   {"100000000000003@lid", "100000000000003", domain.ServerLID, true},
		"pn":    {"5511999990003@s.whatsapp.net", "5511999990003", domain.ServerPN, false},
		"grupo": {"120363-100@g.us", "120363-100", domain.ServerGroup, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			jid, err := domain.NewJID(c.raw)
			if err != nil {
				t.Fatalf("erro inesperado: %v", err)
			}
			if jid.User() != c.user {
				t.Fatalf("User = %q, esperado %q", jid.User(), c.user)
			}
			if jid.Server() != c.server {
				t.Fatalf("Server = %q, esperado %q", jid.Server(), c.server)
			}
			if jid.IsLID() != c.isLID {
				t.Fatalf("IsLID = %v, esperado %v", jid.IsLID(), c.isLID)
			}
			if jid.String() != c.raw {
				t.Fatalf("String = %q, esperado %q", jid.String(), c.raw)
			}
		})
	}
}

func TestNewJID_invalido_failFast(t *testing.T) {
	for _, raw := range []string{"", "semarroba", "@semuser", "semserver@", "   "} {
		if _, err := domain.NewJID(raw); err == nil {
			t.Fatalf("esperava erro de validação para %q", raw)
		}
	}
}

func TestJID_imutavel_igualdadePorValor(t *testing.T) {
	a := domain.MustJID("5511999990003@s.whatsapp.net")
	b := domain.MustJID("5511999990003@s.whatsapp.net")
	if a != b { // Value Object: comparável diretamente por ==
		t.Fatal("JIDs com mesmo valor deveriam ser iguais")
	}
	if domain.MustJID("x@lid") == b {
		t.Fatal("JIDs diferentes não deveriam ser iguais")
	}
}
