package logging

import (
	"os"
	"sync"
)

const defaultMaxLogBytes = 1 << 20 // 1 MiB

// RotatingFile is an io.Writer that rotates the file once it exceeds maxBytes,
// keeping a single ".1" backup. No external dependency (KISS); this solves at
// the root the problem of a giant log file that becomes painfully slow to open
// in a plain text editor.
type RotatingFile struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	size     int64
	file     *os.File
}

// NewRotatingFile opens/creates the rotating log file (fail-fast on I/O error).
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

// Write implements io.Writer, rotating before exceeding the limit.
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
	_ = os.Rename(rf.path, rf.path+".1") // single backup; overwrites the previous one
	rf.size = 0
	return rf.open()
}

// Close closes the underlying file.
func (rf *RotatingFile) Close() error {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	return rf.file.Close()
}

// Path returns the file path (used by the /logs HTTP viewer).
func (rf *RotatingFile) Path() string { return rf.path }
