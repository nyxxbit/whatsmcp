// Package contacts é a feature (bounded context) de contatos: resolve a
// identidade COMPLETA de um JID (nome + número + JID cru) e enriquece os logs/
// eventos com tudo isso, visão ampla, sem trocar um dado pelo outro.
package contacts

import (
	"context"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Resolver é um use case (Application Service) que resolve a identidade de um JID.
type Resolver struct {
	repo ports.ContactRepository
}

// NewResolver cria o use case (dependência obrigatória, fail-fast).
func NewResolver(repo ports.ContactRepository) *Resolver {
	if repo == nil {
		panic("contacts: ContactRepository é obrigatório")
	}
	return &Resolver{repo: repo}
}

// Identify devolve a identidade completa (nome + número + JID). Programação
// positiva: nunca quebra o fluxo de quem chama, em falha de infra, cai numa
// identidade só com o JID (sempre exibível).
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

// Resolve devolve uma string única "Nome (número)" (ou o que houver), conveniência
// para usos de uma linha. Para visão ampla com campos separados, use Identify.
func (r *Resolver) Resolve(ctx context.Context, jid domain.JID) string {
	return r.Identify(ctx, jid).Display()
}
