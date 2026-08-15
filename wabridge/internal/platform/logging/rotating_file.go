package logging

import (
	"os"
	"sync"
)

const defaultMaxLogBytes = 1 << 20 // 1 MiB

// RotatingFile é um io.Writer que rotaciona o arquivo ao ultrapassar maxBytes,
// guardando um backup ".1". Sem dependência externa (KISS), resolve de raiz o
// problema do log gigante que travava o Notepad (ver feedback "logs limitados").
type RotatingFile struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	size     int64
	file     *os.File
}

// NewRotatingFile abre/cria o arquivo de log com rotação (fail-fast em erro de IO).
func NewRotatingFile(path string, maxBytes int64) (*RotatingFile, error) {
	if maxBytes <= 0 {
		maxBytes = defaultMaxLogBytes
	}
	rf := &RotatingFile{path: path, maxBytes: maxBytes}
	if err := rf.open(); err != nil {
		return nil, err
	}
	return rf, nil
}

func (rf *RotatingFile) open() error {
	f, err := os.OpenFile(rf.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	rf.file, rf.size = f, info.Size()
	return nil
}

// Write implementa io.Writer, rotacionando antes de estourar o limite.
func (rf *RotatingFile) Write(p []byte) (int, error) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.size+int64(len(p)) > rf.maxBytes {
		if err := rf.rotate(); err != nil {
			return 0, err
		}
	}
	n, err := rf.file.Write(p)
	rf.size += int64(n)
	return n, err
}

func (rf *RotatingFile) rotate() error {
	_ = rf.file.Close()
	_ = os.Rename(rf.path, rf.path+".1") // backup único; sobrescreve o anterior
	rf.size = 0
	return rf.open()
}

// Close fecha o arquivo subjacente.
func (rf *RotatingFile) Close() error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.file.Close()
}

// Path devolve o caminho do arquivo (usado pelo viewer HTTP /logs).
func (rf *RotatingFile) Path() string { return rf.path }
