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

// ContactRepository resolves the COMPLETE identity of a JID (name + phone
// number + JID) by cross-referencing whatsmeow's tables (whatsapp.db):
// whatsmeow_lid_map (@lid → phone number) and whatsmeow_contacts (phone
// number → name). Opens the database in READ-ONLY mode; writes belong to
// whatsmeow itself in the adapter, here we only query.
type ContactRepository struct {
	db *sql.DB
}

var _ ports.ContactRepository = (*ContactRepository)(nil)

// OpenContactRepository opens a read-only handle to whatsmeow's store.
// busy_timeout avoids SQLITE_BUSY while the adapter writes to the same database.
func OpenContactRepository(whatsappDBPath string) (*ContactRepository, error) {
	if whatsappDBPath == "" {
		return nil, fmt.Errorf("sqlite: empty whatsapp.db path")
	}
	db, err := sql.Open("sqlite3", "file:"+whatsappDBPath+"?mode=ro&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("sqlite: open whatsapp.db (ro): %w", err)
	}
	return &ContactRepository{db: db}, nil
}

// Close closes the read-only handle.
func (r *ContactRepository) Close() error { return r.db.Close() }

// Identify resolves the complete identity: NAME + PHONE NUMBER + JID. "Broad
// view": the phone number is resolved even when the name is unknown, and the
// raw JID always comes back. Not finding a name is not an error; it returns
// the Identity with whatever it has.
func (r *ContactRepository) Identify(ctx context.Context, jid domain.JID) (domain.Identity, error) {
	if jid.IsZero() {
		return domain.Identity{}, domain.ErrInvalidJID
	}

	phone := ""
	candidates := make([]string, 0, 2)

	switch {
	case jid.IsLID():
		// @lid → phone number, via lid_map (columns store raw digits, without @server).
		if pn := r.lidToPhone(ctx, jid.User()); pn != "" {
			phone = pn
			candidates = append(candidates, pn+"@"+string(domain.ServerPN))
		}
	case jid.IsPN():
		phone = jid.User()
	}
	// Always also tries the exact JID (covers contacts stored as @lid or @s.whatsapp.net).
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

// lidToPhone resolves the phone number (pn) of a lid; "" if unmapped.
func (r *ContactRepository) lidToPhone(ctx context.Context, lid string) string {
	var pn sql.NullString
	err := r.db.QueryRowContext(ctx, "SELECT pn FROM whatsmeow_lid_map WHERE lid = ? LIMIT 1", lid).Scan(&pn)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(pn.String)
}

// contactName looks up the best name for a their_jid, preferring full_name,
// then business_name, then push_name, then first_name.
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
