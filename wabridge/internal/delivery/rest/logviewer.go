// Package rest contains the delivery layer's HTTP handlers.
package rest

import (
	"io"
	"net/http"
	"os"
)

const defaultTailBytes = 64 << 10 // 64 KiB

// LogViewer serves the tail of the log file in the browser (GET /logs),
// ending the habit of opening the giant .log file in Notepad. It only reads
// the tail (bounded output).
type LogViewer struct {
	path     string
	tailSize int64
}

// NewLogViewer creates the handler (fail-fast on an empty path).
func NewLogViewer(path string, tailSize int64) *LogViewer {
	if path == "" {
		panic("rest: LogViewer requires a log path")
	}
	if tailSize <= 0 {
		tailSize = defaultTailBytes
	}
	return &LogViewer{path: path, tailSize: tailSize}
}

// ServeHTTP implements http.Handler, serving the last lines of the log.
func (v *LogViewer) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	file, err := os.Open(v.path)
	if err != nil {
		http.Error(w, "log unavailable", http.StatusNotFound)
		return
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		http.Error(w, "log unavailable", http.StatusInternalServerError)
		return
	}
	if info.Size() > v.tailSize {
		if _, err := file.Seek(info.Size()-v.tailSize, io.SeekStart); err != nil {
			http.Error(w, "log unavailable", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, file)
}
