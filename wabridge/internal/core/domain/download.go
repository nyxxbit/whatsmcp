package domain

// DownloadResult é um Value Object imutável com o resultado de um download de
// mídia já salvo em disco: tipo, nome e caminho absoluto do arquivo.
type DownloadResult struct {
	kind     MediaKind
	filename string
	path     string
}

// NewDownloadResult cria o resultado.
func NewDownloadResult(kind MediaKind, filename, path string) DownloadResult {
	return DownloadResult{kind: kind, filename: filename, path: path}
}

// Kind devolve o tipo de mídia baixada.
func (d DownloadResult) Kind() MediaKind { return d.kind }

// Filename devolve o nome do arquivo.
func (d DownloadResult) Filename() string { return d.filename }

// Path devolve o caminho absoluto do arquivo salvo.
func (d DownloadResult) Path() string { return d.path }
