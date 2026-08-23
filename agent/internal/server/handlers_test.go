package server

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

func handlerChecksum(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func sendFileMessage(t *testing.T, conn interface{ WriteJSON(any) error }, id, path, transferID string, data []byte, offset, total int64, final bool, checksum string) {
	t.Helper()
	if err := conn.WriteJSON(protocol.Message{
		ID:   id,
		Type: protocol.MsgSendFile,
		Payload: map[string]any{
			"path":        path,
			"transfer_id": transferID,
			"data":        data,
			"offset":      offset,
			"total_size":  total,
			"final":       final,
			"checksum":    checksum,
		},
	}); err != nil {
		t.Fatalf("send file message %s: %v", id, err)
	}
}

func TestSendFileUsesRemoteTemporaryPathAndCommitsValidatedFinalChunk(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "", ReadTimeout)
	greeting := readTestMessage(t, conn)
	if greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAgentInfo)
	}

	destination := filepath.Join(t.TempDir(), "uploaded.bin")
	transferID := "handler-success"
	content := []byte("server-side atomic upload")
	first := content[:10]
	second := content[10:]

	sendFileMessage(t, conn, "chunk-1", destination, transferID, first, 0, int64(len(content)), false, "")
	firstAck := readTestMessage(t, conn)
	if firstAck.Type != protocol.MsgFileAck {
		t.Fatalf("first response type = %q, want %q", firstAck.Type, protocol.MsgFileAck)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists before final chunk: %v", err)
	}

	sendFileMessage(t, conn, "chunk-2", destination, transferID, second, int64(len(first)), int64(len(content)), true, handlerChecksum(content))
	finalAck := readTestMessage(t, conn)
	if finalAck.Type != protocol.MsgFileAck {
		t.Fatalf("final response type = %q, want %q", finalAck.Type, protocol.MsgFileAck)
	}
	var ack map[string]any
	encoded, err := json.Marshal(finalAck.Payload)
	if err != nil {
		t.Fatalf("marshal final ack: %v", err)
	}
	if err := json.Unmarshal(encoded, &ack); err != nil {
		t.Fatalf("decode final ack: %v", err)
	}
	if committed, ok := ack["committed"].(bool); !ok || !committed {
		t.Fatalf("final ack committed = %#v, want true", ack["committed"])
	}

	actual, err := os.ReadFile(destination)
	if err != nil {
		t.Fatalf("read committed destination: %v", err)
	}
	if string(actual) != string(content) {
		t.Fatalf("committed content = %q, want %q", actual, content)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("remote upload temporary files remain: %v", matches)
	}
}

func TestSendFileRejectsChecksumMismatchAndRemovesTemporaryFile(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("unexpected greeting: %q", greeting.Type)
	}

	destination := filepath.Join(t.TempDir(), "uploaded.bin")
	transferID := "handler-checksum"
	content := []byte("checksum content")
	sendFileMessage(t, conn, "bad-checksum", destination, transferID, content, 0, int64(len(content)), true, handlerChecksum([]byte("wrong")))
	response := readTestMessage(t, conn)
	if response.Type != protocol.MsgError {
		t.Fatalf("response type = %q, want %q", response.Type, protocol.MsgError)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after checksum failure: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after checksum failure: %v", matches)
	}
}

func TestSendFileRejectsOffsetMismatch(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("unexpected greeting: %q", greeting.Type)
	}

	destination := filepath.Join(t.TempDir(), "uploaded.bin")
	sendFileMessage(t, conn, "bad-offset", destination, "handler-offset", []byte("data"), 1, 4, true, handlerChecksum([]byte("data")))
	response := readTestMessage(t, conn)
	if response.Type != protocol.MsgError {
		t.Fatalf("response type = %q, want %q", response.Type, protocol.MsgError)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after offset failure: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after offset failure: %v", matches)
	}
}

func TestCancelFileRemovesRemoteTemporaryUpload(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("unexpected greeting: %q", greeting.Type)
	}

	destination := filepath.Join(t.TempDir(), "uploaded.bin")
	transferID := "handler-cancel"
	sendFileMessage(t, conn, "partial", destination, transferID, []byte("partial"), 0, 20, false, "")
	if response := readTestMessage(t, conn); response.Type != protocol.MsgFileAck {
		t.Fatalf("partial response type = %q, want %q", response.Type, protocol.MsgFileAck)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:   "cancel",
		Type: "cancel_file",
		Payload: map[string]any{
			"path":        destination,
			"transfer_id": transferID,
		},
	}); err != nil {
		t.Fatalf("send cancel message: %v", err)
	}
	response := readTestMessage(t, conn)
	if response.Type != protocol.MsgFileAck {
		t.Fatalf("cancel response type = %q, want %q", response.Type, protocol.MsgFileAck)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("destination exists after cancel: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(filepath.Dir(destination), ".lan-commander-upload-*")); len(matches) != 0 {
		t.Fatalf("temporary files remain after cancel: %v", matches)
	}
}
