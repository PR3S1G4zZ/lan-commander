package server

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"

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
	if !c.authed {
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

	if c.server.authToken != "" && auth.Token != c.server.authToken {
		log.Printf("[client %s] Auth failed from %s", c.id, auth.Username)
		c.sendError(msg.ID, "invalid authentication token")
		return
	}

	c.authed = true
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

	result, err := executor.Execute(execPayload.Command, execPayload.Args, execPayload.Timeout, execPayload.Shell)
	if err != nil {
		c.sendError(msg.ID, fmt.Sprintf("execution error: %v", err))
		return
	}

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

	contents, err := filesystem.ListDir(listPayload.Path)
	if err != nil {
		c.sendError(msg.ID, fmt.Sprintf("list_dir error: %v", err))
		return
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

	data, totalSize, err := filesystem.ReadFileChunk(getPayload.Path, getPayload.Offset, chunkSize)
	if err != nil {
		c.sendError(msg.ID, fmt.Sprintf("get_file error: %v", err))
		return
	}

	final := getPayload.Offset+int64(len(data)) >= totalSize

	// Compute SHA256 checksum for the final chunk
	checksum := ""
	if final {
		h := sha256.Sum256(data)
		checksum = fmt.Sprintf("%x", h)
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

	if err := filesystem.WriteFileChunk(sendPayload.Path, sendPayload.Data, sendPayload.Offset); err != nil {
		c.sendError(msg.ID, fmt.Sprintf("send_file error: %v", err))
		return
	}

	// Acknowledge the chunk
	c.sendResponse(msg.ID, protocol.MsgFileAck, map[string]interface{}{
		"path":   sendPayload.Path,
		"offset": sendPayload.Offset,
		"final":  sendPayload.Final,
	})
}

// handleScreenshot captures a screenshot and returns it.
func (c *Client) handleScreenshot(msg protocol.Message) {
	data, width, height, err := agentScreenshot.CaptureAll()
	if err != nil {
		c.sendError(msg.ID, fmt.Sprintf("screenshot error: %v", err))
		return
	}

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
