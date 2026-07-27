package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// Session represents a saved agent connection session.
type Session struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	AuthToken     string    `json:"auth_token,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastConnected time.Time `json:"last_connected"`
}

// Manager handles persistence of agent sessions via SQLite.
type Manager struct {
	db     *sql.DB
	mu     sync.RWMutex
	dbPath string
}

// NewManager creates a new session manager backed by SQLite.
func NewManager() *Manager {
	return &Manager{}
}

// Open initializes the SQLite database and creates the sessions table.
func (m *Manager) Open(dbDir string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if dbDir == "" {
		exe, err := os.Executable()
		if err != nil {
			dbDir = "."
		} else {
			dbDir = filepath.Dir(exe)
		}
	}

	m.dbPath = filepath.Join(dbDir, "lan-commander.db")

	db, err := sql.Open("sqlite", m.dbPath)
	if err != nil {
		return fmt.Errorf("failed to open session database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	m.db = db

	if err := m.createTable(); err != nil {
		m.db.Close()
		m.db = nil
		return fmt.Errorf("failed to create sessions table: %w", err)
	}

	return nil
}

// Close closes the database connection.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db != nil {
		return m.db.Close()
	}
	return nil
}

func (m *Manager) createTable() error {
	query := `
	CREATE TABLE IF NOT EXISTS sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL,
		port INTEGER NOT NULL,
		auth_token TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_connected DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(host, port)
	);`
	_, err := m.db.Exec(query)
	return err
}

// Save inserts or replaces a session in the database.
func (m *Manager) Save(s Session) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return 0, fmt.Errorf("database not open")
	}

	now := time.Now()
	if s.CreatedAt.IsZero() {
		s.CreatedAt = now
	}
	if s.LastConnected.IsZero() {
		s.LastConnected = now
	}

	query := `
	INSERT INTO sessions (name, host, port, auth_token, created_at, last_connected)
	VALUES (?, ?, ?, ?, ?, ?)
	ON CONFLICT(host, port) DO UPDATE SET
		name = excluded.name,
		auth_token = excluded.auth_token,
		last_connected = excluded.last_connected;`

	result, err := m.db.Exec(query, s.Name, s.Host, s.Port, s.AuthToken, s.CreatedAt, s.LastConnected)
	if err != nil {
		return 0, fmt.Errorf("failed to save session: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert id: %w", err)
	}

	return id, nil
}

// Delete removes a session by its ID.
func (m *Manager) Delete(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return fmt.Errorf("database not open")
	}

	result, err := m.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete session %d: %w", id, err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("session %d not found", id)
	}

	return nil
}

// LoadAll loads all saved sessions from the database.
func (m *Manager) LoadAll() ([]Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.db == nil {
		return nil, nil
	}

	rows, err := m.db.Query(
		"SELECT id, name, host, port, auth_token, created_at, last_connected FROM sessions ORDER BY last_connected DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	defer rows.Close()

	var sessions []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.AuthToken, &s.CreatedAt, &s.LastConnected); err != nil {
			return nil, fmt.Errorf("failed to scan session row: %w", err)
		}
		sessions = append(sessions, s)
	}

	if sessions == nil {
		sessions = []Session{}
	}

	return sessions, rows.Err()
}

// LoadByHost loads a specific session by host and port.
func (m *Manager) LoadByHost(host string, port int) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}

	var s Session
	err := m.db.QueryRow(
		"SELECT id, name, host, port, auth_token, created_at, last_connected FROM sessions WHERE host = ? AND port = ?",
		host, port,
	).Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.AuthToken, &s.CreatedAt, &s.LastConnected)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session by host: %w", err)
	}

	return &s, nil
}

// UpdateLastConnected updates the last_connected timestamp for a session.
func (m *Manager) UpdateLastConnected(id int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return fmt.Errorf("database not open")
	}

	_, err := m.db.Exec("UPDATE sessions SET last_connected = ? WHERE id = ?", time.Now(), id)
	return err
}
