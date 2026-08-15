package wa

import (
	"context"
	"fmt"
	"strings"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// Fetch baixa e descriptografa os bytes de uma mídia a partir dos seus metadados
// (camada baixa do download; o caso de uso de alto nível mora na feature messaging).
func (c *Client) Fetch(ctx context.Context, media domain.Media) ([]byte, error) {
	waType, ok := mediaKindToWA(media.Kind())
	if !ok {
		return nil, fmt.Errorf("wa: tipo de mídia não suportado: %s", media.Kind())
	}
	directPath := media.DirectPath()
	if directPath == "" {
		directPath = extractDirectPathFromURL(media.URL()) // fallback do legado
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
		return nil, fmt.Errorf("wa: baixar mídia: %w", err)
	}
	return data, nil
}

// extractDirectPathFromURL deriva o direct_path da URL quando o real não foi salvo.
func extractDirectPathFromURL(url string) string {
	parts := strings.SplitN(url, ".net/", 2)
	if len(parts) < 2 {
		return url
	}
	return "/" + strings.SplitN(parts[1], "?", 2)[0]
}
