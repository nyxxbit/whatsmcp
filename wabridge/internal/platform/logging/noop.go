package logging

import "github.com/nyxxbit/wabridge/internal/core/ports"

// Noop é um Logger que descarta tudo: Null Object Pattern (GoF). Útil em testes
// e onde o log é opcional, evitando checagens de nil espalhadas (programação positiva).
type Noop struct{}

var _ ports.Logger = Noop{}

// Info descarta.
func (Noop) Info(string, ...any) {}

// Warn descarta.
func (Noop) Warn(string, ...any) {}

// Error descarta.
func (Noop) Error(string, ...any) {}

// With devolve o próprio Null Object.
func (Noop) With(...any) ports.Logger { return Noop{} }
