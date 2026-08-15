package domain

import "errors"

// MediaKind classifica o tipo de mídia de uma mensagem. É a chave de Strategy
// tanto no envio (qual *Message do protocolo construir) quanto na recepção.
type MediaKind string

const (
	MediaImage    MediaKind = "image"
	MediaVideo    MediaKind = "video"
	MediaAudio    MediaKind = "audio"
	MediaDocument MediaKind = "document"
)

// ErrInvalidMedia sinaliza metadados de mídia inválidos (fail-fast).
var ErrInvalidMedia = errors.New("domain: mídia inválida")

// ErrMediaNotFound é retornado quando uma mensagem não tem mídia para baixar.
var ErrMediaNotFound = errors.New("domain: mensagem sem mídia")

// MediaSpec são os campos para construir um Media (named fields evitam um
// construtor com dez parâmetros posicionais, legibilidade sem perder imutabilidade).
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

// Media é um Value Object imutável com os metadados de uma mídia do WhatsApp,
// suficientes para persistir e, depois, baixar e descriptografar o conteúdo.
// Os campos []byte são tratados como somente-leitura (não há setters).
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

// NewMedia constrói um Media (fail-fast: o tipo é obrigatório).
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

// Kind devolve o tipo da mídia.
func (m Media) Kind() MediaKind { return m.kind }

// Filename devolve o nome do arquivo.
func (m Media) Filename() string { return m.filename }

// Mimetype devolve o mimetype (preenchido no envio; vazio na recepção).
func (m Media) Mimetype() string { return m.mimetype }

// URL devolve a URL criptografada na CDN do WhatsApp.
func (m Media) URL() string { return m.url }

// DirectPath devolve o caminho direto (do proto) usado para baixar com confiança.
func (m Media) DirectPath() string { return m.directPath }

// MediaKey devolve a chave de descriptografia (somente-leitura).
func (m Media) MediaKey() []byte { return m.mediaKey }

// FileSHA256 devolve o hash do arquivo decifrado (somente-leitura).
func (m Media) FileSHA256() []byte { return m.fileSHA256 }

// FileEncSHA256 devolve o hash do arquivo cifrado (somente-leitura).
func (m Media) FileEncSHA256() []byte { return m.fileEncSHA256 }

// FileLength devolve o tamanho do arquivo em bytes.
func (m Media) FileLength() uint64 { return m.fileLength }

// IsDownloadable indica se há metadados completos para baixar e decifrar a mídia
// (espelha a checagem do bridge legado: sem qualquer um destes, não dá para baixar).
func (m Media) IsDownloadable() bool {
	return m.url != "" &&
		len(m.mediaKey) > 0 &&
		len(m.fileSHA256) > 0 &&
		len(m.fileEncSHA256) > 0 &&
		m.fileLength > 0
}
