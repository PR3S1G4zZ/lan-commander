package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"control-center/backend/audit"
	"control-center/backend/client"
	"control-center/backend/discovery"
	"control-center/backend/protocol"
	"control-center/backend/scripting"
	"control-center/backend/session"
	"control-center/backend/wol"
)

const (
	defaultTimeout = 30 * time.Second
	appName        = "LAN Commander Control Center"
)

// App is the main application structure for Wails.
type App struct {
	ctx       context.Context
	clientMgr *client.Manager
	discover  *discovery.Discovery
	sessions  *session.Manager
	audit     *audit.Logger
	scripts   *scripting.Engine
	wolSender *wol.Sender
	dataDir   string
	mu        sync.Mutex
}

// NewApp creates a new App application struct.
func NewApp() *App {
	return &App{}
}

// resolveDataDir returns the per-user directory where databases and scripts
// live (%APPDATA%\LAN Commander on Windows, ~/.config/LAN Commander on Linux).
// It falls back to the executable's directory only if the user config dir is
// unavailable, and to the working directory as a last resort.
func resolveDataDir() string {
	if cfgDir, err := os.UserConfigDir(); err == nil {
		dir := filepath.Join(cfgDir, "LAN Commander")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		} else {
			log.Printf("app: cannot create data dir %q: %v", dir, err)
		}
	}

	if exe, err := os.Executable(); err == nil {
		log.Printf("app: falling back to executable directory for data storage")
		return filepath.Dir(exe)
	}

	log.Printf("app: falling back to working directory for data storage")
	return "."
}

// startup is called when the app starts. It initializes all services.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// Determine data directory. We deliberately avoid the executable's own
	// directory: when installed under Program Files it is not writable by a
	// non-elevated process, which silently broke session/audit persistence.
	a.dataDir = resolveDataDir()
	log.Printf("app: data directory: %s", a.dataDir)

	// Initialize session manager (SQLite)
	a.sessions = session.NewManager()
	if err := a.sessions.Open(a.dataDir); err != nil {
		log.Printf("app: failed to open session database: %v", err)
	}

	// Initialize audit logger
	a.audit = audit.NewLogger()
	dbPath := filepath.Join(a.dataDir, "audit.db")
	if err := a.audit.OpenDB(dbPath); err != nil {
		log.Printf("app: failed to open audit database: %v", err)
	}

	// Initialize script engine
	scriptsDir := filepath.Join(a.dataDir, "scripts")
	a.scripts = scripting.NewEngineWithPath(scriptsDir)

	// Initialize WOL sender
	a.wolSender = wol.NewSender()

	// Initialize client manager (WebSocket connections to agents)
	a.clientMgr = client.NewManager(a.onAgentMessage)

	// Initialize discovery (mDNS)
	a.discover = discovery.NewDiscovery(a.onAgentDiscovered)

	// Restore previous sessions and reconnect in background
	go func() {
		savedSessions, err := a.sessions.LoadAll()
		if err != nil {
			log.Printf("app: failed to load saved sessions: %v", err)
		}

		for _, s := range savedSessions {
			agentID, err := a.clientMgr.Connect(s.Host, s.Port, s.AuthToken)
			if err != nil {
				log.Printf("app: failed to reconnect to %s:%d: %v", s.Host, s.Port, err)
				continue
			}

			// Update last connected
			_ = a.sessions.UpdateLastConnected(s.ID)

			// Request system info to populate agent details
			go func(id string) {
				sysInfo, err := a.requestSystemInfo(id)
				if err != nil {
					log.Printf("app: failed to get system info for %s: %v", id, err)
				} else if sysInfo != nil {
					a.audit.Log("system_info", id, "system", fmt.Sprintf("Hostname: %s, OS: %s, Arch: %s", sysInfo.Hostname, sysInfo.OS, sysInfo.Arch), audit.StatusSuccess)
				}
			}(agentID)
		}

		// Start mDNS discovery
		a.discover.Start()
	}()

	log.Printf("%s started successfully", appName)
}

// shutdown is called when the app is closing.
func (a *App) shutdown(ctx context.Context) {
	log.Printf("app: shutting down...")

	if a.discover != nil {
		a.discover.Stop()
	}

	if a.clientMgr != nil {
		a.clientMgr.CloseAll()
	}

	if a.sessions != nil {
		a.sessions.Close()
	}

	if a.audit != nil {
		a.audit.Close()
	}

	log.Printf("app: shutdown complete")
}

// --- Agent Discovery Callback ---

func (a *App) onAgentDiscovered(d discovery.AgentDiscovered) {
	a.audit.Log("agent_discovered", "", "system",
		fmt.Sprintf("Agent %s discovered at %s:%d (os=%s, arch=%s)", d.Name, d.Host, d.Port, d.OS, d.Arch),
		audit.StatusSuccess)

	// Auto-connect to discovered agents that aren't already connected
	existing := a.clientMgr.ListAgents()
	for _, agent := range existing {
		if agent.Host == d.Host && agent.Port == d.Port {
			return // Already known
		}
	}

	// Reuse the token of a saved session for this host:port if we have one.
	// Agents are token-protected by default, so a blind anonymous connect
	// would just be rejected.
	token := a.savedTokenFor(d.Host, d.Port)

	if _, err := a.clientMgr.Connect(d.Host, d.Port, token); err != nil {
		if errors.Is(err, client.ErrAuthRequired) {
			// Expected for any agent we have no credentials for: surface it in
			// the audit log so the admin knows to add it manually, and stop.
			a.audit.Log("agent_discovered", "", "system",
				fmt.Sprintf("Agent %s at %s:%d requires a token; add it manually to connect", d.Name, d.Host, d.Port),
				audit.StatusWarning)
			return
		}
		log.Printf("app: failed to auto-connect to discovered agent %s:%d: %v", d.Host, d.Port, err)
	}
}

// savedTokenFor returns the auth token of a saved session matching host:port,
// or an empty string when none exists.
func (a *App) savedTokenFor(host string, port int) string {
	if a.sessions == nil {
		return ""
	}
	saved, err := a.sessions.LoadAll()
	if err != nil {
		return ""
	}
	for _, s := range saved {
		if s.Host == host && s.Port == port {
			return s.AuthToken
		}
	}
	return ""
}

// --- Agent Message Callback ---

func (a *App) onAgentMessage(agentID string, msg *protocol.Message) {
	// Log received messages for auditing
	switch msg.Type {
	case protocol.MsgCommandResult:
		a.audit.Log("command_result", agentID, "system", "Command result received", audit.StatusSuccess)
	case protocol.MsgSystemUpdate:
		a.audit.Log("system_update", agentID, "system", "System info update received", audit.StatusSuccess)
	case protocol.MsgError:
		errDetail := msg.Error
		if errDetail == "" {
			errDetail = "unknown error"
		}
		a.audit.Log("agent_error", agentID, "system", errDetail, audit.StatusError)
	}
}

// --- Binding: GetAgents ---

// GetAgents returns information about all known (connected and disconnected) agents.
func (a *App) GetAgents() []client.AgentInfo {
	a.mu.Lock()
	defer a.mu.Unlock()

	agents := a.clientMgr.ListAgents()
	if agents == nil {
		return []client.AgentInfo{}
	}
	return agents
}

// --- Binding: ConnectAgent ---

// ConnectAgent establishes a WebSocket connection to an agent.
func (a *App) ConnectAgent(host string, port int, authToken string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	agentID, err := a.clientMgr.Connect(host, port, authToken)
	if err != nil {
		a.audit.Log("connect", "", "user", fmt.Sprintf("Failed to connect to %s:%d: %v", host, port, err), audit.StatusError)
		return fmt.Errorf("failed to connect to agent: %w", err)
	}

	a.audit.Log("connect", agentID, "user", fmt.Sprintf("Connected to %s:%d", host, port), audit.StatusSuccess)

	// Auto-save session
	s := session.Session{
		Name:      fmt.Sprintf("%s:%d", host, port),
		Host:      host,
		Port:      port,
		AuthToken: authToken,
	}
	if _, err := a.sessions.Save(s); err != nil {
		log.Printf("app: failed to save session: %v", err)
	}

	return nil
}

// --- Binding: DisconnectAgent ---

// DisconnectAgent gracefully disconnects from an agent.
func (a *App) DisconnectAgent(agentID string) error {
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if err := a.clientMgr.Disconnect(agentID); err != nil {
		a.audit.Log("disconnect", agentID, "user", fmt.Sprintf("Failed to disconnect: %v", err), audit.StatusError)
		return fmt.Errorf("failed to disconnect: %w", err)
	}

	a.audit.Log("disconnect", agentID, "user", "Disconnected from agent", audit.StatusSuccess)
	return nil
}

// --- Binding: ExecCommand ---

// ExecCommand executes a command on a specific agent.
func (a *App) ExecCommand(agentID string, command string, timeout int) (*protocol.CommandResultPayload, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if command == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}

	duration := defaultTimeout
	if timeout > 0 {
		duration = time.Duration(timeout) * time.Second
	}

	a.audit.Log("exec_command", agentID, "user", fmt.Sprintf("Command: %s", command), audit.StatusSuccess)

	result, err := a.requestCommandResult(agentID, command, duration)
	if err != nil {
		a.audit.Log("exec_command", agentID, "user", fmt.Sprintf("Command failed: %v", err), audit.StatusError)
		return nil, fmt.Errorf("command execution failed: %w", err)
	}

	return result, nil
}

// --- Binding: ExecCommandMulti ---

// ExecCommandMulti executes a command on multiple agents in parallel.
func (a *App) ExecCommandMulti(agentIDs []string, command string, timeout int) map[string]*protocol.CommandResultPayload {
	results := make(map[string]*protocol.CommandResultPayload)

	if len(agentIDs) == 0 || command == "" {
		return results
	}

	duration := defaultTimeout
	if timeout > 0 {
		duration = time.Duration(timeout) * time.Second
	}

	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, agentID := range agentIDs {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()

			result, err := a.requestCommandResult(id, command, duration)
			mu.Lock()
			if err != nil {
				results[id] = &protocol.CommandResultPayload{
					Stdout:   "",
					Stderr:   err.Error(),
					ExitCode: -1,
				}
			} else {
				results[id] = result
			}
			mu.Unlock()
		}(agentID)
	}

	wg.Wait()

	a.audit.Log("exec_command_multi", "", "user",
		fmt.Sprintf("Executed command on %d agents: %s", len(agentIDs), command),
		audit.StatusSuccess)

	return results
}

// --- Binding: ListDir ---

// ListDir lists the contents of a directory on an agent.
func (a *App) ListDir(agentID string, path string) (*protocol.DirContentsPayload, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if path == "" {
		return nil, fmt.Errorf("path cannot be empty")
	}

	a.audit.Log("list_dir", agentID, "user", fmt.Sprintf("Path: %s", path), audit.StatusSuccess)

	var result protocol.DirContentsPayload
	err := a.clientMgr.SendAndParse(agentID, protocol.MsgListDir, protocol.ListDirPayload{Path: path}, &result, defaultTimeout)
	if err != nil {
		a.audit.Log("list_dir", agentID, "user", fmt.Sprintf("Failed: %v", err), audit.StatusError)
		return nil, fmt.Errorf("failed to list directory: %w", err)
	}

	return &result, nil
}

// --- Binding: GetSystemInfo ---

// GetSystemInfo retrieves system information from an agent.
func (a *App) GetSystemInfo(agentID string) (*protocol.SystemInfoPayload, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}

	a.audit.Log("system_info", agentID, "user", "Requesting system info", audit.StatusSuccess)

	result, err := a.requestSystemInfo(agentID)
	if err != nil {
		a.audit.Log("system_info", agentID, "user", fmt.Sprintf("Failed: %v", err), audit.StatusError)
		return nil, fmt.Errorf("failed to get system info: %w", err)
	}

	return result, nil
}

// --- Binding: RequestScreenshot ---

// RequestScreenshot requests a screenshot from an agent.
func (a *App) RequestScreenshot(agentID string) (*protocol.ScreenshotDataPayload, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}

	a.audit.Log("screenshot", agentID, "user", "Requesting screenshot", audit.StatusSuccess)

	var result protocol.ScreenshotDataPayload
	err := a.clientMgr.SendAndParse(agentID, protocol.MsgScreenshot, struct{}{}, &result, defaultTimeout)
	if err != nil {
		a.audit.Log("screenshot", agentID, "user", fmt.Sprintf("Failed: %v", err), audit.StatusError)
		return nil, fmt.Errorf("failed to request screenshot: %w", err)
	}

	return &result, nil
}

// --- Binding: TransferFile ---

// TransferFile downloads a complete file from an agent and stores it atomically
// at localPath. Chunks are requested sequentially so the client never holds the
// entire remote file in memory.
func (a *App) TransferFile(agentID string, remotePath string, localPath string) error {
	if agentID == "" {
		return fmt.Errorf("agent ID cannot be empty")
	}
	if remotePath == "" {
		return fmt.Errorf("remote path cannot be empty")
	}
	if localPath == "" {
		return fmt.Errorf("local path cannot be empty")
	}

	localPath, err := filepath.Abs(localPath)
	if err != nil {
		return fmt.Errorf("invalid local path: %w", err)
	}
	parentDir := filepath.Dir(localPath)
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
	defer os.Remove(partPath)

	const chunkSize = 64 * 1024
	var offset int64
	var totalSize int64 = -1

	for {
		response, requestErr := a.clientMgr.SendRequest(agentID, protocol.MsgGetFile, protocol.GetFilePayload{
			Path:      remotePath,
			Offset:    offset,
			ChunkSize: chunkSize,
		}, defaultTimeout)
		if requestErr != nil {
			_ = partFile.Close()
			a.audit.Log("transfer_file", agentID, "user", fmt.Sprintf("Failed: %v", requestErr), audit.StatusError)
			return fmt.Errorf("file transfer failed: %w", requestErr)
		}

		payloadBytes, marshalErr := json.Marshal(response.Payload)
		if marshalErr != nil {
			_ = partFile.Close()
			return fmt.Errorf("invalid file chunk response: %w", marshalErr)
		}
		var chunk protocol.FileChunkPayload
		if unmarshalErr := json.Unmarshal(payloadBytes, &chunk); unmarshalErr != nil {
			_ = partFile.Close()
			return fmt.Errorf("invalid file chunk response: %w", unmarshalErr)
		}
		if chunk.Offset != offset {
			_ = partFile.Close()
			return fmt.Errorf("file chunk offset mismatch: received %d, expected %d", chunk.Offset, offset)
		}
		if totalSize < 0 {
			totalSize = chunk.TotalSize
		} else if chunk.TotalSize != totalSize {
			_ = partFile.Close()
			return fmt.Errorf("file size changed during transfer")
		}
		if totalSize < 0 || offset+int64(len(chunk.Data)) > totalSize {
			_ = partFile.Close()
			return fmt.Errorf("invalid file chunk size")
		}
		if len(chunk.Data) > 0 {
			if _, writeErr := partFile.WriteAt(chunk.Data, offset); writeErr != nil {
				_ = partFile.Close()
				return fmt.Errorf("cannot write downloaded chunk: %w", writeErr)
			}
		}
		offset += int64(len(chunk.Data))
		if chunk.Final {
			if offset != totalSize {
				_ = partFile.Close()
				return fmt.Errorf("final chunk ended at %d, expected %d", offset, totalSize)
			}
			if chunk.Checksum != "" {
				if err := verifyFileChecksum(partFile, chunk.Checksum); err != nil {
					_ = partFile.Close()
					return err
				}
			}
			break
		}
		if len(chunk.Data) == 0 {
			_ = partFile.Close()
			return fmt.Errorf("agent returned an empty non-final file chunk")
		}
	}

	if err := partFile.Close(); err != nil {
		return fmt.Errorf("cannot close downloaded file: %w", err)
	}
	if err := os.Rename(partPath, localPath); err != nil {
		return fmt.Errorf("cannot finalize downloaded file %q: %w", localPath, err)
	}

	a.audit.Log("transfer_file", agentID, "user",
		fmt.Sprintf("Remote: %s -> Local: %s (%d bytes)", remotePath, localPath, totalSize),
		audit.StatusSuccess)
	return nil
}

func verifyFileChecksum(file *os.File, expected string) error {
	if _, err := file.Seek(0, 0); err != nil {
		return fmt.Errorf("cannot verify downloaded file: %w", err)
	}
	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return fmt.Errorf("cannot verify downloaded file: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, expected) {
		return fmt.Errorf("download checksum mismatch")
	}
	return nil
}

// --- Binding: WakeOnLAN ---

// WakeOnLAN sends a Wake-on-LAN magic packet to wake a machine.
func (a *App) WakeOnLAN(macAddr string, broadcastIP string) error {
	if macAddr == "" {
		return fmt.Errorf("MAC address cannot be empty")
	}
	if !wol.ValidateMAC(macAddr) {
		return fmt.Errorf("invalid MAC address format: %s", macAddr)
	}

	a.audit.Log("wol", "", "user", fmt.Sprintf("Sending WOL to MAC: %s via %s", macAddr, broadcastIP), audit.StatusSuccess)

	if err := a.wolSender.Send(macAddr, broadcastIP); err != nil {
		a.audit.Log("wol", "", "user", fmt.Sprintf("Failed: %v", err), audit.StatusError)
		return fmt.Errorf("failed to send WOL packet: %w", err)
	}

	return nil
}

// --- Binding: SaveSession ---

// SaveSession saves a connection as a persistent session. The auth token must
// be included: without it, reconnecting on the next launch fails against any
// agent that requires authentication (which is now the installer default).
func (a *App) SaveSession(host string, port int, name string, authToken string) error {
	if host == "" {
		return fmt.Errorf("host cannot be empty")
	}
	if port < 1 || port > 65535 {
		return fmt.Errorf("invalid port: %d", port)
	}
	if name == "" {
		name = fmt.Sprintf("%s:%d", host, port)
	}

	s := session.Session{
		Name:      name,
		Host:      host,
		Port:      port,
		AuthToken: authToken,
	}

	if _, err := a.sessions.Save(s); err != nil {
		a.audit.Log("save_session", "", "user", fmt.Sprintf("Failed to save session: %v", err), audit.StatusError)
		return fmt.Errorf("failed to save session: %w", err)
	}

	a.audit.Log("save_session", "", "user", fmt.Sprintf("Saved session: %s (%s:%d)", name, host, port), audit.StatusSuccess)
	return nil
}

// --- Binding: DeleteSession ---

// DeleteSession removes a saved session.
func (a *App) DeleteSession(id int64) error {
	if id <= 0 {
		return fmt.Errorf("invalid session ID: %d", id)
	}

	if err := a.sessions.Delete(id); err != nil {
		a.audit.Log("delete_session", "", "user", fmt.Sprintf("Failed to delete session %d: %v", id, err), audit.StatusError)
		return fmt.Errorf("failed to delete session: %w", err)
	}

	a.audit.Log("delete_session", "", "user", fmt.Sprintf("Deleted session %d", id), audit.StatusSuccess)
	return nil
}

// --- Binding: GetSessions ---

// GetSessions returns all saved sessions.
func (a *App) GetSessions() []session.Session {
	sessions, err := a.sessions.LoadAll()
	if err != nil {
		log.Printf("app: failed to load sessions: %v", err)
		return []session.Session{}
	}
	return sessions
}

// --- Binding: RunScript ---

// RunScript executes a script on a specific agent.
func (a *App) RunScript(agentID string, scriptName string, scriptContent string) (*protocol.CommandResultPayload, error) {
	if agentID == "" {
		return nil, fmt.Errorf("agent ID cannot be empty")
	}
	if scriptName == "" && scriptContent == "" {
		return nil, fmt.Errorf("script name or content must be provided")
	}

	var script *scripting.Script
	var err error

	if scriptName != "" {
		// Load saved script
		script, err = a.scripts.Get(scriptName)
		if err != nil {
			return nil, fmt.Errorf("script not found: %w", err)
		}
	} else {
		// Use inline script content
		script = &scripting.Script{
			Name:    "_inline_",
			Content: scriptContent,
		}
	}

	a.audit.Log("run_script", agentID, "user", fmt.Sprintf("Script: %s", script.Name), audit.StatusSuccess)

	// Get available vars from the agent info
	vars := map[string]string{
		"AGENT_ID": agentID,
		"DATE":     time.Now().Format(time.RFC3339),
	}
	if agent := a.clientMgr.GetAgent(agentID); agent != nil {
		vars["HOSTNAME"] = agent.Name
		vars["OS"] = agent.OS
		vars["ARCH"] = agent.Arch
	}

	agentIDForExec := agentID // capture for closure

	// Execute the script one command at a time
	executor := scripting.CommandExecutor(func(command string) (*scripting.ExecutionResult, error) {
		result, err := a.requestCommandResult(agentIDForExec, command, 60*time.Second) // scripts get longer timeout
		if err != nil {
			return &scripting.ExecutionResult{
				Command:  command,
				Stderr:   err.Error(),
				ExitCode: -1,
			}, err
		}
		return &scripting.ExecutionResult{
			Command:  command,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
			ExitCode: result.ExitCode,
			Duration: result.Duration,
		}, nil
	})

	scriptResult, err := a.scripts.Execute(script, vars, executor)
	if err != nil {
		a.audit.Log("run_script", agentID, "user", fmt.Sprintf("Script execution failed: %v", err), audit.StatusError)
		return nil, fmt.Errorf("script execution failed: %w", err)
	}

	// Build a combined result from all lines
	combinedStdout := ""
	combinedStderr := ""
	for _, r := range scriptResult.Results {
		if r.Stdout != "" {
			combinedStdout += fmt.Sprintf("# Line %d: %s\n%s\n", r.LineNumber, r.Command, r.Stdout)
		}
		if r.Stderr != "" {
			combinedStderr += fmt.Sprintf("# Line %d: %s\n%s\n", r.LineNumber, r.Command, r.Stderr)
		}
	}

	finalResult := &protocol.CommandResultPayload{
		Stdout:   combinedStdout,
		Stderr:   combinedStderr,
		ExitCode: scriptResult.FailedCount,
	}

	return finalResult, nil
}

// --- Binding: GetAuditLogs ---

// GetAuditLogs returns the most recent audit log entries.
func (a *App) GetAuditLogs(limit int) []audit.Entry {
	return a.audit.GetRecent(limit)
}

// --- Binding: GetScripts ---

// GetScripts returns all saved scripts.
func (a *App) GetScripts() []scripting.Script {
	return a.scripts.List()
}

// --- Binding: SaveScript ---

// SaveScript saves a script to the engine.
func (a *App) SaveScript(name string, content string) error {
	if name == "" {
		return fmt.Errorf("script name cannot be empty")
	}
	if content == "" {
		return fmt.Errorf("script content cannot be empty")
	}

	if err := a.scripts.Save(name, content); err != nil {
		a.audit.Log("save_script", "", "user", fmt.Sprintf("Failed to save script '%s': %v", name, err), audit.StatusError)
		return fmt.Errorf("failed to save script: %w", err)
	}

	a.audit.Log("save_script", "", "user", fmt.Sprintf("Saved script: %s (%d chars)", name, len(content)), audit.StatusSuccess)
	return nil
}

// --- Internal helpers ---

// requestCommandResult sends a command execution request and waits for the result.
func (a *App) requestCommandResult(agentID string, command string, timeout time.Duration) (*protocol.CommandResultPayload, error) {
	payload := protocol.ExecCommandPayload{
		Command: command,
		Timeout: int(timeout.Seconds()),
	}

	var result protocol.CommandResultPayload
	err := a.clientMgr.SendAndParse(agentID, protocol.MsgExecCommand, payload, &result, timeout)
	if err != nil {
		return nil, err
	}

	return &result, nil
}

// requestSystemInfo sends a system info request and waits for the result.
func (a *App) requestSystemInfo(agentID string) (*protocol.SystemInfoPayload, error) {
	var result protocol.SystemInfoPayload
	err := a.clientMgr.SendAndParse(agentID, protocol.MsgSystemInfo, struct{}{}, &result, defaultTimeout)
	if err != nil {
		return nil, err
	}

	return &result, nil
}
