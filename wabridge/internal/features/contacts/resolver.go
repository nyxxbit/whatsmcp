// Package contacts is the contacts feature (bounded context): it resolves the
// FULL identity of a JID (name + number + raw JID) and enriches logs/events
// with all of it, a broad view, without trading one piece of data for another.
package contacts

import (
	"context"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Resolver is a use case (Application Service) that resolves the identity of a JID.
type Resolver struct {
	repo ports.ContactRepository
}

// NewResolver creates the use case (required dependency, fail-fast).
func NewResolver(repo ports.ContactRepository) *Resolver {
	if repo == nil {
		panic("contacts: ContactRepository is required")
	}
	return &Resolver{repo: repo}
}

// Identify returns the full identity (name + number + JID). Positive
// programming: it never breaks the caller's flow; on infra failure, it falls
// back to an identity with just the JID (always displayable).
func (r *Resolver) Identify(ctx context.Context, jid domain.JID) domain.Identity {
	id, err := r.repo.Identify(ctx, jid)
	if err != nil {
		fallback, ferr := domain.NewIdentity(jid, "", "")
		if ferr != nil {
			return domain.Identity{}
		}
		return fallback
	}
	return id
}

// Resolve returns a single "Name (number)" string (or whatever is available),
// a convenience for one-line uses. For a broad view with separate fields, use
// Identify.
func (r *Resolver) Resolve(ctx context.Context, jid domain.JID) string {
	return r.Identify(ctx, jid).Display()
}
