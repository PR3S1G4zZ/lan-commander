package filesystem

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func testChecksum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestWriteAtomicUploadChunkCommitsOnlyValidatedFinalChunk(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "uploaded.bin")
	transferID := "transfer-atomic"
	content := []byte("atomic upload content")

	if err := WriteAtomicUploadChunk(destination, transferID, content[:7], 0, int64(len(content)), false, ""); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists before final chunk: %v", err)
	}

	if err := WriteAtomicUploadChunk(destination, transferID, content[7:], 7, int64(len(content)), true, testChecksum(content)); err != nil {
		t.Fatalf("write final chunk: %v", err)
	}
	actual, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read committed upload: %v", err)
	}
	if !bytes.Equal(actual, content) {
		t.Fatalf("committed content = %q, want %q", actual, content)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("upload temporary files remain after commit: %v", matches)
	}
}

func TestWriteAtomicUploadChunkRejectsInvalidOffsetAndRemovesTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "uploaded.bin")

	if err := WriteAtomicUploadChunk(destination, "transfer-offset", []byte("data"), 2, 4, false, ""); err == nil {
		t.Fatal("invalid first offset was accepted")
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after offset failure: %v", matches)
	}
}

func TestWriteAtomicUploadChunkRejectsChecksumAndPreservesExistingDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "uploaded.bin")
	if err := os.WriteFile(destination, []byte("old"), 0600); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	content := []byte("new")
	if err := WriteAtomicUploadChunk(destination, "transfer-checksum", content, 0, int64(len(content)), true, testChecksum([]byte("wrong"))); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
	actual, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read preserved destination: %v", err)
	}
	if string(actual) != "old" {
		t.Fatalf("destination changed after checksum failure: %q", actual)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after checksum failure: %v", matches)
	}
}

func TestWriteAtomicUploadChunkRejectsChangedTotalAndRemovesTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "uploaded.bin")
	transferID := "transfer-total"

	if err := WriteAtomicUploadChunk(destination, transferID, []byte("one"), 0, 6, false, ""); err != nil {
		t.Fatalf("write first chunk: %v", err)
	}
	if err := WriteAtomicUploadChunk(destination, transferID, []byte("two"), 3, 7, false, ""); err == nil {
		t.Fatal("changed total size was accepted")
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after total-size failure: %v", matches)
	}
}

func TestCancelAtomicUploadRemovesTemporaryFilesWithoutTouchingDestination(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "uploaded.bin")
	if err := WriteAtomicUploadChunk(destination, "transfer-cancel", []byte("partial"), 0, 20, false, ""); err != nil {
		t.Fatalf("write partial upload: %v", err)
	}

	if err := CancelAtomicUpload(destination, "transfer-cancel"); err != nil {
		t.Fatalf("cancel upload: %v", err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after cancel: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after cancel: %v", matches)
	}
}
