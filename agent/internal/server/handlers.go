package server

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/mediacode/lan-commander/agent/internal/audit"
	"github.com/mediacode/lan-commander/agent/internal/executor"
	"github.com/mediacode/lan-commander/agent/internal/filesystem"
	"github.com/mediacode/lan-commander/agent/internal/protocol"
	agentScreenshot "github.com/mediacode/lan-commander/agent/internal/screenshot"
)

// handleMessage dispatches a message to its type-specific handler.
func (c *Client) handleMessage(msg protocol.Message) {
	// auth is always allowed even if not authenticated
	if msg.Type == protocol.MsgAuth {
		c.handleAuth(msg)
		return
	}

	// keep_alive is always allowed
	if msg.Type == protocol.MsgKeepAlive {
		return
	}

	// All other messages require authentication
	if !c.authed.Load() {
		c.sendError(msg.ID, "authentication required")
		return
	}

	switch msg.Type {
	case protocol.MsgExecCommand:
		c.handleExecCommand(msg)
	case protocol.MsgListDir:
		c.handleListDir(msg)
	case protocol.MsgGetFile:
		c.handleGetFile(msg)
	case protocol.MsgSendFile:
		c.handleSendFile(msg)
	case protocol.MsgCancelFile:
		c.handleCancelFile(msg)
	case protocol.MsgScreenshot:
		c.handleScreenshot(msg)
	case protocol.MsgSystemInfo:
		c.handleSystemInfo(msg)
	default:
		c.sendError(msg.ID, fmt.Sprintf("unknown message type: %s", msg.Type))
	}
}

// handleAuth processes authentication messages.
func (c *Client) handleAuth(msg protocol.Message) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		c.sendError(msg.ID, "invalid auth payload")
		return
	}

	var auth protocol.AuthPayload
	if err := json.Unmarshal(payloadBytes, &auth); err != nil {
		c.sendError(msg.ID, "invalid auth payload format")
		return
	}

	if c.server.authToken != "" && subtle.ConstantTimeCompare([]byte(auth.Token), []byte(c.server.authToken)) != 1 {
		attempts := c.authAttempts.Add(1)
		log.Printf("[client %s] Auth failed from %s (attempt %d/%d)", c.id, auth.Username, attempts, MaxAuthAttempts)
		c.server.guard.fail(c.ip)
		c.user.Store(auth.Username)
		c.record("auth", audit.ResultDenied, fmt.Sprintf("invalid token (attempt %d/%d)", attempts, MaxAuthAttempts))
		c.sendError(msg.ID, "invalid authentication token")
		if attempts >= MaxAuthAttempts {
			log.Printf("[client %s] Too many failed auth attempts, closing connection", c.id)
			c.close()
		}
		return
	}

	c.authed.Store(true)
	c.server.guard.succeed(c.ip)
	c.user.Store(auth.Username)
	c.record("auth", audit.ResultOK, "")
	log.Printf("[client %s] Authenticated (user: %s)", c.id, auth.Username)

	c.sendResponse(msg.ID, protocol.MsgAuthOk, map[string]string{
		"message": "authenticated successfully",
	})

	// Send agent info after successful auth
	c.sendAgentInfo()
}

// handleExecCommand runs a command and returns the result.
func (c *Client) handleExecCommand(msg protocol.Message) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		c.sendError(msg.ID, "invalid exec payload")
		return
	}

	var execPayload protocol.ExecCommandPayload
	if err := json.Unmarshal(payloadBytes, &execPayload); err != nil {
		c.sendError(msg.ID, "invalid exec payload format")
		return
	}

	started := time.Now()
	result, err := executor.Execute(execPayload.Command, execPayload.Args, execPayload.Timeout, execPayload.Shell)
	if err != nil {
		c.record("exec_command", audit.ResultError, fmt.Sprintf("%s: %v", execPayload.Command, err))
		c.sendError(msg.ID, fmt.Sprintf("execution error: %v", err))
		return
	}
	c.record("exec_command", audit.ResultOK, fmt.Sprintf("exit=%d duration=%s shell=%q command=%s",
		result.ExitCode, time.Since(started).Round(time.Millisecond), execPayload.Shell, execPayload.Command))

	c.sendResponse(msg.ID, protocol.MsgCommandResult, result)
}

// handleListDir lists the contents of a directory.
func (c *Client) handleListDir(msg protocol.Message) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		c.sendError(msg.ID, "invalid list_dir payload")
		return
	}

	var listPayload protocol.ListDirPayload
	if err := json.Unmarshal(payloadBytes, &listPayload); err != nil {
		c.sendError(msg.ID, "invalid list_dir payload format")
		return
	}

	contents, err := filesystem.ListDir(listPayload.Path, listPayload.Offset, listPayload.Limit)
	if err != nil {
		c.record("list_dir", audit.ResultError, fmt.Sprintf("%s: %v", listPayload.Path, err))
		c.sendError(msg.ID, fmt.Sprintf("list_dir error: %v", err))
		return
	}
	// Only the first page is recorded; the rest are the same browsing action.
	if listPayload.Offset == 0 {
		c.record("list_dir", audit.ResultOK, listPayload.Path)
	}

	c.sendResponse(msg.ID, protocol.MsgDirContents, contents)
}

// handleGetFile reads a file chunk and returns it.
func (c *Client) handleGetFile(msg protocol.Message) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		c.sendError(msg.ID, "invalid get_file payload")
		return
	}

	var getPayload protocol.GetFilePayload
	if err := json.Unmarshal(payloadBytes, &getPayload); err != nil {
		c.sendError(msg.ID, "invalid get_file payload format")
		return
	}

	chunkSize := getPayload.ChunkSize
	if chunkSize <= 0 {
		chunkSize = filesystem.DefaultChunkSize
	}
	if chunkSize > filesystem.MaxChunkSize {
		chunkSize = filesystem.MaxChunkSize
	}

	data, totalSize, err := filesystem.ReadFileChunk(getPayload.Path, getPayload.Offset, chunkSize)
	if err != nil {
		c.record("get_file", audit.ResultError, fmt.Sprintf("%s: %v", getPayload.Path, err))
		c.sendError(msg.ID, fmt.Sprintf("get_file error: %v", err))
		return
	}
	// A download is many chunk requests; record it once, when it starts.
	if getPayload.Offset == 0 {
		c.record("get_file", audit.ResultOK, fmt.Sprintf("%s (%d bytes)", getPayload.Path, totalSize))
	}

	final := getPayload.Offset+int64(len(data)) >= totalSize

	// Compute the checksum of the complete file on the final chunk.
	checksum := ""
	if final {
		checksum, err = filesystem.FileSHA256(getPayload.Path)
		if err != nil {
			c.sendError(msg.ID, fmt.Sprintf("checksum error: %v", err))
			return
		}
	}

	c.sendResponse(msg.ID, protocol.MsgFileChunk, protocol.FileChunkPayload{
		Path:      getPayload.Path,
		Data:      data,
		Offset:    getPayload.Offset,
		TotalSize: totalSize,
		Final:     final,
		Checksum:  checksum,
	})
}

// handleSendFile writes a file chunk from the client.
func (c *Client) handleSendFile(msg protocol.Message) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		c.sendError(msg.ID, "invalid send_file payload")
		return
	}

	var sendPayload protocol.SendFilePayload
	if err := json.Unmarshal(payloadBytes, &sendPayload); err != nil {
		c.sendError(msg.ID, "invalid send_file payload format")
		return
	}

	committed := false
	if sendPayload.TransferID == "" {
		// Keep the original protocol behavior for older clients that do not send
		// an atomic transfer ID.
		if err := filesystem.WriteFileChunk(sendPayload.Path, sendPayload.Data, sendPayload.Offset); err != nil {
			c.sendError(msg.ID, fmt.Sprintf("send_file error: %v", err))
			return
		}
		committed = sendPayload.Final
	} else {
		if err := filesystem.WriteAtomicUploadChunk(
			sendPayload.Path,
			sendPayload.TransferID,
			sendPayload.Data,
			sendPayload.Offset,
			sendPayload.TotalSize,
			sendPayload.Final,
			sendPayload.Checksum,
		); err != nil {
			c.sendError(msg.ID, fmt.Sprintf("send_file error: %v", err))
			return
		}
		committed = sendPayload.Final
	}

	// An upload is many chunks; record where it starts and where it ends.
	if sendPayload.Offset == 0 || sendPayload.Final {
		c.record("send_file", audit.ResultOK, fmt.Sprintf("%s offset=%d final=%t committed=%t",
			sendPayload.Path, sendPayload.Offset, sendPayload.Final, committed))
	}

	// Acknowledge the chunk. The committed field is only part of the extended
	// atomic-transfer contract; legacy clients keep the original ACK shape.
	ack := map[string]interface{}{
		"path":   sendPayload.Path,
		"offset": sendPayload.Offset,
		"final":  sendPayload.Final,
	}
	if sendPayload.TransferID != "" {
		ack["committed"] = committed
	}
	c.sendResponse(msg.ID, protocol.MsgFileAck, ack)
}

func (c *Client) handleCancelFile(msg protocol.Message) {
	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		c.sendError(msg.ID, "invalid cancel_file payload")
		return
	}
	var cancelPayload protocol.CancelFilePayload
	if err := json.Unmarshal(payloadBytes, &cancelPayload); err != nil {
		c.sendError(msg.ID, "invalid cancel_file payload format")
		return
	}
	if err := filesystem.CancelAtomicUpload(cancelPayload.Path, cancelPayload.TransferID); err != nil {
		c.record("cancel_file", audit.ResultError, fmt.Sprintf("%s: %v", cancelPayload.Path, err))
		c.sendError(msg.ID, fmt.Sprintf("cancel_file error: %v", err))
		return
	}
	c.record("cancel_file", audit.ResultOK, cancelPayload.Path)
	c.sendResponse(msg.ID, protocol.MsgFileAck, map[string]interface{}{
		"path":      cancelPayload.Path,
		"committed": false,
		"canceled":  true,
	})
}

// handleScreenshot captures a screenshot and returns it.
func (c *Client) handleScreenshot(msg protocol.Message) {
	data, width, height, err := agentScreenshot.CaptureAll()
	if err != nil {
		c.record("screenshot", audit.ResultError, err.Error())
		c.sendError(msg.ID, fmt.Sprintf("screenshot error: %v", err))
		return
	}
	c.record("screenshot", audit.ResultOK, fmt.Sprintf("%dx%d", width, height))

	c.sendResponse(msg.ID, protocol.MsgScreenshotData, protocol.ScreenshotDataPayload{
		Format: "png",
		Data:   data,
		Width:  width,
		Height: height,
	})
}

// handleSystemInfo returns system information immediately.
func (c *Client) handleSystemInfo(msg protocol.Message) {
	info := c.server.monitor.GetSystemInfo()
	c.sendResponse(msg.ID, protocol.MsgSystemUpdate, info)
}
