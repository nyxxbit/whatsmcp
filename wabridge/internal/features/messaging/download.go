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

// Downloader is the media download use case: it resolves metadata from the
// repository, uses an on-disk cache, and only downloads (via MediaFetcher)
// when needed. Composes a Repository (persistence) with a Fetcher (network):
// Single Responsibility.
type Downloader struct {
	repo    ports.MessageRepository
	fetcher ports.MediaFetcher
	baseDir string
	log     ports.Logger
}

var _ ports.MediaDownloader = (*Downloader)(nil)

// NewDownloader creates the use case (fail-fast on dependencies).
func NewDownloader(repo ports.MessageRepository, fetcher ports.MediaFetcher, baseDir string, log ports.Logger) *Downloader {
	if repo == nil || fetcher == nil || log == nil {
		panic("messaging: Downloader requires repo, fetcher, and log")
	}
	if baseDir == "" {
		baseDir = "store"
	}
	return &Downloader{repo: repo, fetcher: fetcher, baseDir: baseDir, log: log}
}

// Download ensures the media is on disk and returns the result. Positive
// programming: explicit errors (missing media, incomplete metadata,
// network/IO failure).
func (d *Downloader) Download(ctx context.Context, messageID, chatJID string) (domain.DownloadResult, error) {
	media, err := d.repo.FindMedia(ctx, messageID, chatJID)
	if err != nil {
		return domain.DownloadResult{}, err // includes domain.ErrMediaNotFound
	}

	chatDir := filepath.Join(d.baseDir, strings.ReplaceAll(chatJID, ":", "_"))
	localPath := filepath.Join(chatDir, media.Filename())
	absPath, err := filepath.Abs(localPath)
	if err != nil {
		return domain.DownloadResult{}, fmt.Errorf("messaging: absolute path: %w", err)
	}

	// Cache hit: already downloaded before.
	if _, err := os.Stat(localPath); err == nil {
		return domain.NewDownloadResult(media.Kind(), media.Filename(), absPath), nil
	}

	if !media.IsDownloadable() {
		return domain.DownloadResult{}, fmt.Errorf("messaging: incomplete media metadata for download")
	}
	if err := os.MkdirAll(chatDir, 0o755); err != nil {
		return domain.DownloadResult{}, fmt.Errorf("messaging: create chat folder: %w", err)
	}
	data, err := d.fetcher.Fetch(ctx, media)
	if err != nil {
		return domain.DownloadResult{}, err
	}
	if err := os.WriteFile(localPath, data, 0o644); err != nil {
		return domain.DownloadResult{}, fmt.Errorf("messaging: save media: %w", err)
	}
	d.log.Info("media downloaded", "type", media.Kind(), "file", absPath, "bytes", len(data))
	return domain.NewDownloadResult(media.Kind(), media.Filename(), absPath), nil
}
