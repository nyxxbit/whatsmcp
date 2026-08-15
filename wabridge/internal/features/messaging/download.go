package messaging

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nyxxbit/wabridge/internal/core/domain"
	"github.com/nyxxbit/wabridge/internal/core/ports"
)

// Downloader é o caso de uso de download de mídia: resolve os metadados no
// repositório, usa cache em disco e só baixa (via MediaFetcher) quando preciso.
// Compõe um Repository (persistência) com um Fetcher (rede): Single Responsibility.
type Downloader struct {
	repo    ports.MessageRepository
	fetcher ports.MediaFetcher
	baseDir string
	log     ports.Logger
}

var _ ports.MediaDownloader = (*Downloader)(nil)

// NewDownloader cria o caso de uso (fail-fast nas dependências).
func NewDownloader(repo ports.MessageRepository, fetcher ports.MediaFetcher, baseDir string, log ports.Logger) *Downloader {
	if repo == nil || fetcher == nil || log == nil {
		panic("messaging: Downloader exige repo, fetcher e log")
	}
	if baseDir == "" {
		baseDir = "store"
	}
	return &Downloader{repo: repo, fetcher: fetcher, baseDir: baseDir, log: log}
}

// Download garante a mídia em disco e devolve o resultado. Programação positiva:
// erros explícitos (mídia inexistente, metadados incompletos, falha de rede/IO).
func (d *Downloader) Download(ctx context.Context, messageID, chatJID string) (domain.DownloadResult, error) {
	media, err := d.repo.FindMedia(ctx, messageID, chatJID)
	if err != nil {
		return domain.DownloadResult{}, err // inclui domain.ErrMediaNotFound
	}

	chatDir := filepath.Join(d.baseDir, strings.ReplaceAll(chatJID, ":", "_"))
	localPath := filepath.Join(chatDir, media.Filename())
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return domain.DownloadResult{}, fmt.Errorf("messaging: caminho absoluto: %w", err)
	}

	// Cache hit: já baixado antes.
	if _, err := os.Stat(localPath); err == nil {
		return domain.NewDownloadResult(media.Kind(), media.Filename(), absPath), nil
	}

	if !media.IsDownloadable() {
		return domain.DownloadResult{}, fmt.Errorf("messaging: metadados de mídia incompletos para download")
	}
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		return domain.DownloadResult{}, fmt.Errorf("messaging: criar pasta da conversa: %w", err)
	}
	data, err := d.fetcher.Fetch(ctx, media)
	if err != nil {
		return domain.DownloadResult{}, err
	}
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return domain.DownloadResult{}, fmt.Errorf("messaging: salvar mídia: %w", err)
	}
	d.log.Info("mídia baixada", "tipo", media.Kind(), "arquivo", absPath, "bytes", len(data))
	return domain.NewDownloadResult(media.Kind(), media.Filename(), absPath), nil
}
