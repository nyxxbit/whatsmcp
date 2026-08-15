package logging

import "github.com/nyxxbit/wabridge/internal/core/ports"

// Noop is a Logger that discards everything: Null Object Pattern (GoF). Useful
// in tests and wherever logging is optional, avoiding nil checks scattered
// around the codebase (positive programming).
type Noop struct{}

var _ ports.Logger = Noop{}

// Info discards.
func (Noop) Info(string, ...any) {}

// Warn discards.
func (Noop) Warn(string, ...any) {}

// Error discards.
func (Noop) Error(string, ...any) {}

// With returns the Null Object itself.
func (Noop) With(...any) ports.Logger { return Noop{} }
