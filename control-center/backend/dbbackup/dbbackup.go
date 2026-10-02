// Package dbbackup keeps a safety copy of a SQLite database the first time a
// new application version opens it, so an upgrade (and any schema migration it
// runs) can always be rolled back by restoring the copy.
package dbbackup

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// keepBackups is how many version backups are retained per database.
const keepBackups = 5

var unsafeVersionChars = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// BackupBeforeUpgrade copies dbPath to "<dbPath>.bak-<version>" unless the file
// does not exist, is empty, or already has a backup for that version. It must be
// called before the database is opened, so the copy is a consistent snapshot of
// what the previous version left behind. It returns the backup path, or "" when
// nothing was copied.
func BackupBeforeUpgrade(dbPath, version string) (string, error) {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	if version == "" {
		return "", errors.New("version is required")
	}
	version = unsafeVersionChars.ReplaceAllString(version, "_")

	info, err := os.Stat(dbPath)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil // first run: nothing to protect
	}
	if err != nil {
		return "", fmt.Errorf("stat database: %w", err)
	}
	if info.IsDir() || info.Size() == 0 {
		return "", nil
	}

	backupPath := dbPath + ".bak-" + version
	if _, err := os.Stat(backupPath); err == nil {
		return "", nil // this version already took its snapshot
	}

	if err := copyFile(dbPath, backupPath); err != nil {
		return "", err
	}
	prune(dbPath)
	return backupPath, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer in.Close()

	tmp, err := os.CreateTemp(filepath.Dir(dst), filepath.Base(dst)+".tmp-*")
	if err != nil {
		return fmt.Errorf("create backup: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := io.Copy(tmp, in); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("copy database: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("flush backup: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close backup: %w", err)
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		cleanup()
		return fmt.Errorf("restrict backup: %w", err)
	}
	// Rename last, so a half-written copy can never be mistaken for a backup.
	if err := os.Rename(tmpName, dst); err != nil {
		cleanup()
		return fmt.Errorf("publish backup: %w", err)
	}
	return nil
}

// prune removes the oldest backups beyond keepBackups.
func prune(dbPath string) {
	matches, err := filepath.Glob(dbPath + ".bak-*")
	if err != nil || len(matches) <= keepBackups {
		return
	}
	type backup struct {
		path string
		mod  int64
	}
	backups := make([]backup, 0, len(matches))
	for _, m := range matches {
		if info, err := os.Stat(m); err == nil {
			backups = append(backups, backup{m, info.ModTime().UnixNano()})
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].mod > backups[j].mod })
	for _, old := range backups[min(keepBackups, len(backups)):] {
		_ = os.Remove(old.path)
	}
}
