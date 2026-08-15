package sqlite_test

import (
	"context"
	"os"
	"testing"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/platform/persistence/sqlite"
)

// TestContactRepository_resolveContraBaseReal is a guarded integration test:
// it only runs if WABRIDGE_WADB points to a real whatsapp.db. Validates both
// resolution paths (direct PN and @lid → phone number → name).
func TestContactRepository_resolveContraBaseReal(t *testing.T) {
	path := os.Getenv("WABRIDGE_WADB")
	if path == "" {
		t.Skip("WABRIDGE_WADB not set, skipping integration test")
	}
	if _, err := os.Stat(path); err != nil {
		t.Skipf("whatsapp.db inaccessible at %q: %v", path, err)
	}

	repo, err := sqlite.OpenContactRepository(path)
	if err != nil {
		t.Fatalf("open repo: %v", err)
	}
	defer repo.Close()
	ctx := context.Background()

	t.Run("direct PN: name AND phone number", func(t *testing.T) {
		id, err := repo.Identify(ctx, domain.MustJID("5511999990003@s.whatsapp.net"))
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if id.Name() != "Sam Rivera" {
			t.Fatalf("Name = %q, want %q", id.Name(), "Sam Rivera")
		}
		if id.Phone() != "5511999990003" {
			t.Fatalf("Phone = %q, want %q (broad view: name AND phone number)", id.Phone(), "5511999990003")
		}
	})

	t.Run("@lid resolves the phone number even without a name", func(t *testing.T) {
		// 100000000000004@lid → 5511999990003 (lid_map). The phone number must
		// resolve even when the name doesn't exist: broad view.
		id, err := repo.Identify(ctx, domain.MustJID("100000000000004@lid"))
		if err != nil {
			t.Fatalf("Identify: %v", err)
		}
		if id.Phone() != "5511999990003" {
			t.Fatalf("Phone = %q, want %q", id.Phone(), "5511999990003")
		}
	})
}
