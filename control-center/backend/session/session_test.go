package session

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"
	"time"
)

type memoryTokenStore struct {
	protected map[string]string
	protects  int
}

func (s *memoryTokenStore) Protect(token string) (string, error) {
	if s.protected == nil {
		s.protected = make(map[string]string)
	}
	s.protects++
	ciphertext := "memory:" + token
	s.protected[ciphertext] = token
	return ciphertext, nil
}

func (s *memoryTokenStore) Unprotect(ciphertext string) (string, error) {
	if token, ok := s.protected[ciphertext]; ok {
		return token, nil
	}
	return "", errors.New("unknown ciphertext")
}

func setSessionStoreReflection(t *testing.T, manager *Manager, store *memoryTokenStore) {
	t.Helper()
	method := reflect.ValueOf(manager).MethodByName("SetStore")
	if !method.IsValid() {
		t.Fatal("Manager.SetStore is not implemented")
	}
	method.Call([]reflect.Value{reflect.ValueOf(store)})
}

func setSessionField(t *testing.T, session *Session, name string, value any) {
	t.Helper()
	field := reflect.ValueOf(session).Elem().FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("Session.%s is not implemented", name)
	}
	incoming := reflect.ValueOf(value)
	if !incoming.Type().AssignableTo(field.Type()) {
		t.Fatalf("Session.%s type = %s, cannot assign %s", name, field.Type(), incoming.Type())
	}
	field.Set(incoming)
}

func getSessionField(t *testing.T, session Session, name string) reflect.Value {
	t.Helper()
	field := reflect.ValueOf(session).FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("Session.%s is not implemented", name)
	}
	return field
}

func createLegacyDatabase(t *testing.T, dir string) {
	t.Helper()
	db, err := sql.Open("sqlite", dir+"/lan-commander.db")
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE sessions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL DEFAULT '',
		host TEXT NOT NULL,
		port INTEGER NOT NULL,
		auth_token TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		last_connected DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		UNIQUE(host, port)
	)`)
	if err != nil {
		t.Fatalf("create legacy sessions table: %v", err)
	}
	_, err = db.Exec(`INSERT INTO sessions (name, host, port, auth_token) VALUES (?, ?, ?, ?)`, "legacy", "agent.local", 8080, "legacy-token")
	if err != nil {
		t.Fatalf("insert legacy session: %v", err)
	}
}

func TestOpenMigratesLegacyRowsAndProtectsTokens(t *testing.T) {
	dir := t.TempDir()
	createLegacyDatabase(t, dir)
	store := &memoryTokenStore{}
	manager := NewManager()
	setSessionStoreReflection(t, manager, store)
	if err := manager.Open(dir); err != nil {
		t.Fatalf("open session manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	saved, err := manager.LoadByHost("agent.local", 8080)
	if err != nil {
		t.Fatalf("load migrated session: %v", err)
	}
	if saved == nil || saved.AuthToken != "legacy-token" {
		t.Fatalf("migrated session = %#v, want restored legacy token", saved)
	}
	if store.protects != 1 {
		t.Fatalf("Protect calls = %d, want 1 during legacy migration", store.protects)
	}

	db, err := sql.Open("sqlite", dir+"/lan-commander.db")
	if err != nil {
		t.Fatalf("open migrated database: %v", err)
	}
	defer db.Close()
	var plaintext, protected string
	if err := db.QueryRow("SELECT auth_token, auth_token_protected FROM sessions WHERE host = ?", "agent.local").Scan(&plaintext, &protected); err != nil {
		t.Fatalf("inspect migrated token columns: %v", err)
	}
	if plaintext != "" || protected != "memory:legacy-token" {
		t.Fatalf("migrated token columns = plaintext %q, protected %q", plaintext, protected)
	}
}

func TestSaveAndLoadPersistsTLSFieldsAndProtectedToken(t *testing.T) {
	manager := NewManager()
	store := &memoryTokenStore{}
	setSessionStoreReflection(t, manager, store)
	dir := t.TempDir()
	if err := manager.Open(dir); err != nil {
		t.Fatalf("open session manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	want := Session{Name: "secure", Host: "secure.local", Port: 8443, AuthToken: "secret", CreatedAt: time.Now()}
	setSessionField(t, &want, "TLS", true)
	setSessionField(t, &want, "CAFile", "C:\\certs\\agent-ca.pem")
	setSessionField(t, &want, "ServerName", "agent.internal")
	if _, err := manager.Save(want); err != nil {
		t.Fatalf("save secure session: %v", err)
	}

	got, err := manager.LoadByHost(want.Host, want.Port)
	if err != nil {
		t.Fatalf("load secure session: %v", err)
	}
	if got == nil || got.AuthToken != want.AuthToken {
		t.Fatalf("loaded token = %#v, want protected round trip", got)
	}
	if !getSessionField(t, *got, "TLS").Bool() ||
		getSessionField(t, *got, "CAFile").String() != "C:\\certs\\agent-ca.pem" ||
		getSessionField(t, *got, "ServerName").String() != "agent.internal" {
		t.Fatalf("loaded TLS fields = %#v", got)
	}
}

func TestLoadAllMigratesLegacyRowsAfterClosingTheCursor(t *testing.T) {
	dir := t.TempDir()
	createLegacyDatabase(t, dir)
	manager := NewManager()
	setSessionStoreReflection(t, manager, &memoryTokenStore{})
	if err := manager.Open(dir); err != nil {
		t.Fatalf("open session manager: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	result := make(chan error, 1)
	go func() {
		_, err := manager.LoadAll()
		result <- err
	}()
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("load all sessions: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("LoadAll did not finish after reading legacy rows")
	}
}
