// Package registry keeps the catalog of features and wires them into the core.
// It is the mechanism that makes the system Open-Closed: adding a feature is
// just Add()-ing it in the composition root, no existing code changes.
package registry

import (
	"fmt"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Registry accumulates features and initializes them in a batch.
type Registry struct {
	features []ports.Feature
	log      ports.Logger
}

// New creates the registry (logger required, fail-fast).
func New(log ports.Logger) *Registry {
	if log == nil {
		panic("registry: logger is required")
	}
	return &Registry{log: log}
}

// Add enrolls a feature (chainable). Does not initialize it yet.
func (r *Registry) Add(f ports.Feature) *Registry {
	if f == nil {
		panic("registry: nil feature")
	}
	r.features = append(r.features, f)
	return r
}

// StartAll registers all features with the given dependencies.
// Fail-fast: the first feature that fails aborts the entire initialization.
func (r *Registry) StartAll(deps ports.FeatureDeps) error {
	for _, feature := range r.features {
		if err := feature.Register(deps); err != nil {
			return fmt.Errorf("registry: feature %q failed to start: %w", feature.Name(), err)
		}
		r.log.Info("feature registered", "feature", feature.Name())
	}
	return nil
}
