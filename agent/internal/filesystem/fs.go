package filesystem

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

const (
	// DefaultChunkSize is 64KB.
	DefaultChunkSize = 64 * 1024
	// MaxDirEntries limits the number of entries returned per ListDir call.
	MaxDirEntries = 50
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

	if offset >= fileSize {
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
