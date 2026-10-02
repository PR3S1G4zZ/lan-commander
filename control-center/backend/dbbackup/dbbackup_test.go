package dbbackup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeDB(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write db: %v", err)
	}
}

func TestBackupCopiesExistingDatabaseOncePerVersion(t *testing.T) {
	db := filepath.Join(t.TempDir(), "lan-commander.db")
	writeDB(t, db, "version-1-data")

	backup, err := BackupBeforeUpgrade(db, "v1.1.0")
	if err != nil {
		t.Fatalf("BackupBeforeUpgrade: %v", err)
	}
	if backup != db+".bak-1.1.0" {
		t.Fatalf("backup path = %q", backup)
	}
	if got, _ := os.ReadFile(backup); string(got) != "version-1-data" {
		t.Fatalf("backup content = %q", got)
	}

	// The app then migrates and changes the live database...
	writeDB(t, db, "migrated-data")
	// ...and a later start of the same version must NOT overwrite the snapshot.
	again, err := BackupBeforeUpgrade(db, "1.1.0")
	if err != nil || again != "" {
		t.Fatalf("second call = (%q, %v), want no new backup", again, err)
	}
	if got, _ := os.ReadFile(backup); string(got) != "version-1-data" {
		t.Fatalf("snapshot was overwritten: %q", got)
	}
}

func TestBackupSkipsMissingAndEmptyDatabases(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "none.db")
	if b, err := BackupBeforeUpgrade(missing, "1.1.0"); b != "" || err != nil {
		t.Fatalf("missing db: (%q, %v)", b, err)
	}
	empty := filepath.Join(dir, "empty.db")
	writeDB(t, empty, "")
	if b, err := BackupBeforeUpgrade(empty, "1.1.0"); b != "" || err != nil {
		t.Fatalf("empty db: (%q, %v)", b, err)
	}
	if _, err := BackupBeforeUpgrade(empty, " "); err == nil {
		t.Fatal("an empty version must be rejected")
	}
}

func TestBackupSanitisesVersionAndLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "a.db")
	writeDB(t, db, "x")

	backup, err := BackupBeforeUpgrade(db, "1.2.0/../evil")
	if err != nil {
		t.Fatalf("BackupBeforeUpgrade: %v", err)
	}
	if filepath.Dir(backup) != dir {
		t.Fatalf("backup escaped the data directory: %q", backup)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
	if len(leftovers) != 0 {
		t.Fatalf("temporary files left behind: %v", leftovers)
	}
}

func TestBackupKeepsOnlyTheMostRecentVersions(t *testing.T) {
	dir := t.TempDir()
	db := filepath.Join(dir, "a.db")
	writeDB(t, db, "x")

	base := time.Now().Add(-time.Hour)
	for i := 0; i < keepBackups+2; i++ {
		old := db + ".bak-0." + string(rune('a'+i))
		writeDB(t, old, "old")
		stamp := base.Add(time.Duration(i) * time.Minute)
		if err := os.Chtimes(old, stamp, stamp); err != nil {
			t.Fatalf("chtimes: %v", err)
		}
	}
	if _, err := BackupBeforeUpgrade(db, "9.9.9"); err != nil {
		t.Fatalf("BackupBeforeUpgrade: %v", err)
	}

	matches, _ := filepath.Glob(db + ".bak-*")
	if len(matches) != keepBackups {
		t.Fatalf("%d backups kept, want %d", len(matches), keepBackups)
	}
	if _, err := os.Stat(db + ".bak-9.9.9"); err != nil {
		t.Fatalf("the newest backup must survive pruning: %v", err)
	}
}
