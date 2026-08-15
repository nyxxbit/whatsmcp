package sqlite_test

import (
	"context"
	"os"
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/platform/persistence/sqlite"
)

// TestContactRepository_resolveContraBaseReal é um teste de integração guardado:
// só roda se WABRIDGE_WADB apontar para um whatsapp.db real. Valida os dois
// caminhos de resolução (PN direto e @lid → número → nome).
func TestContactRepository_resolveContraBaseReal(t *testing.T) {
	path := os.Getenv("WABRIDGE_WADB")
	if path == "" {
		t.Skip("WABRIDGE_WADB não definido, pulando teste de integração")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("whatsapp.db inacessível em %q: %v", path, err)
	}

	repo, err := sqlite.OpenContactRepository(path)
	if err != nil {
		t.Fatalf("abrir repo: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()

	t.Run("PN direto: nome E número", func(t *testing.T) {
		id, err := repo.Identify(ctx, domain.MustJID("5511999990003@s.whatsapp.net"))
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if id.Name() != "Sam Rivera" {
			t.Fatalf("Name = %q, esperava %q", id.Name(), "Sam Rivera")
		}
		if id.Phone() != "5511999990003" {
			t.Fatalf("Phone = %q, esperava %q (visão ampla: nome E número)", id.Phone(), "5511999990003")
		}
	})

	t.Run("@lid resolve o número mesmo sem nome", func(t *testing.T) {
		// 100000000000004@lid → 5511999990003 (lid_map). O número precisa resolver
		// mesmo que o nome não exista, visão ampla.
		id, err := repo.Identify(ctx, domain.MustJID("100000000000004@lid"))
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if id.Phone() != "5511999990003" {
			t.Fatalf("Phone = %q, esperava %q", id.Phone(), "5511999990003")
		}
	})
}
