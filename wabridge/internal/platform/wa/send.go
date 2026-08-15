package wa

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// SendText sends a simple text message.
func (c *Client) SendText(ctx context.Context, to domain.JID, body string) error {
	rjid, err := c.recipient(to)
	if err != nil {
		return err
	}
	if !c.wm.IsConnected() {
		return fmt.Errorf("wa: not connected to WhatsApp")
	}
	if _, err := c.wm.SendMessage(ctx, rjid, &waProto.Message{Conversation: proto.String(body)}); err != nil {
		return fmt.Errorf("wa: send text: %w", err)
	}
	return nil
}

// SendMedia sends a file (image/audio/video/document) with an optional
// caption; the type is inferred from the extension (Strategy). Ports the
// legacy sendWhatsAppMessage.
func (c *Client) SendMedia(ctx context.Context, to domain.JID, mediaPath, caption string) error {
	rjid, err := c.recipient(to)
	if err != nil {
		return err
	}
	if !c.wm.IsConnected() {
		return fmt.Errorf("wa: not connected to WhatsApp")
	}
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("wa: read media %q: %w", mediaPath, err)
	}
	ext := ""
	if i := strings.LastIndex(mediaPath, "."); i >= 0 {
		ext = mediaPath[i+1:]
	}
	mediaType, mimeType := mediaStrategy(ext)
	resp, err := c.wm.Upload(ctx, data, mediaType)
	if err != nil {
		return fmt.Errorf("wa: upload media: %w", err)
	}
	msg, err := buildMediaMessage(mediaType, mimeType, caption, mediaPath, data, resp)
	if err != nil {
		return err
	}
	if _, err := c.wm.SendMessage(ctx, rjid, msg); err != nil {
		return fmt.Errorf("wa: send media: %w", err)
	}
	return nil
}

// recipient converts a domain.JID into whatsmeow's types.JID (accepts PN, @lid, group).
func (c *Client) recipient(to domain.JID) (types.JID, error) {
	if to.IsZero() {
		return types.JID{}, fmt.Errorf("wa: empty recipient")
	}
	jid, err := types.ParseJID(to.String())
	if err != nil {
		return types.JID{}, fmt.Errorf("wa: invalid JID %q: %w", to.String(), err)
	}
	return jid, nil
}

// buildMediaMessage builds the right *waProto.Message for each media type.
func buildMediaMessage(mediaType whatsmeow.MediaType, mimeType, caption, mediaPath string, data []byte, resp whatsmeow.UploadResponse) (*waProto.Message, error) {
	msg := &waProto.Message{}
	switch mediaType {
	case whatsmeow.MediaImage:
		msg.ImageMessage = &waProto.ImageMessage{
			Caption: proto.String(caption), Mimetype: proto.String(mimeType),
			URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
		}
	case whatsmeow.MediaAudio:
		seconds := uint32(30)
		var waveform []byte
		if strings.Contains(mimeType, "ogg") {
			s, w, err := analyzeOggOpus(data)
			if err != nil {
				return nil, fmt.Errorf("wa: analyze Ogg Opus: %w", err)
			}
			seconds, waveform = s, w
		}
		msg.AudioMessage = &waProto.AudioMessage{
			Mimetype: proto.String(mimeType), URL: &resp.URL, DirectPath: &resp.DirectPath,
			MediaKey: resp.MediaKey, FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256,
			FileLength: &resp.FileLength, Seconds: proto.Uint32(seconds), PTT: proto.Bool(true), Waveform: waveform,
		}
	case whatsmeow.MediaVideo:
		msg.VideoMessage = &waProto.VideoMessage{
			Caption: proto.String(caption), Mimetype: proto.String(mimeType),
			URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
		}
	case whatsmeow.MediaDocument:
		fileName := filepath.Base(mediaPath) // handles / and \ (avoids "Untitled")
		msg.DocumentMessage = &waProto.DocumentMessage{
			Title: proto.String(fileName), FileName: proto.String(fileName), Caption: proto.String(caption),
			Mimetype: proto.String(mimeType), URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
		}
	}
	return msg, nil
}
