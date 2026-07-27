package audit

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusSuccess = "success"
	StatusError   = "error"
	StatusWarning = "warning"
)

// Entry represents a single audit log entry.
type Entry struct {
	ID        int64     `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	Action    string    `json:"action"`
	AgentID   string    `json:"agent_id"`
	User      string    `json:"user"`
	Details   string    `json:"details"`
	Status    string    `json:"status"`
}

// Logger provides in-memory ring buffer audit logging with optional SQLite persistence.
type Logger struct {
	mu        sync.RWMutex
	entries   []Entry
	capacity  int
	nextID    int64
	db        *sql.DB
	dbEnabled bool
}

// NewLogger creates a new audit logger with a default capacity of 1000 entries.
func NewLogger() *Logger {
	return &Logger{
		entries:  make([]Entry, 0, 1000),
		capacity: 1000,
		nextID:   1,
	}
}

// NewLoggerWithCapacity creates a new audit logger with a specified capacity.
func NewLoggerWithCapacity(capacity int) *Logger {
	return &Logger{
		entries:  make([]Entry, 0, capacity),
		capacity: capacity,
		nextID:   1,
	}
}

// OpenDB opens a SQLite database for persistent audit log storage.
func (l *Logger) OpenDB(dbPath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if dbPath == "" {
		dbPath = "audit.db"
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open audit database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	l.db = db
	l.dbEnabled = true

	if err := l.createTable(); err != nil {
		l.db.Close()
		l.db = nil
		l.dbEnabled = false
		return fmt.Errorf("failed to create audit_log table: %w", err)
	}

	return nil
}

func (l *Logger) createTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		action TEXT NOT NULL,
		agent_id TEXT NOT NULL DEFAULT '',
		user TEXT NOT NULL DEFAULT '',
		details TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'success'
	);`
	_, err := l.db.Exec(query)
	return err
}

// Close closes the database connection if open.
func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.db != nil {
		return l.db.Close()
	}
	return nil
}

// Log adds a new audit log entry.
func (l *Logger) Log(action, agentID, user, details, status string) {
	if status == "" {
		status = StatusSuccess
	}

	entry := Entry{
		ID:        l.nextID,
		Timestamp: time.Now(),
		Action:    action,
		AgentID:   agentID,
		User:      user,
		Details:   details,
		Status:    status,
	}

	l.mu.Lock()
	l.nextID++

	// Ring buffer: append, trim if over capacity
	if len(l.entries) >= l.capacity {
		l.entries = append(l.entries[1:], entry)
	} else {
		l.entries = append(l.entries, entry)
	}

	// Also write to DB if enabled
	var dbErr error
	if l.dbEnabled && l.db != nil {
		_, dbErr = l.db.Exec(
			"INSERT INTO audit_log (timestamp, action, agent_id, user, details, status) VALUES (?, ?, ?, ?, ?, ?)",
			entry.Timestamp, entry.Action, entry.AgentID, entry.User, entry.Details, entry.Status,
		)
	}
	l.mu.Unlock()

	if dbErr != nil {
		// Silently log db error — audit should never break the main flow
		fmt.Printf("audit: failed to write to database: %v\n", dbErr)
	}
}

// GetRecent returns the most recent N audit log entries.
func (l *Logger) GetRecent(limit int) []Entry {
	if limit <= 0 {
		limit = l.capacity
	}
	if limit > l.capacity {
		limit = l.capacity
	}

	l.mu.RLock()
	defer l.mu.RUnlock()

	total := len(l.entries)
	if total == 0 {
		return []Entry{}
	}

	start := total - limit
	if start < 0 {
		start = 0
	}

	result := make([]Entry, total-start)
	copy(result, l.entries[start:])
	return result
}

// Clear removes all entries from the in-memory buffer.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = make([]Entry, 0, l.capacity)
}
