package wa

import (
	"context"
	"os"
	"time"

	"rsc.io/qr"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// pairWithQR roda o pareamento por QR: salva o PNG e publica QRCodeReady (a tray
// abre a imagem; o domínio não conhece UI). Bloqueia até sucesso/timeout/erro.
func (c *Client) pairWithQR() {
	qrChan, err := c.wm.GetQRChannel(context.Background())
	if err != nil {
		c.log.Error("obter canal de QR", "err", err)
		return
	}
	if err := c.wm.Connect(); err != nil {
		c.log.Error("conectar para QR", "err", err)
		return
	}
	published := false // publica QRCodeReady só no primeiro code (o QR rotaciona ~30s)
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			code, qerr := qr.Encode(evt.Code, qr.M)
			if qerr != nil {
				c.log.Error("codificar QR", "err", qerr)
				continue
			}
			if werr := os.WriteFile(c.qrPath, code.PNG(), 0o644); werr != nil {
				c.log.Error("salvar QR", "err", werr)
				continue
			}
			if !published {
				c.publish(domain.NewQRCodeReady(c.qrPath, time.Now()))
				published = true
				c.log.Info("QR gerado, escaneie no WhatsApp > Aparelhos conectados", "arquivo", c.qrPath)
			}
		case "success":
			c.log.Info("QR pareado com sucesso")
			_ = os.Remove(c.qrPath)
			return
		case "timeout":
			c.log.Warn("QR expirou, clique em Conectar para gerar outro")
			return
		case "error":
			c.log.Error("erro no fluxo de QR", "err", evt.Error)
			return
		}
	}
}
