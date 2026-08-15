// Package logging fornece a implementação do Port ports.Logger usando log/slog
// (estruturado), com rotação por tamanho e um Null Object para testes.
package logging

import (
	"io"
	"log/slog"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// SlogLogger adapta log/slog ao contrato ports.Logger (padrão Adapter).
type SlogLogger struct {
	l *slog.Logger
}

var _ ports.Logger = (*SlogLogger)(nil)

// New cria um logger estruturado escrevendo em w (use RotatingFile em produção).
func New(w io.Writer) *SlogLogger {
	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelInfo})
	return &SlogLogger{l: slog.New(handler)}
}

// Info registra em nível informativo.
func (s *SlogLogger) Info(msg string, args ...any) { s.l.Info(msg, args...) }

// Warn registra em nível de alerta.
func (s *SlogLogger) Warn(msg string, args ...any) { s.l.Warn(msg, args...) }

// Error registra em nível de erro.
func (s *SlogLogger) Error(msg string, args ...any) { s.l.Error(msg, args...) }

// With deriva um logger com campos fixos (contexto), sem mutar o original.
func (s *SlogLogger) With(args ...any) ports.Logger { return &SlogLogger{l: s.l.With(args...)} }
