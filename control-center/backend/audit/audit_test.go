package audit

import (
	"path/filepath"
	"testing"
)

func TestPersistentRecentEntriesAndClear(t *testing.T) {
	logger := NewLoggerWithCapacity(10)
	dbPath := filepath.Join(t.TempDir(), "audit.db")
	if err := logger.OpenDB(dbPath); err != nil {
		t.Fatalf("open audit database: %v", err)
	}
	defer logger.Close()

	logger.Log("exec_command", "agent-1", "user", "echo ok", StatusSuccess)
	entries := logger.GetRecent(10)
	if len(entries) != 1 || entries[0].Action != "exec_command" {
		t.Fatalf("unexpected persisted entries: %#v", entries)
	}

	logger.Clear()
	if entries := logger.GetRecent(10); len(entries) != 0 {
		t.Fatalf("clear left persisted entries: %#v", entries)
	}
}
