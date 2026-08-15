package domain

// DownloadResult is an immutable Value Object with the result of a media
// download already saved to disk: kind, name and absolute file path.
type DownloadResult struct {
	kind     MediaKind
	filename string
	path     string
}

// NewDownloadResult creates the result.
func NewDownloadResult(kind MediaKind, filename, path string) DownloadResult {
	return DownloadResult{kind: kind, filename: filename, path: path}
}

// Kind returns the kind of media downloaded.
func (d DownloadResult) Kind() MediaKind { return d.kind }

// Filename returns the file name.
func (d DownloadResult) Filename() string { return d.filename }

// Path returns the absolute path of the saved file.
func (d DownloadResult) Path() string { return d.path }
