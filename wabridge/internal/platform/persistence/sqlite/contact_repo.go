package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	_ "github.com/mattn/go-sqlite3"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// ContactRepository resolve a identidade COMPLETA de um JID (nome + número + JID)
// cruzando as tabelas do whatsmeow (whatsapp.db): whatsmeow_lid_map (@lid → número)
// e whatsmeow_contacts (número → nome). Abre o banco em modo SOMENTE-LEITURA, a
// escrita pertence ao próprio whatsmeow no adapter; aqui só consultamos.
type ContactRepository struct {
	db *sql.DB
}

var _ ports.ContactRepository = (*ContactRepository)(nil)

// OpenContactRepository abre um handle read-only para o store do whatsmeow.
// busy_timeout evita SQLITE_BUSY enquanto o adapter escreve na mesma base.
func OpenContactRepository(whatsappDBPath string) (*ContactRepository, error) {
	if whatsappDBPath == "" {
		return nil, fmt.Errorf("sqlite: caminho do whatsapp.db vazio")
	}
	db, err := sql.Open("sqlite3", "file:"+whatsappDBPath+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("sqlite: abrir whatsapp.db (ro): %w", err)
	}
	return &ContactRepository{db: db}, nil
}

// Close fecha o handle read-only.
func (r *ContactRepository) Close() error { return r.db.Close() }

// Identify resolve a identidade completa: NOME + NÚMERO + JID. "Visão ampla": o
// número é resolvido mesmo quando o nome é desconhecido, e o JID cru sempre volta.
// Não é erro não achar nome, devolve a Identity com o que houver.
func (r *ContactRepository) Identify(ctx context.Context, jid domain.JID) (domain.Identity, error) {
	if jid.IsZero() {
		return domain.Identity{}, domain.ErrInvalidJID
	}

	phone := ""
	candidates := make([]string, 0, 2)

	switch {
	case jid.IsLID():
		// @lid → número, via lid_map (colunas guardam dígitos crus, sem @server).
		if pn := r.lidToPhone(ctx, jid.User()); pn != "" {
			phone = pn
			candidates = append(candidates, pn+"@"+string(domain.ServerPN))
		}
	case jid.IsPN():
		phone = jid.User()
	}
	// Sempre tenta também o JID exato (cobre contatos gravados como @lid ou @s.whatsapp.net).
	candidates = append(candidates, jid.String())

	name := ""
	for _, theirJID := range candidates {
		if n := r.contactName(ctx, theirJID); n != "" {
			name = n
			break
		}
	}
	return domain.NewIdentity(jid, name, phone)
}

// lidToPhone resolve o número (pn) de um lid; "" se não mapeado.
func (r *ContactRepository) lidToPhone(ctx context.Context, lid string) string {
	var pn sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT pn FROM whatsmeow_lid_map WHERE lid = ? LIMIT 1", lid).Scan(&pn)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(pn.String)
}

// contactName busca o melhor nome de um their_jid: full_name → business_name →
// push_name → first_name (mesma prioridade usada na resolução de nomes dos peões).
func (r *ContactRepository) contactName(ctx context.Context, theirJID string) string {
	var full, business, push, first sql.NullString
	err := r.db.QueryRowContext(ctx,
		`SELECT full_name, business_name, push_name, first_name
		 FROM whatsmeow_contacts WHERE their_jid = ? LIMIT 1`,
		theirJID,
	).Scan(&full, &business, &push, &first)
	if errors.Is(err, sql.ErrNoRows) || err != nil {
		return ""
	}
	for _, candidate := range []sql.NullString{full, business, push, first} {
		if name := strings.TrimSpace(candidate.String); name != "" {
			return name
		}
	}
	return ""
}
