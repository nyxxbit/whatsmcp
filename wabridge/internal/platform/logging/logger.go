// Package logging provides the implementation of the ports.Logger Port using
// log/slog (structured), with size-based rotation and a Null Object for tests.
package logging

import (
	"io"
	"log/slog"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// SlogLogger adapts log/slog to the ports.Logger contract (Adapter pattern).
type SlogLogger struct {
	l *slog.Logger
}

var _ ports.Logger = (*SlogLogger)(nil)

// New creates a structured logger writing to w (use RotatingFile in production).
func New(w io.Writer) *SlogLogger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &SlogLogger{l: slog.New(handler)}
}

// Info logs at the informational level.
func (s *SlogLogger) Info(msg string, args ...any) { s.l.Info(msg, args...) }

// Warn logs at the warning level.
func (s *SlogLogger) Warn(msg string, args ...any) { s.l.Warn(msg, args...) }

// Error logs at the error level.
func (s *SlogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }

// With derives a logger with fixed fields (context), without mutating the original.
func (s *SlogLogger) With(args ...any) ports.Logger { return &SlogLogger{l: s.l.With(args...)} }
