// Package audit keeps a local, append-only record of what was done to this
// machine through the agent, independent of any log the controller may keep.
package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	// DefaultMaxSize is the size at which the log is rotated.
	DefaultMaxSize int64 = 5 * 1024 * 1024
	// maxDetailRunes bounds a single record so a huge command cannot bloat the log.
	maxDetailRunes = 512
)

// Result values for Event.Result.
const (
	ResultOK     = "ok"
	ResultError  = "error"
	ResultDenied = "denied"
)

// Event is one line of the audit log.
type Event struct {
	Time   time.Time `json:"time"`
	Action string    `json:"action"`
	Result string    `json:"result"`
	// Remote is the peer address, Client the connection ID and User the
	// self-declared name sent at authentication (informational, not verified).
	Remote string `json:"remote,omitempty"`
	Client string `json:"client,omitempty"`
	User   string `json:"user,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// Logger writes JSON lines to a file and rotates it once to "<path>.1" when it
// grows past its maximum size. A nil *Logger is valid and discards everything.
type Logger struct {
	mu      sync.Mutex
	path    string
	file    *os.File
	size    int64
	maxSize int64
}

// Open creates (or appends to) the audit log at path. An empty path disables
// auditing and returns a nil Logger.
func Open(path string) (*Logger, error) {
	return open(path, DefaultMaxSize)
}

func open(path string, maxSize int64) (*Logger, error) {
	if path == "" {
		return nil, nil
	}
	l := &Logger{path: path, maxSize: maxSize}
	if err := l.openFile(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) openFile() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return fmt.Errorf("create audit log directory: %w", err)
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("open audit log: %w", err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("stat audit log: %w", err)
	}
	l.file = f
	l.size = info.Size()
	return nil
}

// Log appends an event. Write errors are deliberately ignored: auditing must
// never break the operation it records.
func (l *Logger) Log(e Event) {
	if l == nil {
		return
	}
	if e.Time.IsZero() {
		e.Time = time.Now().UTC()
	}
	e.Detail = truncate(e.Detail, maxDetailRunes)

	line, err := json.Marshal(e)
	if err != nil {
		return
	}
	line = append(line, '\n')

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	if l.size+int64(len(line)) > l.maxSize {
		l.rotate()
		if l.file == nil {
			return
		}
	}
	n, _ := l.file.Write(line)
	l.size += int64(n)
}

// rotate keeps exactly one previous generation. Callers hold l.mu.
func (l *Logger) rotate() {
	_ = l.file.Close()
	l.file = nil
	backup := l.path + ".1"
	_ = os.Remove(backup)
	_ = os.Rename(l.path, backup)
	_ = l.openFile()
}

// Close flushes and closes the log file.
func (l *Logger) Close() {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

func truncate(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return string(runes[:limit]) + "…"
}
