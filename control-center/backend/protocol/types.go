package protocol

import "time"

// Message types (direction: client -> agent)
const (
	MsgExecCommand = "exec_command"
	MsgListDir     = "list_dir"
	MsgGetFile     = "get_file"
	MsgSendFile    = "send_file"
	MsgCancelFile  = "cancel_file"
	MsgScreenshot  = "screenshot"
	MsgSystemInfo  = "system_info"
	MsgAuth        = "auth"
	MsgKeepAlive   = "keep_alive"
	MsgScriptRun   = "script_run"
)

// Message types (direction: agent -> client)
const (
	MsgCommandResult  = "command_result"
	MsgDirContents    = "dir_contents"
	MsgFileChunk      = "file_chunk"
	MsgFileAck        = "file_ack"
	MsgScreenshotData = "screenshot_data"
	MsgSystemUpdate   = "system_update"
	MsgError          = "error"
	MsgAuthRequired   = "auth_required"
	MsgAuthOk         = "auth_ok"
	MsgAgentInfo      = "agent_info"
)

// Message is the base envelope for all WebSocket messages
type Message struct {
	ID        string      `json:"id,omitempty"`
	Type      string      `json:"type"`
	Payload   interface{} `json:"payload"`
	Timestamp time.Time   `json:"timestamp"`
	Error     string      `json:"error,omitempty"`
}

// --- Request Payloads ---

type ExecCommandPayload struct {
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Timeout int      `json:"timeout,omitempty"` // seconds, 0 = no timeout
	Shell   string   `json:"shell,omitempty"`   // "" = default (cmd/powershell/bash)
}

type ListDirPayload struct {
	Path string `json:"path"`
}

type GetFilePayload struct {
	Path      string `json:"path"`
	Offset    int64  `json:"offset,omitempty"`
	ChunkSize int    `json:"chunk_size,omitempty"` // default 64KB
}

type SendFilePayload struct {
	Path       string `json:"path"`
	Data       []byte `json:"data,omitempty"`
	Offset     int64  `json:"offset,omitempty"`
	TotalSize  int64  `json:"total_size,omitempty"`
	Final      bool   `json:"final,omitempty"`
	Checksum   string `json:"checksum,omitempty"`
	TransferID string `json:"transfer_id,omitempty"`
}

type CancelFilePayload struct {
	Path       string `json:"path"`
	TransferID string `json:"transfer_id"`
}

type AuthPayload struct {
	Token    string `json:"token"`
	Username string `json:"username,omitempty"`
}

type ScriptRunPayload struct {
	Name   string   `json:"name"`
	Script string   `json:"script"`
	Args   []string `json:"args,omitempty"`
}

// --- Response Payloads ---

type CommandResultPayload struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exit_code"`
	Duration int    `json:"duration_ms"`
}

type DirEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime string `json:"mod_time"`
}

type DirContentsPayload struct {
	Path    string     `json:"path"`
	Entries []DirEntry `json:"entries"`
	Total   int        `json:"total"`
}

type FileChunkPayload struct {
	Path      string `json:"path"`
	Data      []byte `json:"data"`
	Offset    int64  `json:"offset"`
	TotalSize int64  `json:"total_size,omitempty"`
	Final     bool   `json:"final"`
	Checksum  string `json:"checksum,omitempty"`
}

type SystemInfoPayload struct {
	Hostname     string     `json:"hostname"`
	OS           string     `json:"os"`
	Platform     string     `json:"platform"`
	Arch         string     `json:"arch"`
	CPU          CPUInfo    `json:"cpu"`
	Memory       MemoryInfo `json:"memory"`
	Disks        []DiskInfo `json:"disks"`
	Uptime       uint64     `json:"uptime"`
	Net          NetInfo    `json:"net"`
	AgentVersion string     `json:"agent_version"`
}

type CPUInfo struct {
	Percent float64 `json:"percent"`
	Cores   int     `json:"cores"`
	Model   string  `json:"model"`
}

type MemoryInfo struct {
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

type DiskInfo struct {
	Mount   string  `json:"mount"`
	FSType  string  `json:"fs_type"`
	Total   uint64  `json:"total"`
	Used    uint64  `json:"used"`
	Free    uint64  `json:"free"`
	Percent float64 `json:"percent"`
}

type NetInfo struct {
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Hostname string `json:"hostname"`
}

type ScreenshotDataPayload struct {
	Format string `json:"format"` // "png" or "jpeg"
	Data   []byte `json:"data"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

type AgentInfoPayload struct {
	Hostname      string `json:"hostname"`
	OS            string `json:"os"`
	Arch          string `json:"arch"`
	AgentVersion  string `json:"agent_version"`
	Port          int    `json:"port"`
	Authenticated bool   `json:"authenticated"`
}
