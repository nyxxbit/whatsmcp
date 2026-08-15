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

// SendText envia uma mensagem de texto simples.
func (c *Client) SendText(ctx context.Context, to domain.JID, body string) error {
	rjid, err := c.recipient(to)
	if err != nil {
		return err
	}
	if !c.wm.IsConnected() {
		return fmt.Errorf("wa: não conectado ao WhatsApp")
	}
	if _, err := c.wm.SendMessage(ctx, rjid, &waProto.Message{Conversation: proto.String(body)}); err != nil {
		return fmt.Errorf("wa: enviar texto: %w", err)
	}
	return nil
}

// SendMedia envia um arquivo (imagem/áudio/vídeo/documento) com legenda opcional;
// o tipo é inferido pela extensão (Strategy). Porta o sendWhatsAppMessage legado.
func (c *Client) SendMedia(ctx context.Context, to domain.JID, mediaPath, caption string) error {
	rjid, err := c.recipient(to)
	if err != nil {
		return err
	}
	if !c.wm.IsConnected() {
		return fmt.Errorf("wa: não conectado ao WhatsApp")
	}
	data, err := os.ReadFile(mediaPath)
	if err != nil {
		return fmt.Errorf("wa: ler mídia %q: %w", mediaPath, err)
	}
	ext := ""
	if i := strings.LastIndex(mediaPath, "."); i >= 0 {
		ext = mediaPath[i+1:]
	}
	mediaType, mimeType := mediaStrategy(ext)
	resp, err := c.wm.Upload(ctx, data, mediaType)
	if err != nil {
		return fmt.Errorf("wa: subir mídia: %w", err)
	}
	msg, err := buildMediaMessage(mediaType, mimeType, caption, mediaPath, data, resp)
	if err != nil {
		return err
	}
	if _, err := c.wm.SendMessage(ctx, rjid, msg); err != nil {
		return fmt.Errorf("wa: enviar mídia: %w", err)
	}
	return nil
}

// recipient converte um domain.JID no types.JID do whatsmeow (aceita PN, @lid, grupo).
func (c *Client) recipient(to domain.JID) (types.JID, error) {
	if to.IsZero() {
		return types.JID{}, fmt.Errorf("wa: destinatário vazio")
	}
	jid, err := types.ParseJID(to.String())
	if err != nil {
		return types.JID{}, fmt.Errorf("wa: JID inválido %q: %w", to.String(), err)
	}
	return jid, nil
}

// buildMediaMessage monta o *waProto.Message certo para cada tipo de mídia.
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
				return nil, fmt.Errorf("wa: analisar Ogg Opus: %w", err)
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
		fileName := filepath.Base(mediaPath) // lida com / e \ (evita "Untitled")
		msg.DocumentMessage = &waProto.DocumentMessage{
			Title: proto.String(fileName), FileName: proto.String(fileName), Caption: proto.String(caption),
			Mimetype: proto.String(mimeType), URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
		}
	}
	return msg, nil
}
