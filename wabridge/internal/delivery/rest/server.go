// Package rest is the HTTP delivery layer: it recreates EXACTLY the contract
// of the legacy bridge (/api/send, /api/download, /api/sync-labels, /logs) on
// top of the core's ports. Uses its own ServeMux (no global state) and depends
// only on interfaces.
package rest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Config gathers the API's dependencies (injected at the composition root).
type Config struct {
	Sender      ports.MessageSender
	Downloader  ports.MediaDownloader
	LabelSyncer ports.LabelSyncer
	LogPath     string
	Log         ports.Logger
}

// Server exposes the REST API. Depends only on ports (DIP).
type Server struct {
	sender      ports.MessageSender
	downloader  ports.MediaDownloader
	labelSyncer ports.LabelSyncer
	logViewer   *LogViewer
	log         ports.Logger
}

// NewServer builds the server (fail-fast on required dependencies).
func NewServer(cfg Config) (*Server, error) {
	if cfg.Sender == nil || cfg.Downloader == nil || cfg.LabelSyncer == nil || cfg.Log == nil {
		return nil, fmt.Errorf("rest: Sender, Downloader, LabelSyncer, and Log are required")
	}
	if cfg.LogPath == "" {
		return nil, fmt.Errorf("rest: LogPath is required for /logs")
	}
	return &Server{
		sender:      cfg.Sender,
		downloader:  cfg.Downloader,
		labelSyncer: cfg.LabelSyncer,
		logViewer:   NewLogViewer(cfg.LogPath, 0),
		log:         cfg.Log,
	}, nil
}

// Handler sets up the routing. Unmapped paths (including "/") return 404:
// that is what existing health checks rely on ("404 means the server is up").
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/download", s.handleDownload)
	mux.HandleFunc("/api/sync-labels", s.handleSyncLabels)
	mux.Handle("/logs", s.logViewer)
	return mux
}

// Start brings up the HTTP server (blocking; run it in a goroutine from the
// composition root).
func (s *Server) Start(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second, // mitigates slowloris (best practice)
	}
	s.log.Info("REST is up", "addr", addr)
	return srv.ListenAndServe()
}
