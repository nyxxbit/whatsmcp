// Package rest é a camada de entrega HTTP: recria EXATAMENTE o contrato do bridge
// legado (/api/send, /api/download, /api/sync-labels, /logs) sobre as portas do
// núcleo. Usa um ServeMux próprio (sem estado global) e só depende de interfaces.
package rest

import (
	"fmt"
	"net/http"
	"time"

	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Config reúne as dependências da API (injetadas no composition root).
type Config struct {
	Sender      ports.MessageSender
	Downloader  ports.MediaDownloader
	LabelSyncer ports.LabelSyncer
	LogPath     string
	Log         ports.Logger
}

// Server expõe a API REST. Depende apenas de ports (DIP).
type Server struct {
	sender      ports.MessageSender
	downloader  ports.MediaDownloader
	labelSyncer ports.LabelSyncer
	logViewer   *LogViewer
	log         ports.Logger
}

// NewServer monta o servidor (fail-fast nas dependências obrigatórias).
func NewServer(cfg Config) (*Server, error) {
	if cfg.Sender == nil || cfg.Downloader == nil || cfg.LabelSyncer == nil || cfg.Log == nil {
		return nil, fmt.Errorf("rest: Sender, Downloader, LabelSyncer e Log são obrigatórios")
	}
	if cfg.LogPath == "" {
		return nil, fmt.Errorf("rest: LogPath é obrigatório para o /logs")
	}
	return &Server{
		sender:      cfg.Sender,
		downloader:  cfg.Downloader,
		labelSyncer: cfg.LabelSyncer,
		logViewer:   NewLogViewer(cfg.LogPath, 0),
		log:         cfg.Log,
	}, nil
}

// Handler monta o roteamento. Caminhos não mapeados (inclusive "/") devolvem 404 -
// is what existing health checks rely on ("404 means the server is up").
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/send", s.handleSend)
	mux.HandleFunc("/api/download", s.handleDownload)
	mux.HandleFunc("/api/sync-labels", s.handleSyncLabels)
	mux.Handle("/logs", s.logViewer)
	return mux
}

// Start sobe o servidor HTTP (bloqueante; rode em goroutine no composition root).
func (s *Server) Start(addr string) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second, // mitiga slowloris (boa prática)
	}
	s.log.Info("REST no ar", "addr", addr)
	return srv.ListenAndServe()
}
