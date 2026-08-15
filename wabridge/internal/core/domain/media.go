package domain

import "errors"

// MediaKind classifies the media type of a message. It is the Strategy key
// both on send (which protocol *Message to build) and on receive.
type MediaKind string

const (
	MediaImage    MediaKind = "image"
	MediaVideo    MediaKind = "video"
	MediaAudio    MediaKind = "audio"
	MediaDocument MediaKind = "document"
)

// ErrInvalidMedia signals invalid media metadata (fail-fast).
var ErrInvalidMedia = errors.New("domain: invalid media")

// ErrMediaNotFound is returned when a message has no media to download.
var ErrMediaNotFound = errors.New("domain: message has no media")

// MediaSpec holds the fields to build a Media (named fields avoid a
// constructor with ten positional parameters, readability without losing immutability).
type MediaSpec struct {
	Kind          MediaKind
	Filename      string
	Mimetype      string
	URL           string
	DirectPath    string
	MediaKey      []byte
	FileSHA256    []byte
	FileEncSHA256 []byte
	FileLength    uint64
}

// Media is an immutable Value Object with the metadata of a WhatsApp media item,
// enough to persist and later download and decrypt the content.
// The []byte fields are treated as read-only (there are no setters).
type Media struct {
	kind          MediaKind
	filename      string
	mimetype      string
	url           string
	directPath    string
	mediaKey      []byte
	fileSHA256    []byte
	fileEncSHA256 []byte
	fileLength    uint64
}

// NewMedia builds a Media (fail-fast: the kind is required).
func NewMedia(s MediaSpec) (Media, error) {
	if s.Kind == "" {
		return Media{}, ErrInvalidMedia
	}
	return Media{
		kind:          s.Kind,
		filename:      s.Filename,
		mimetype:      s.Mimetype,
		url:           s.URL,
		directPath:    s.DirectPath,
		mediaKey:      s.MediaKey,
		fileSHA256:    s.FileSHA256,
		fileEncSHA256: s.FileEncSHA256,
		fileLength:    s.FileLength,
	}, nil
}

// Kind returns the media's kind.
func (m Media) Kind() MediaKind { return m.kind }

// Filename returns the file name.
func (m Media) Filename() string { return m.filename }

// Mimetype returns the mimetype (filled in on send; empty on receive).
func (m Media) Mimetype() string { return m.mimetype }

// URL returns the encrypted URL on WhatsApp's CDN.
func (m Media) URL() string { return m.url }

// DirectPath returns the direct path (from the proto) used to download reliably.
func (m Media) DirectPath() string { return m.directPath }

// MediaKey returns the decryption key (read-only).
func (m Media) MediaKey() []byte { return m.mediaKey }

// FileSHA256 returns the hash of the decrypted file (read-only).
func (m Media) FileSHA256() []byte { return m.fileSHA256 }

// FileEncSHA256 returns the hash of the encrypted file (read-only).
func (m Media) FileEncSHA256() []byte { return m.fileEncSHA256 }

// FileLength returns the file size in bytes.
func (m Media) FileLength() uint64 { return m.fileLength }

// IsDownloadable reports whether there is complete metadata to download and decrypt the media
// (mirrors the legacy bridge's check: without any one of these, it cannot be downloaded).
func (m Media) IsDownloadable() bool {
	return m.url != "" &&
		len(m.mediaKey) > 0 &&
		len(m.fileSHA256) > 0 &&
		len(m.fileEncSHA256) > 0 &&
		m.fileLength > 0
}
