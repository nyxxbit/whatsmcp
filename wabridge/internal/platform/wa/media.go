package wa

import (
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/binary/proto"

	"github.com/nyxxbit/wabridge/internal/core/domain"
)

// mediaStrategy maps the file extension to whatsmeow's media type and the
// correct mimetype (so WhatsApp can preview it). Identical to the legacy switch.
func mediaStrategy(ext string) (whatsmeow.MediaType, string) {
	switch strings.ToLower(ext) {
	case "jpg", "jpeg":
		return whatsmeow.MediaImage, "image/jpeg"
	case "png":
		return whatsmeow.MediaImage, "image/png"
	case "gif":
		return whatsmeow.MediaImage, "image/gif"
	case "webp":
		return whatsmeow.MediaImage, "image/webp"
	case "ogg":
		return whatsmeow.MediaAudio, "audio/ogg; codecs=opus"
	case "mp4":
		return whatsmeow.MediaVideo, "video/mp4"
	case "avi":
		return whatsmeow.MediaVideo, "video/avi"
	case "mov":
		return whatsmeow.MediaVideo, "video/quicktime"
	case "pdf":
		return whatsmeow.MediaDocument, "application/pdf"
	case "xlsx":
		return whatsmeow.MediaDocument, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "xls":
		return whatsmeow.MediaDocument, "application/vnd.ms-excel"
	case "docx":
		return whatsmeow.MediaDocument, "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "doc":
		return whatsmeow.MediaDocument, "application/msword"
	default:
		return whatsmeow.MediaDocument, "application/octet-stream"
	}
}

// extractMedia translates the media of a protocol message into domain.Media
// (nil when there is no media). Already includes the direct_path from the
// proto (reliable download).
func extractMedia(msg *waProto.Message) *domain.Media {
	if msg == nil {
		return nil
	}
	stamp := time.Now().Format("20060102_150405")
	switch {
	case msg.GetImageMessage() != nil:
		m := msg.GetImageMessage()
		return buildMedia(domain.MediaImage, "image_"+stamp+".jpg", m.GetURL(), m.GetDirectPath(),
			m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetFileLength())
	case msg.GetVideoMessage() != nil:
		m := msg.GetVideoMessage()
		return buildMedia(domain.MediaVideo, "video_"+stamp+".mp4", m.GetURL(), m.GetDirectPath(),
			m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetFileLength())
	case msg.GetAudioMessage() != nil:
		m := msg.GetAudioMessage()
		return buildMedia(domain.MediaAudio, "audio_"+stamp+".ogg", m.GetURL(), m.GetDirectPath(),
			m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetFileLength())
	case msg.GetDocumentMessage() != nil:
		m := msg.GetDocumentMessage()
		filename := m.GetFileName()
		if filename == "" {
			filename = "document_" + stamp
		}
		return buildMedia(domain.MediaDocument, filename, m.GetURL(), m.GetDirectPath(),
			m.GetMediaKey(), m.GetFileSHA256(), m.GetFileEncSHA256(), m.GetFileLength())
	}
	return nil
}

func buildMedia(kind domain.MediaKind, filename, url, directPath string, key, sha, encSHA []byte, length uint64) *domain.Media {
	media, err := domain.NewMedia(domain.MediaSpec{
		Kind: kind, Filename: filename, URL: url, DirectPath: directPath,
		MediaKey: key, FileSHA256: sha, FileEncSHA256: encSHA, FileLength: length,
	})
	if err != nil {
		return nil
	}
	return &media
}

// mediaKindToWA converts the domain type into whatsmeow's enum (for download).
func mediaKindToWA(kind domain.MediaKind) (whatsmeow.MediaType, bool) {
	switch kind {
	case domain.MediaImage:
		return whatsmeow.MediaImage, true
	case domain.MediaVideo:
		return whatsmeow.MediaVideo, true
	case domain.MediaAudio:
		return whatsmeow.MediaAudio, true
	case domain.MediaDocument:
		return whatsmeow.MediaDocument, true
	default:
		return "", false
	}
}

// downloadable implements whatsmeow.DownloadableMessage from a domain.Media.
type downloadable struct {
	url, directPath          string
	mediaKey, sha256, encSHA []byte
	length                   uint64
	mediaType                whatsmeow.MediaType
}

func (d downloadable) GetDirectPath() string             { return d.directPath }
func (d downloadable) GetURL() string                    { return d.url }
func (d downloadable) GetMediaKey() []byte               { return d.mediaKey }
func (d downloadable) GetFileLength() uint64             { return d.length }
func (d downloadable) GetFileSHA256() []byte             { return d.sha256 }
func (d downloadable) GetFileEncSHA256() []byte          { return d.encSHA }
func (d downloadable) GetMediaType() whatsmeow.MediaType { return d.mediaType }
