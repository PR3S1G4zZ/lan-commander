package filesystem

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

const (
	// DefaultChunkSize is 64KB.
	DefaultChunkSize = 64 * 1024
	// MaxDirEntries limits the number of entries returned per ListDir call.
	MaxDirEntries = 50
	// MaxChunkSize prevents excessive allocations from a single request.
	MaxChunkSize = 4 * 1024 * 1024
)

// ErrPathTraversal is returned when a path contains traversal sequences.
var ErrPathTraversal = fmt.Errorf("path traversal detected")

// safePath validates and cleans a path, rejecting traversal attempts.
// If the path is relative, it's converted to absolute using the current
// working directory.
func safePath(path string) (string, error) {
	if strings.Contains(path, "..") {
		return "", fmt.Errorf("%w: %q contains \"..\"", ErrPathTraversal, path)
	}
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		abs, err := filepath.Abs(cleaned)
		if err != nil {
			return "", fmt.Errorf("cannot resolve path %q: %w", path, err)
		}
		cleaned = abs
	}
	return cleaned, nil
}

// ListDir returns the contents of a directory, up to MaxDirEntries.
func ListDir(path string) (protocol.DirContentsPayload, error) {
	safe, err := safePath(path)
	if err != nil {
		return protocol.DirContentsPayload{}, err
	}

	info, err := os.Stat(safe)
	if err != nil {
		return protocol.DirContentsPayload{}, fmt.Errorf("cannot access %q: %w", safe, err)
	}
	if !info.IsDir() {
		return protocol.DirContentsPayload{}, fmt.Errorf("%q is not a directory", safe)
	}

	entries, err := os.ReadDir(safe)
	if err != nil {
		return protocol.DirContentsPayload{}, fmt.Errorf("cannot read directory %q: %w", safe, err)
	}

	limit := MaxDirEntries
	if len(entries) < limit {
		limit = len(entries)
	}

	dirEntries := make([]protocol.DirEntry, 0, limit)
	for _, e := range entries[:limit] {
		fi, err := e.Info()
		if err != nil {
			continue
		}
		dirEntries = append(dirEntries, protocol.DirEntry{
			Name:    e.Name(),
			Path:    filepath.Join(safe, e.Name()),
			IsDir:   e.IsDir(),
			Size:    fi.Size(),
			Mode:    fi.Mode().String(),
			ModTime: fi.ModTime().Format("2006-01-02T15:04:05Z07:00"),
		})
	}

	return protocol.DirContentsPayload{
		Path:    safe,
		Entries: dirEntries,
		Total:   len(dirEntries),
	}, nil
}

// ReadFileChunk reads a section of a file starting at offset with the given size.
// If chunkSize <= 0, DefaultChunkSize is used.
func ReadFileChunk(path string, offset int64, chunkSize int) ([]byte, int64, error) {
	if offset < 0 {
		return nil, 0, fmt.Errorf("invalid offset %d", offset)
	}

	safe, err := safePath(path)
	if err != nil {
		return nil, 0, err
	}

	if chunkSize <= 0 {
		chunkSize = DefaultChunkSize
	}

	f, err := os.Open(safe)
	if err != nil {
		return nil, 0, fmt.Errorf("cannot open %q: %w", safe, err)
	}
	defer f.Close()

	// Get file size for bounds checking
	fi, err := f.Stat()
	if err != nil {
		return nil, 0, fmt.Errorf("cannot stat %q: %w", safe, err)
	}
	fileSize := fi.Size()

	if offset > fileSize {
		return nil, fileSize, fmt.Errorf("offset %d beyond file size %d", offset, fileSize)
	}
	if offset == fileSize {
		if fileSize == 0 {
			return []byte{}, fileSize, nil
		}
		return nil, fileSize, fmt.Errorf("offset %d beyond file size %d", offset, fileSize)
	}

	// Clamp chunk size to remaining bytes
	remaining := fileSize - offset
	if int64(chunkSize) > remaining {
		chunkSize = int(remaining)
	}

	buf := make([]byte, chunkSize)
	n, err := f.ReadAt(buf, offset)
	if err != nil && n == 0 {
		return nil, fileSize, fmt.Errorf("read error at offset %d: %w", offset, err)
	}
	buf = buf[:n]

	return buf, fileSize, nil
}

// WriteFileChunk writes data to a file at the given offset.
// If offset == 0, the file is created or truncated.
func WriteFileChunk(path string, data []byte, offset int64) error {
	if offset < 0 {
		return fmt.Errorf("invalid offset %d", offset)
	}

	safe, err := safePath(path)
	if err != nil {
		return err
	}

	var flag int
	if offset == 0 {
		flag = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	} else {
		flag = os.O_CREATE | os.O_WRONLY
	}

	f, err := os.OpenFile(safe, flag, 0644)
	if err != nil {
		return fmt.Errorf("cannot open %q for writing: %w", safe, err)
	}
	defer f.Close()

	if offset > 0 {
		// Ensure we can seek to the right position (file may be shorter)
		fi, err := f.Stat()
		if err != nil {
			return fmt.Errorf("cannot stat %q: %w", safe, err)
		}
		if offset > fi.Size() {
			// Pad with zeros if offset is beyond current size
			if err := f.Truncate(offset); err != nil {
				return fmt.Errorf("cannot extend file %q: %w", safe, err)
			}
		}
	}

	n, err := f.WriteAt(data, offset)
	if err != nil {
		return fmt.Errorf("write error at offset %d: %w", offset, err)
	}
	if n != len(data) {
		return fmt.Errorf("short write: wrote %d of %d bytes", n, len(data))
	}

	return nil
}

// WriteAtomicUploadChunk appends one validated upload chunk to a temporary file
// in the destination directory. The final path is replaced only after the
// final chunk has the expected size and SHA-256 checksum.
func WriteAtomicUploadChunk(path, transferID string, data []byte, offset, totalSize int64, final bool, expectedChecksum string) error {
	if strings.TrimSpace(transferID) == "" {
		return fmt.Errorf("transfer ID cannot be empty")
	}
	if strings.ContainsAny(transferID, `/\\`) || strings.Contains(transferID, "..") {
		return fmt.Errorf("invalid transfer ID")
	}

	safe, err := safePath(path)
	if err != nil {
		return err
	}
	tempPath := filepath.Join(filepath.Dir(safe), ".lan-commander-upload-"+transferID+".part")
	metaPath := tempPath + ".meta"
	cleanup := func() {
		_ = os.Remove(tempPath)
		_ = os.Remove(metaPath)
	}
	if offset < 0 || totalSize < 0 {
		cleanup()
		return fmt.Errorf("invalid upload offset or total size")
	}
	if len(data) > DefaultChunkSize {
		cleanup()
		return fmt.Errorf("upload chunk exceeds %d bytes", DefaultChunkSize)
	}
	if offset+int64(len(data)) > totalSize {
		cleanup()
		return fmt.Errorf("upload chunk exceeds advertised total size")
	}
	if !final && len(data) == 0 {
		cleanup()
		return fmt.Errorf("empty non-final upload chunk")
	}
	if final && offset+int64(len(data)) != totalSize {
		cleanup()
		return fmt.Errorf("final upload chunk does not reach advertised total size")
	}
	if !final && offset+int64(len(data)) >= totalSize {
		cleanup()
		return fmt.Errorf("non-final upload chunk reaches advertised total size")
	}

	if offset == 0 {
		if err := os.WriteFile(tempPath, nil, 0600); err != nil {
			cleanup()
			return fmt.Errorf("cannot create temporary upload: %w", err)
		}
		if err := os.WriteFile(metaPath, []byte(strconv.FormatInt(totalSize, 10)), 0600); err != nil {
			cleanup()
			return fmt.Errorf("cannot create upload metadata: %w", err)
		}
	} else {
		metadata, err := os.ReadFile(metaPath)
		if err != nil {
			cleanup()
			return fmt.Errorf("cannot read upload metadata: %w", err)
		}
		expectedTotal, err := strconv.ParseInt(strings.TrimSpace(string(metadata)), 10, 64)
		if err != nil || expectedTotal != totalSize {
			cleanup()
			return fmt.Errorf("upload total size changed from %d to %d", expectedTotal, totalSize)
		}
		info, err := os.Stat(tempPath)
		if err != nil {
			cleanup()
			return fmt.Errorf("cannot resume temporary upload: %w", err)
		}
		if info.Size() != offset {
			cleanup()
			return fmt.Errorf("upload offset %d does not match temporary size %d", offset, info.Size())
		}
	}

	file, err := os.OpenFile(tempPath, os.O_WRONLY, 0600)
	if err != nil {
		cleanup()
		return fmt.Errorf("cannot open temporary upload: %w", err)
	}
	if len(data) > 0 {
		n, writeErr := file.WriteAt(data, offset)
		if writeErr != nil {
			_ = file.Close()
			cleanup()
			return fmt.Errorf("cannot write upload chunk: %w", writeErr)
		}
		if n != len(data) {
			_ = file.Close()
			cleanup()
			return fmt.Errorf("short upload write: wrote %d of %d bytes", n, len(data))
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		cleanup()
		return fmt.Errorf("cannot flush upload chunk: %w", err)
	}
	if err := file.Close(); err != nil {
		cleanup()
		return fmt.Errorf("cannot close temporary upload: %w", err)
	}
	if !final {
		return nil
	}
	if strings.TrimSpace(expectedChecksum) == "" {
		cleanup()
		return fmt.Errorf("final upload chunk is missing its checksum")
	}
	actualChecksum, err := FileSHA256(tempPath)
	if err != nil {
		cleanup()
		return fmt.Errorf("cannot checksum temporary upload: %w", err)
	}
	if !strings.EqualFold(actualChecksum, expectedChecksum) {
		cleanup()
		return fmt.Errorf("upload checksum mismatch")
	}
	if err := os.Rename(tempPath, safe); err != nil {
		cleanup()
		return fmt.Errorf("cannot atomically finalize upload: %w", err)
	}
	_ = os.Remove(metaPath)
	return nil
}

// CancelAtomicUpload removes an in-progress upload and leaves the final path
// untouched. It is safe to call after a transfer has already committed.
func CancelAtomicUpload(path, transferID string) error {
	if strings.TrimSpace(transferID) == "" {
		return fmt.Errorf("transfer ID cannot be empty")
	}
	if strings.ContainsAny(transferID, `/\\`) || strings.Contains(transferID, "..") {
		return fmt.Errorf("invalid transfer ID")
	}
	safe, err := safePath(path)
	if err != nil {
		return err
	}
	tempPath := filepath.Join(filepath.Dir(safe), ".lan-commander-upload-"+transferID+".part")
	metaPath := tempPath + ".meta"
	for _, candidate := range []string{tempPath, metaPath} {
		if err := os.Remove(candidate); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cannot remove temporary upload %q: %w", candidate, err)
		}
	}
	return nil
}

// FileSize returns the size of the file at path.
func FileSize(path string) (int64, error) {
	safe, err := safePath(path)
	if err != nil {
		return 0, err
	}
	fi, err := os.Stat(safe)
	if err != nil {
		return 0, fmt.Errorf("cannot stat %q: %w", safe, err)
	}
	return fi.Size(), nil
}

// PathExists checks if a path exists and optionally verifies it is a directory.
func PathExists(path string, mustBeDir bool) (bool, error) {
	safe, err := safePath(path)
	if err != nil {
		return false, err
	}
	fi, err := os.Stat(safe)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("cannot access %q: %w", safe, err)
	}
	if mustBeDir && !fi.IsDir() {
		return false, fmt.Errorf("%q is not a directory", safe)
	}
	return true, nil
}

// FileSHA256 computes the checksum of the complete file.
func FileSHA256(path string) (string, error) {
	safe, err := safePath(path)
	if err != nil {
		return "", err
	}
	f, err := os.Open(safe)
	if err != nil {
		return "", fmt.Errorf("cannot open %q for checksum: %w", safe, err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("cannot checksum %q: %w", safe, err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
