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
		"group": {"120363-100@g.us", "120363-100", domain.ServerGroup, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			jid, err := domain.NewJID(c.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if jid.User() != c.user {
				t.Fatalf("User = %q, expected %q", jid.User(), c.user)
			}
			if jid.Server() != c.server {
				t.Fatalf("Server = %q, expected %q", jid.Server(), c.server)
			}
			if jid.IsLID() != c.isLID {
				t.Fatalf("IsLID = %v, expected %v", jid.IsLID(), c.isLID)
			}
			if jid.String() != c.raw {
				t.Fatalf("String = %q, expected %q", jid.String(), c.raw)
			}
		})
	}
}

func TestNewJID_invalido_failFast(t *testing.T) {
	for _, raw := range []string{"", "semarroba", "@semuser", "semserver@", "   "} {
		if _, err := domain.NewJID(raw); err == nil {
			t.Fatalf("expected a validation error for %q", raw)
		}
	}
}

func TestJID_imutavel_igualdadePorValor(t *testing.T) {
	a := domain.MustJID("5511999990003@s.whatsapp.net")
	b := domain.MustJID("5511999990003@s.whatsapp.net")
	if a != b { // Value Object: directly comparable via ==
		t.Fatal("JIDs with the same value should be equal")
	}
	if domain.MustJID("x@lid") == b {
		t.Fatal("different JIDs should not be equal")
	}
}
