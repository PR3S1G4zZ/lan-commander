package transfer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"control-center/backend/protocol"

	"github.com/google/uuid"
)

const DefaultChunkSize = 64 * 1024

type Requester interface {
	SendRequest(agentID string, msgType string, payload interface{}, timeout time.Duration) (*protocol.Message, error)
}

type fileAckPayload struct {
	Path      string `json:"path"`
	Offset    int64  `json:"offset"`
	Final     bool   `json:"final"`
	Committed *bool  `json:"committed,omitempty"`
}

func Download(ctx context.Context, requester Requester, agentID, remotePath, localPath string, timeout time.Duration) error {
	if err := validateRequest(ctx, requester, agentID, remotePath, localPath); err != nil {
		return err
	}

	destination, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("invalid local path: %w", err)
	}
	parentDir := filepath.Dir(destination)
	info, err := os.Stat(parentDir)
	if err != nil {
		return fmt.Errorf("local directory is not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("local path parent is not a directory: %s", parentDir)
	}

	partFile, err := os.CreateTemp(parentDir, ".lan-commander-download-*")
	if err != nil {
		return fmt.Errorf("cannot create temporary download: %w", err)
	}
	partPath := partFile.Name()
	committed := false
	defer func() {
		_ = partFile.Close()
		if !committed {
			_ = os.Remove(partPath)
		}
	}()

	var offset int64
	var totalSize int64 = -1
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("download cancelled: %w", err)
		}
		response, err := requester.SendRequest(agentID, protocol.MsgGetFile, protocol.GetFilePayload{
			Path:      remotePath,
			Offset:    offset,
			ChunkSize: DefaultChunkSize,
		}, timeout)
		if err != nil {
			return fmt.Errorf("file transfer failed: %w", err)
		}

		var chunk protocol.FileChunkPayload
		if err := decodePayload(response, &chunk); err != nil {
			return fmt.Errorf("invalid file chunk response: %w", err)
		}
		if chunk.Offset != offset {
			return fmt.Errorf("file chunk offset mismatch: received %d, expected %d", chunk.Offset, offset)
		}
		if len(chunk.Data) > DefaultChunkSize {
			return fmt.Errorf("file chunk exceeds %d bytes", DefaultChunkSize)
		}
		if totalSize < 0 {
			totalSize = chunk.TotalSize
		} else if chunk.TotalSize != totalSize {
			return fmt.Errorf("file size changed during transfer")
		}
		if totalSize < 0 || offset+int64(len(chunk.Data)) > totalSize {
			return fmt.Errorf("invalid file chunk size")
		}
		if !chunk.Final && len(chunk.Data) == 0 {
			return fmt.Errorf("agent returned an empty non-final file chunk")
		}
		if _, err := partFile.Write(chunk.Data); err != nil {
			return fmt.Errorf("cannot write downloaded chunk: %w", err)
		}
		offset += int64(len(chunk.Data))
		if !chunk.Final {
			if offset >= totalSize {
				return fmt.Errorf("non-final chunk reached the advertised file size")
			}
			continue
		}
		if offset != totalSize {
			return fmt.Errorf("final chunk ended at %d, expected %d", offset, totalSize)
		}
		if chunk.Checksum == "" {
			return fmt.Errorf("final file chunk is missing its checksum")
		}
		if err := verifyChecksum(partFile, chunk.Checksum); err != nil {
			return err
		}
		break
	}

	if err := partFile.Sync(); err != nil {
		return fmt.Errorf("cannot flush downloaded file: %w", err)
	}
	if err := partFile.Close(); err != nil {
		return fmt.Errorf("cannot close downloaded file: %w", err)
	}
	if err := os.Rename(partPath, destination); err != nil {
		return fmt.Errorf("cannot finalize downloaded file %q: %w", destination, err)
	}
	committed = true
	return nil
}

func Upload(ctx context.Context, requester Requester, agentID, localPath, remotePath string, timeout time.Duration) (err error) {
	if err := validateRequest(ctx, requester, agentID, localPath, remotePath); err != nil {
		return err
	}

	source, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open upload source: %w", err)
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil {
		return fmt.Errorf("cannot stat upload source: %w", err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("upload source is not a regular file")
	}

	totalSize := info.Size()
	offset := int64(0)
	transferID := uuid.NewString()
	sentChunk := false
	defer func() {
		if err != nil && sentChunk {
			_, _ = requester.SendRequest(agentID, protocol.MsgCancelFile, protocol.CancelFilePayload{
				Path:       remotePath,
				TransferID: transferID,
			}, timeout)
		}
	}()
	hasher := sha256.New()
	for {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("upload cancelled: %w", err)
		}
		buffer := make([]byte, DefaultChunkSize)
		n, readErr := source.Read(buffer)
		if n > 0 {
			data := buffer[:n]
			if _, err := hasher.Write(data); err != nil {
				return fmt.Errorf("cannot checksum upload source: %w", err)
			}
			final := offset+int64(n) == totalSize
			payload := protocol.SendFilePayload{
				Path:       remotePath,
				Data:       data,
				Offset:     offset,
				TotalSize:  totalSize,
				Final:      final,
				TransferID: transferID,
			}
			if final {
				payload.Checksum = hex.EncodeToString(hasher.Sum(nil))
			}
			sentChunk = true
			if err := sendUploadChunk(ctx, requester, agentID, payload, timeout); err != nil {
				return err
			}
			offset += int64(n)
			if final {
				return nil
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				if totalSize == 0 {
					payload := protocol.SendFilePayload{
						Path:       remotePath,
						TotalSize:  0,
						Final:      true,
						Checksum:   hex.EncodeToString(hasher.Sum(nil)),
						TransferID: transferID,
					}
					sentChunk = true
					return sendUploadChunk(ctx, requester, agentID, payload, timeout)
				}
				return fmt.Errorf("upload ended at %d, expected %d bytes", offset, totalSize)
			}
			return fmt.Errorf("cannot read upload source: %w", readErr)
		}
	}
}

func sendUploadChunk(ctx context.Context, requester Requester, agentID string, payload protocol.SendFilePayload, timeout time.Duration) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("upload cancelled: %w", err)
	}
	response, err := requester.SendRequest(agentID, protocol.MsgSendFile, payload, timeout)
	if err != nil {
		return fmt.Errorf("upload chunk failed at offset %d: %w", payload.Offset, err)
	}
	if response == nil || response.Type != protocol.MsgFileAck {
		return fmt.Errorf("invalid upload acknowledgement at offset %d", payload.Offset)
	}
	var ack fileAckPayload
	if err := decodePayload(response, &ack); err != nil {
		return fmt.Errorf("invalid upload acknowledgement at offset %d: %w", payload.Offset, err)
	}
	if ack.Path != "" && ack.Path != payload.Path {
		return fmt.Errorf("upload acknowledgement path mismatch: received %q, expected %q", ack.Path, payload.Path)
	}
	if ack.Offset != payload.Offset {
		return fmt.Errorf("upload acknowledgement offset mismatch: received %d, expected %d", ack.Offset, payload.Offset)
	}
	if ack.Final != payload.Final {
		return fmt.Errorf("upload acknowledgement final flag mismatch")
	}
	if payload.Final && ack.Committed != nil && !*ack.Committed {
		return fmt.Errorf("agent did not commit the final upload chunk")
	}
	return nil
}

func validateRequest(ctx context.Context, requester Requester, agentID, firstPath, secondPath string) error {
	if ctx == nil {
		return fmt.Errorf("transfer context cannot be nil")
	}
	if requester == nil {
		return fmt.Errorf("transfer requester cannot be nil")
	}
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	if strings.TrimSpace(firstPath) == "" {
		return fmt.Errorf("transfer path cannot be empty")
	}
	if strings.TrimSpace(secondPath) == "" {
		return fmt.Errorf("transfer destination cannot be empty")
	}
	return nil
}

func decodePayload(response *protocol.Message, target any) error {
	if response == nil {
		return fmt.Errorf("empty response")
	}
	data, err := json.Marshal(response.Payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func verifyChecksum(file *os.File, expected string) error {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("cannot verify transferred file: %w", err)
	}
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return fmt.Errorf("cannot verify transferred file: %w", err)
	}
	actual := hex.EncodeToString(hasher.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("transfer checksum mismatch")
	}
	return nil
}
