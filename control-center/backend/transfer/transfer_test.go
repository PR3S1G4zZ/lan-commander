package transfer

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"control-center/backend/protocol"
)

type transferRequest struct {
	agentID string
	msgType string
	payload any
}

type fakeRequester struct {
	requests  []transferRequest
	responses []protocol.Message
	err       error
}

func (f *fakeRequester) SendRequest(agentID string, msgType string, payload interface{}, _ time.Duration) (*protocol.Message, error) {
	f.requests = append(f.requests, transferRequest{agentID: agentID, msgType: msgType, payload: payload})
	if f.err != nil {
		return nil, f.err
	}
	if len(f.responses) == 0 {
		return nil, errors.New("no fake response available")
	}
	response := f.responses[0]
	f.responses = f.responses[1:]
	return &response, nil
}

type testGetFilePayload struct {
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	ChunkSize int    `json:"chunk_size,omitempty"`
}

type testFileChunkPayload struct {
	Path      string `json:"path"`
	Data      []byte `json:"data"`
	Offset    int64  `json:"offset"`
	TotalSize int64  `json:"total_size,omitempty"`
	Final     bool   `json:"final"`
	Checksum  string `json:"checksum,omitempty"`
}

type testSendFilePayload struct {
	Path       string `json:"path"`
	Data       []byte `json:"data,omitempty"`
	Offset     int64  `json:"offset,omitempty"`
	TotalSize  int64  `json:"total_size,omitempty"`
	Final      bool   `json:"final,omitempty"`
	Checksum   string `json:"checksum,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`
}

func encodePayload(t *testing.T, payload any, target any) {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
}

func checksum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func TestDownloadStreams64KiBVerifiesChecksumAndFinalizesAtomically(t *testing.T) {
	remote := bytes.Repeat([]byte("lan-commander-transfer-"), 7000)
	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "download.bin")
	if err := os.WriteFile(localPath, []byte("old content"), 0600); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	responses := make([]protocol.Message, 0, 3)
	for offset := int64(0); offset < int64(len(remote)); {
		end := offset + int64(DefaultChunkSize)
		if end > int64(len(remote)) {
			end = int64(len(remote))
		}
		responses = append(responses, protocol.Message{
			Type: protocol.MsgFileChunk,
			Payload: testFileChunkPayload{
				Path:      "/remote/download.bin",
				Data:      remote[offset:end],
				Offset:    offset,
				TotalSize: int64(len(remote)),
				Final:     end == int64(len(remote)),
				Checksum:  checksum(remote),
			},
		})
		offset = end
	}

	fake := &fakeRequester{responses: responses}
	if err := Download(context.Background(), fake, "agent-1", "/remote/download.bin", localPath, time.Second); err != nil {
		t.Fatalf("Download() error = %v", err)
	}

	actual, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read finalized destination: %v", err)
	}
	if !bytes.Equal(actual, remote) {
		t.Fatalf("downloaded content differs from remote")
	}
	for i, request := range fake.requests {
		if request.msgType != protocol.MsgGetFile {
			t.Fatalf("request %d type = %q, want %q", i, request.msgType, protocol.MsgGetFile)
		}
		var payload testGetFilePayload
		encodePayload(t, request.payload, &payload)
		if payload.ChunkSize != DefaultChunkSize {
			t.Fatalf("request %d chunk size = %d, want %d", i, payload.ChunkSize, DefaultChunkSize)
		}
	}
	if matches, _ := filepath.Glob(filepath.Join(localDir, ".lan-commander-download-*")); len(matches) != 0 {
		t.Fatalf("download temporary files remain: %v", matches)
	}
}

func TestDownloadRejectsChecksumMismatchWithoutReplacingDestination(t *testing.T) {
	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "download.bin")
	if err := os.WriteFile(localPath, []byte("keep me"), 0600); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	fake := &fakeRequester{responses: []protocol.Message{{
		Type: protocol.MsgFileChunk,
		Payload: testFileChunkPayload{
			Data:      []byte("replacement"),
			Offset:    0,
			TotalSize: int64(len("replacement")),
			Final:     true,
			Checksum:  checksum([]byte("different")),
		},
	}}}
	if err := Download(context.Background(), fake, "agent-1", "/remote/file", localPath, time.Second); err == nil {
		t.Fatal("Download() succeeded with a checksum mismatch")
	}

	actual, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read preserved destination: %v", err)
	}
	if string(actual) != "keep me" {
		t.Fatalf("destination changed after checksum failure: %q", actual)
	}
	if matches, _ := filepath.Glob(filepath.Join(localDir, ".lan-commander-download-*")); len(matches) != 0 {
		t.Fatalf("download temporary files remain after checksum failure: %v", matches)
	}
}

func TestDownloadRejectsEmptyNonFinalChunkWithoutReplacingDestination(t *testing.T) {
	localDir := t.TempDir()
	localPath := filepath.Join(localDir, "download.bin")
	if err := os.WriteFile(localPath, []byte("keep me"), 0600); err != nil {
		t.Fatalf("seed destination: %v", err)
	}

	fake := &fakeRequester{responses: []protocol.Message{{
		Type: protocol.MsgFileChunk,
		Payload: testFileChunkPayload{
			Data:      []byte{},
			Offset:    0,
			TotalSize: 10,
			Final:     false,
		},
	}}}
	if err := Download(context.Background(), fake, "agent-1", "/remote/file", localPath, time.Second); err == nil {
		t.Fatal("Download() accepted an empty non-final chunk")
	}

	actual, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatalf("read preserved destination: %v", err)
	}
	if string(actual) != "keep me" {
		t.Fatalf("destination changed after empty non-final chunk: %q", actual)
	}
}

func TestUploadStreams64KiBAndSendsFinalChecksum(t *testing.T) {
	data := bytes.Repeat([]byte("upload-data-"), 7000)
	localPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(localPath, data, 0600); err != nil {
		t.Fatalf("create upload source: %v", err)
	}

	chunks := (len(data) + DefaultChunkSize - 1) / DefaultChunkSize
	responses := make([]protocol.Message, chunks)
	for i := range responses {
		offset := i * DefaultChunkSize
		responses[i] = protocol.Message{Type: protocol.MsgFileAck, Payload: map[string]any{
			"path":      "/remote/upload.bin",
			"offset":    offset,
			"final":     i == chunks-1,
			"committed": i == chunks-1,
		}}
	}
	fake := &fakeRequester{responses: responses}

	if err := Upload(context.Background(), fake, "agent-1", localPath, "/remote/upload.bin", time.Second); err != nil {
		t.Fatalf("Upload() error = %v", err)
	}
	if len(fake.requests) != chunks {
		t.Fatalf("request count = %d, want %d", len(fake.requests), chunks)
	}

	var expectedOffset int64
	var transferID string
	for i, request := range fake.requests {
		if request.msgType != protocol.MsgSendFile {
			t.Fatalf("request %d type = %q, want %q", i, request.msgType, protocol.MsgSendFile)
		}
		var payload testSendFilePayload
		encodePayload(t, request.payload, &payload)
		if len(payload.Data) > DefaultChunkSize {
			t.Fatalf("request %d sent %d bytes, exceeds %d", i, len(payload.Data), DefaultChunkSize)
		}
		if payload.Offset != expectedOffset || payload.TotalSize != int64(len(data)) {
			t.Fatalf("request %d offset/total = %d/%d, want %d/%d", i, payload.Offset, payload.TotalSize, expectedOffset, len(data))
		}
		if payload.TransferID == "" {
			t.Fatalf("request %d has no transfer id", i)
		}
		if transferID == "" {
			transferID = payload.TransferID
		} else if payload.TransferID != transferID {
			t.Fatalf("request %d changed transfer id", i)
		}
		if payload.Final != (i == chunks-1) {
			t.Fatalf("request %d final = %v", i, payload.Final)
		}
		if payload.Final && payload.Checksum != checksum(data) {
			t.Fatalf("final checksum = %q, want %q", payload.Checksum, checksum(data))
		}
		expectedOffset += int64(len(payload.Data))
	}
}

func TestUploadStopsWhenAgentRejectsAChunk(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(localPath, bytes.Repeat([]byte("x"), DefaultChunkSize+1), 0600); err != nil {
		t.Fatalf("create upload source: %v", err)
	}

	fake := &fakeRequester{
		responses: []protocol.Message{{Type: protocol.MsgFileAck, Payload: map[string]any{}}, {Type: protocol.MsgError, Error: "reject"}},
	}
	if err := Upload(context.Background(), fake, "agent-1", localPath, "/remote/upload.bin", time.Second); err == nil {
		t.Fatal("Upload() succeeded after an agent rejected a chunk")
	}
}

func TestUploadCancelsRemoteTemporaryAfterAChunkFailure(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "upload.bin")
	if err := os.WriteFile(localPath, bytes.Repeat([]byte("x"), DefaultChunkSize+1), 0600); err != nil {
		t.Fatalf("create upload source: %v", err)
	}

	fake := &fakeRequester{
		responses: []protocol.Message{
			{Type: protocol.MsgFileAck, Payload: map[string]any{"offset": 0, "final": false}},
			{Type: protocol.MsgError, Error: "connection lost"},
		},
	}
	if err := Upload(context.Background(), fake, "agent-1", localPath, "/remote/upload.bin", time.Second); err == nil {
		t.Fatal("Upload() succeeded after a chunk failure")
	}
	if len(fake.requests) != 3 {
		t.Fatalf("request count = %d, want data, failed data, cancel", len(fake.requests))
	}
	if fake.requests[2].msgType != "cancel_file" {
		t.Fatalf("cleanup request type = %q, want cancel_file", fake.requests[2].msgType)
	}
	var canceled struct {
		Path       string `json:"path"`
		TransferID string `json:"transfer_id"`
	}
	encodePayload(t, fake.requests[2].payload, &canceled)
	if canceled.Path != "/remote/upload.bin" || canceled.TransferID == "" {
		t.Fatalf("cleanup payload = %#v", canceled)
	}
}
