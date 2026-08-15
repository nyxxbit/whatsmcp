package wa

import (
	"context"
	"os"
	"time"

	"rsc.io/qr"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// pairWithQR runs QR pairing: saves the PNG and publishes QRCodeReady (the
// tray opens the image; the domain layer knows nothing about UI). Blocks
// until success/timeout/error.
func (c *Client) pairWithQR() {
	qrChan, err := c.wm.GetQRChannel(context.Background())
	if err != nil {
		c.log.Error("get QR channel", "err", err)
		return
	}
	if err := c.wm.Connect(); err != nil {
		c.log.Error("connect for QR", "err", err)
		return
	}
	published := false // publishes QRCodeReady only on the first code (the QR rotates ~30s)
	for evt := range qrChan {
		switch evt.Event {
		case "code":
			code, qerr := qr.Encode(evt.Code, qr.M)
			if qerr != nil {
				c.log.Error("encode QR", "err", qerr)
				continue
			}
			if werr := os.WriteFile(c.qrPath, code.PNG(), 0o644); werr != nil {
				c.log.Error("save QR", "err", werr)
				continue
			}
			if !published {
				c.publish(domain.NewQRCodeReady(c.qrPath, time.Now()))
				published = true
				c.log.Info("QR generated, scan it in WhatsApp > Linked devices", "file", c.qrPath)
			}
		case "success":
			c.log.Info("QR paired successfully")
			_ = os.Remove(c.qrPath)
			return
		case "timeout":
			c.log.Warn("QR expired, click Connect to generate another one")
			return
		case "error":
			c.log.Error("error in QR flow", "err", evt.Error)
			return
		}
	}
}
