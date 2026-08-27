package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"control-center/backend/securestore"

	_ "modernc.org/sqlite"
)

// Session represents a saved agent connection session.
type Session struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	Host          string    `json:"host"`
	Port          int       `json:"port"`
	Secure        bool      `json:"secure"`
	AuthToken     string    `json:"auth_token,omitempty"`
	TLS           bool      `json:"tls,omitempty"`
	CAFile        string    `json:"ca_file,omitempty"`
	ServerName    string    `json:"server_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	LastConnected time.Time `json:"last_connected"`
}

// Manager handles persistence of agent sessions via SQLite.
type Manager struct {
	db     *sql.DB
	mu     sync.RWMutex
	dbPath string
	store  securestore.Store
}

// NewManager creates a new session manager backed by SQLite.
func NewManager() *Manager {
	return &Manager{store: securestore.Default()}
}

// NewManagerWithStore creates a manager with an explicit token store. It is
// useful for tests and for callers that provide an approved platform store.
func NewManagerWithStore(store securestore.Store) *Manager {
	if store == nil {
		store = securestore.Default()
	}
	return &Manager{store: store}
}

// SetStore replaces the token store before or between database operations.
func (m *Manager) SetStore(store securestore.Store) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if store == nil {
		store = securestore.Default()
	}
	m.store = store
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
	if err := m.migrateSchema(); err != nil {
		m.db.Close()
		m.db = nil
		return fmt.Errorf("failed to migrate sessions table: %w", err)
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
		secure INTEGER NOT NULL DEFAULT 0,
		auth_token TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_connected DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(host, port)
	);`
	_, err := m.db.Exec(query)
	return err
}

func (m *Manager) migrateSchema() error {
	rows, err := m.db.Query("PRAGMA table_info(sessions)")
	if err != nil {
		return err
	}
	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, columnType string
		var notNull, primaryKey int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			rows.Close()
			return err
		}
		columns[name] = true
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	additions := []struct {
		name string
		ddl  string
	}{
		{name: "secure", ddl: "ALTER TABLE sessions ADD COLUMN secure INTEGER NOT NULL DEFAULT 0"},
		{name: "auth_token_protected", ddl: "ALTER TABLE sessions ADD COLUMN auth_token_protected TEXT NOT NULL DEFAULT ''"},
		{name: "tls_enabled", ddl: "ALTER TABLE sessions ADD COLUMN tls_enabled INTEGER NOT NULL DEFAULT 0"},
		{name: "ca_file", ddl: "ALTER TABLE sessions ADD COLUMN ca_file TEXT NOT NULL DEFAULT ''"},
		{name: "server_name", ddl: "ALTER TABLE sessions ADD COLUMN server_name TEXT NOT NULL DEFAULT ''"},
	}
	for _, addition := range additions {
		if columns[addition.name] {
			continue
		}
		if _, err := m.db.Exec(addition.ddl); err != nil {
			return fmt.Errorf("add %s: %w", addition.name, err)
		}
	}
	return nil
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

	protectedToken := ""
	if s.AuthToken != "" {
		if m.store == nil {
			return 0, fmt.Errorf("protect session token: %w", securestore.ErrUnavailable)
		}
		var err error
		protectedToken, err = m.store.Protect(s.AuthToken)
		if err != nil {
			return 0, fmt.Errorf("protect session token: %w", err)
		}
	}

	secure := 0
	if s.Secure || s.TLS {
		secure = 1
	}
	tlsEnabled := 0
	if s.TLS || s.Secure {
		tlsEnabled = 1
	}

	query := `
	INSERT INTO sessions (name, host, port, secure, auth_token, auth_token_protected, tls_enabled, ca_file, server_name, created_at, last_connected)
	VALUES (?, ?, ?, ?, '', ?, ?, ?, ?, ?, ?)
	ON CONFLICT(host, port) DO UPDATE SET
		name = excluded.name,
		secure = excluded.secure,
		auth_token = '',
		auth_token_protected = excluded.auth_token_protected,
		tls_enabled = excluded.tls_enabled,
		ca_file = excluded.ca_file,
		server_name = excluded.server_name,
		last_connected = excluded.last_connected;`

	result, err := m.db.Exec(query, s.Name, s.Host, s.Port, secure, protectedToken, tlsEnabled, s.CAFile, s.ServerName, s.CreatedAt, s.LastConnected)
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
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return nil, nil
	}

	rows, err := m.db.Query(
		"SELECT id, name, host, port, secure, auth_token, auth_token_protected, tls_enabled, ca_file, server_name, created_at, last_connected FROM sessions ORDER BY last_connected DESC")
	if err != nil {
		return nil, fmt.Errorf("failed to load sessions: %w", err)
	}
	defer rows.Close()

	type sessionRow struct {
		session        Session
		protectedToken string
		tlsEnabled     bool
	}
	var loadedRows []sessionRow
	for rows.Next() {
		var s Session
		var protectedToken string
		var tlsEnabled int
		if err := rows.Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.Secure, &s.AuthToken, &protectedToken, &tlsEnabled, &s.CAFile, &s.ServerName, &s.CreatedAt, &s.LastConnected); err != nil {
			return nil, fmt.Errorf("failed to scan session row: %w", err)
		}
		loadedRows = append(loadedRows, sessionRow{
			session:        s,
			protectedToken: protectedToken,
			tlsEnabled:     tlsEnabled != 0,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("failed to close session rows: %w", err)
	}

	sessions := make([]Session, 0, len(loadedRows))
	for _, loaded := range loadedRows {
		s := loaded.session
		s.AuthToken, err = m.restoreToken(s.ID, s.AuthToken, loaded.protectedToken)
		if err != nil {
			return nil, err
		}
		s.TLS = loaded.tlsEnabled
		s.Secure = s.Secure || s.TLS
		sessions = append(sessions, s)
	}

	return sessions, nil
}

// LoadByHost loads a specific session by host and port.
func (m *Manager) LoadByHost(host string, port int) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.db == nil {
		return nil, fmt.Errorf("database not open")
	}

	var s Session
	var protectedToken string
	var tlsEnabled int
	err := m.db.QueryRow(
		"SELECT id, name, host, port, secure, auth_token, auth_token_protected, tls_enabled, ca_file, server_name, created_at, last_connected FROM sessions WHERE host = ? AND port = ?",
		host, port,
	).Scan(&s.ID, &s.Name, &s.Host, &s.Port, &s.Secure, &s.AuthToken, &protectedToken, &tlsEnabled, &s.CAFile, &s.ServerName, &s.CreatedAt, &s.LastConnected)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session by host: %w", err)
	}

	s.AuthToken, err = m.restoreToken(s.ID, s.AuthToken, protectedToken)
	if err != nil {
		return nil, err
	}
	s.TLS = tlsEnabled != 0
	s.Secure = s.Secure || s.TLS
	return &s, nil
}

func (m *Manager) restoreToken(id int64, plaintext, protected string) (string, error) {
	if protected != "" {
		if m.store == nil {
			return "", fmt.Errorf("restore session token: %w", securestore.ErrUnavailable)
		}
		token, err := m.store.Unprotect(protected)
		if err != nil {
			return "", fmt.Errorf("restore session token: %w", err)
		}
		if plaintext != "" {
			if _, err := m.db.Exec("UPDATE sessions SET auth_token = '' WHERE id = ?", id); err != nil {
				return "", fmt.Errorf("clear legacy session token: %w", err)
			}
		}
		return token, nil
	}
	if plaintext == "" {
		return "", nil
	}
	if m.store == nil {
		return "", fmt.Errorf("migrate session token: %w", securestore.ErrUnavailable)
	}
	protectedToken, err := m.store.Protect(plaintext)
	if err != nil {
		return "", fmt.Errorf("migrate session token: %w", err)
	}
	if _, err := m.db.Exec("UPDATE sessions SET auth_token = '', auth_token_protected = ? WHERE id = ?", protectedToken, id); err != nil {
		return "", fmt.Errorf("store migrated session token: %w", err)
	}
	return plaintext, nil
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
