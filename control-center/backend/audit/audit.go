package audit

import (
	"database/sql"
	"fmt"
	"log"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

const (
	StatusSuccess   = "success"
	StatusError     = "error"
	StatusWarning   = "warning"
	defaultCapacity = 1000
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
		entries:  make([]Entry, 0, defaultCapacity),
		capacity: defaultCapacity,
		nextID:   1,
	}
}

// NewLoggerWithCapacity creates a new audit logger with a specified capacity.
func NewLoggerWithCapacity(capacity int) *Logger {
	if capacity <= 0 {
		capacity = defaultCapacity
	}
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

	if l.db != nil {
		if err := l.db.Close(); err != nil {
			return fmt.Errorf("failed to close existing audit database: %w", err)
		}
		l.db = nil
		l.dbEnabled = false
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return fmt.Errorf("failed to open audit database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	l.db = db

	if err := l.createTable(); err != nil {
		l.db.Close()
		l.db = nil
		l.dbEnabled = false
		return fmt.Errorf("failed to create audit_log table: %w", err)
	}

	var nextID int64
	if err := db.QueryRow("SELECT COALESCE(MAX(id), 0) + 1 FROM audit_log").Scan(&nextID); err != nil {
		db.Close()
		l.db = nil
		return fmt.Errorf("failed to initialize audit log ID: %w", err)
	}
	if nextID < 1 {
		nextID = 1
	}
	l.nextID = nextID
	l.dbEnabled = true

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
		err := l.db.Close()
		l.db = nil
		l.dbEnabled = false
		return err
	}
	return nil
}

// Log adds a new audit log entry.
func (l *Logger) Log(action, agentID, user, details, status string) {
	if status == "" {
		status = StatusSuccess
	}

	l.mu.Lock()
	fallbackID := l.nextID
	l.nextID++
	entry := Entry{
		ID:        fallbackID,
		Timestamp: time.Now(),
		Action:    action,
		AgentID:   agentID,
		User:      user,
		Details:   details,
		Status:    status,
	}

	var dbErr error
	if l.dbEnabled && l.db != nil {
		result, err := l.db.Exec(
			"INSERT INTO audit_log (timestamp, action, agent_id, user, details, status) VALUES (?, ?, ?, ?, ?, ?)",
			entry.Timestamp, entry.Action, entry.AgentID, entry.User, entry.Details, entry.Status,
		)
		if err != nil {
			dbErr = err
		} else if id, err := result.LastInsertId(); err != nil {
			dbErr = fmt.Errorf("failed to get inserted audit ID: %w", err)
		} else if id > 0 {
			entry.ID = id
			if l.nextID <= id {
				l.nextID = id + 1
			}
		}
	}

	// Ring buffer: append, trim if over capacity.
	if len(l.entries) >= l.capacity {
		l.entries = append(l.entries[1:], entry)
	} else {
		l.entries = append(l.entries, entry)
	}
	l.mu.Unlock()

	if dbErr != nil {
		// Audit persistence must not break the main flow, but the failure must be
		// visible to operators and tests.
		log.Printf("audit: failed to write to database: %v", dbErr)
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

	if l.dbEnabled && l.db != nil {
		entries, err := l.getRecentFromDBLocked(limit)
		if err == nil {
			return entries
		}
		log.Printf("audit: failed to read persisted entries: %v", err)
	}

	return l.getRecentFromMemoryLocked(limit)
}

func (l *Logger) getRecentFromDBLocked(limit int) ([]Entry, error) {
	rows, err := l.db.Query(`
		SELECT id, timestamp, action, agent_id, user, details, status
		FROM audit_log
		ORDER BY id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	entries := make([]Entry, 0, limit)
	for rows.Next() {
		var entry Entry
		if err := rows.Scan(&entry.ID, &entry.Timestamp, &entry.Action, &entry.AgentID, &entry.User, &entry.Details, &entry.Status); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Keep the historical API order: oldest to newest within the selected
	// recent window.
	for left, right := 0, len(entries)-1; left < right; left, right = left+1, right-1 {
		entries[left], entries[right] = entries[right], entries[left]
	}
	return entries, nil
}

func (l *Logger) getRecentFromMemoryLocked(limit int) []Entry {
	if limit > l.capacity {
		limit = l.capacity
	}

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

// Clear removes all entries from the in-memory buffer and the optional
// persistent store.
func (l *Logger) Clear() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.dbEnabled && l.db != nil {
		if _, err := l.db.Exec("DELETE FROM audit_log"); err != nil {
			log.Printf("audit: failed to clear persisted entries: %v", err)
		}
	}
	l.entries = make([]Entry, 0, l.capacity)
}
