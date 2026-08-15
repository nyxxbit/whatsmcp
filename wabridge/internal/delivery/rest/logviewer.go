// Package rest contém os handlers HTTP de entrega (delivery layer).
package rest

import (
	"io"
	"net/http"
	"os"
)

const defaultTailBytes = 64 << 10 // 64 KiB

// LogViewer serve o final do arquivo de log no navegador (GET /logs), o fim do
// hábito de abrir o .log gigante no Notepad. Lê apenas o tail (saída limitada).
type LogViewer struct {
	path     string
	tailSize int64
}

// NewLogViewer cria o handler (fail-fast em path vazio).
func NewLogViewer(path string, tailSize int64) *LogViewer {
	if path == "" {
		panic("rest: LogViewer exige path do log")
	}
	if tailSize <= 0 {
		tailSize = defaultTailBytes
	}
	return &LogViewer{path: path, tailSize: tailSize}
}

// ServeHTTP implementa http.Handler servindo as últimas linhas do log.
func (v *LogViewer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	file, err := os.Open(v.path)
	if err != nil {
		http.Error(w, "log indisponível", http.StatusNotFound)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "log indisponível", http.StatusInternalServerError)
		return
	}
	if info.Size() > v.tailSize {
		if _, err := file.Seek(info.Size()-v.tailSize, io.SeekStart); err != nil {
			http.Error(w, "log indisponível", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, file)
}
