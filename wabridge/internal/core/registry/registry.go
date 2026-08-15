// Package registry mantém o catálogo de features e as conecta ao núcleo.
// É o mecanismo que torna o sistema Open-Closed: adicionar uma feature é
// apenas Add()-á-la no composition root, nenhum código existente muda.
package registry

import (
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Registry acumula features e as inicializa em bloco.
type Registry struct {
	features []ports.Feature
	log      ports.Logger
}

// New cria o registry (logger obrigatório, fail-fast).
func New(log ports.Logger) *Registry {
	if log == nil {
		panic("registry: logger é obrigatório")
	}
	return &Registry{log: log}
}

// Add inscreve uma feature (encadeável). Ainda não a inicializa.
func (r *Registry) Add(f ports.Feature) *Registry {
	if f == nil {
		panic("registry: feature nil")
	}
	r.features = append(r.features, f)
	return r
}

// StartAll registra todas as features com as dependências dadas.
// Fail-fast: a primeira feature que falhar aborta toda a inicialização.
func (r *Registry) StartAll(deps ports.FeatureDeps) error {
	for _, feature := range r.features {
		if err := feature.Register(deps); err != nil {
			return fmt.Errorf("registry: feature %q falhou ao iniciar: %w", feature.Name(), err)
		}
		r.log.Info("feature registrada", "feature", feature.Name())
	}
	return nil
}
