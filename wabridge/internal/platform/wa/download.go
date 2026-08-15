package wa

import (
	"context"
	"fmt"
	"strings"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// Fetch downloads and decrypts a media's bytes from its metadata (the
// low-level download layer; the high-level use case lives in the messaging
// feature).
func (c *Client) Fetch(ctx context.Context, media domain.Media) ([]byte, error) {
	waType, ok := mediaKindToWA(media.Kind())
	if !ok {
		return nil, fmt.Errorf("wa: unsupported media type: %s", media.Kind())
	}
	directPath := media.DirectPath()
	if directPath == "" {
		directPath = extractDirectPathFromURL(media.URL()) // legacy fallback
	}
	dl := downloadable{
		url:        media.URL(),
		directPath: directPath,
		mediaKey:   media.MediaKey(),
		sha256:     media.FileSHA256(),
		encSHA:     media.FileEncSHA256(),
		length:     media.FileLength(),
		mediaType:  waType,
	}
	data, err := c.wm.Download(ctx, dl)
	if err != nil {
		return nil, fmt.Errorf("wa: download media: %w", err)
	}
	return data, nil
}

// extractDirectPathFromURL derives the direct_path from the URL when the real one wasn't saved.
func extractDirectPathFromURL(url string) string {
	parts := strings.SplitN(url, ".net/", 2)
	if len(parts) < 2 {
		return url
	}
	return "/" + strings.SplitN(parts[1], "?", 2)[0]
}
