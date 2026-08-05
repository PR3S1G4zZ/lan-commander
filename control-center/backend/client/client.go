package client

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"sync"
	"time"

	"control-center/backend/protocol"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

const (
	writeTimeout        = 10 * time.Second
	readTimeout         = 60 * time.Second
	handshakeTimeout    = 10 * time.Second
	heartbeatInterval   = 30 * time.Second
	maxReconnectRetries = 5
	reconnectDelay      = 5 * time.Second
)

// AgentInfo holds the runtime state of a connected/known agent.
type AgentInfo struct {
	ID         string                      `json:"id"`
	Host       string                      `json:"host"`
	Port       int                         `json:"port"`
	AuthToken  string                      `json:"auth_token,omitempty"`
	Secure     bool                        `json:"secure"`
	Name       string                      `json:"name"`
	OS         string                      `json:"os"`
	Arch       string                      `json:"arch"`
	Connected  bool                        `json:"connected"`
	LastSeen   time.Time                   `json:"last_seen"`
	SystemInfo *protocol.SystemInfoPayload `json:"system_info,omitempty"`
}

// AgentConnection holds the WebSocket connection and associated state for an agent.
type AgentConnection struct {
	info      *AgentInfo
	conn      *websocket.Conn
	writeMu   sync.Mutex
	cancel    context.CancelFunc
	cancelCtx context.Context
	done      chan struct{}
}

// OnAgentMessage is a callback invoked when a message is received from an agent.
type OnAgentMessage func(agentID string, msg *protocol.Message)

// pendingRequest tracks an outgoing request that expects a response.
type pendingRequest struct {
	ch      chan *protocol.Message
	timeout time.Duration
	timer   *time.Timer
}

// Manager manages WebSocket connections to multiple agents.
type Manager struct {
	agents       map[string]*AgentConnection // agentId -> connection
	mu           sync.RWMutex
	onMessage    OnAgentMessage
	globalCtx    context.Context
	globalCancel context.CancelFunc
	pending      map[string]*pendingRequest // msgID -> pendingRequest
	pendingMu    sync.RWMutex
}

// NewManager creates a new agent connection manager.
func NewManager(onMessage OnAgentMessage) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		agents:       make(map[string]*AgentConnection),
		onMessage:    onMessage,
		globalCtx:    ctx,
		globalCancel: cancel,
		pending:      make(map[string]*pendingRequest),
	}
}

func websocketScheme(secure bool) string {

	if secure {

		return "wss"

	}

	return "ws"
}

func newDialer(secure bool) websocket.Dialer {

	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}

	if secure {

		dialer.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12}

	}

	return dialer
}

// Connect establishes a WebSocket connection to an agent at host:port.
// If authToken is non-empty, it performs an authentication handshake.
func (m *Manager) Connect(host string, port int, authToken string, secure bool) (string, error) {
	// Check if already connected to this host:port
	m.mu.RLock()
	for _, ac := range m.agents {
		if ac.info.Host == host && ac.info.Port == port && ac.info.Secure == secure && ac.info.Connected {
			m.mu.RUnlock()
			return ac.info.ID, nil
		}
	}
	m.mu.RUnlock()

	// Generate a new agent ID
	agentID := uuid.New().String()

	// Build WebSocket URL
	u := url.URL{
		Scheme: websocketScheme(secure),
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/ws",
	}

	// Dial with timeout
	ctx, cancel := context.WithTimeout(m.globalCtx, handshakeTimeout)
	defer cancel()

	dialer := newDialer(secure)

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect to agent at %s:%d: %w", host, port, err)
	}

	// The authentication handshake must complete before the read pump starts:
	// gorilla/websocket forbids concurrent readers, and the agent sends
	// auth_required as soon as the socket opens. Doing this afterwards raced
	// the pump for the agent's reply and failed nondeterministically.
	if err := handshake(conn, authToken); err != nil {
		conn.Close()
		return "", fmt.Errorf("agent at %s:%d: %w", host, port, err)
	}

	info := &AgentInfo{
		ID:        agentID,
		Host:      host,
		Port:      port,
		AuthToken: authToken,
		Secure:    secure,
		Connected: true,
		LastSeen:  time.Now(),
	}

	agentCtx, agentCancel := context.WithCancel(m.globalCtx)
	ac := &AgentConnection{
		info:      info,
		conn:      conn,
		cancel:    agentCancel,
		cancelCtx: agentCtx,
		done:      make(chan struct{}),
	}

	m.mu.Lock()
	m.agents[agentID] = ac
	m.mu.Unlock()

	// Start read pump
	go m.readPump(ac)

	// Start heartbeat
	go m.heartbeat(ac)

	log.Printf("client: connected to agent %s at %s:%d", agentID, host, port)
	return agentID, nil
}

// ErrAuthRequired reports that the agent demands a token the caller did not
// supply. Callers use it to distinguish "wrong credentials" from "unreachable".
var ErrAuthRequired = errors.New("agent requires an authentication token")

// handshake completes the agent's greeting exchange on a freshly dialed
// connection, before any concurrent reader exists. An agent configured with a
// token greets with auth_required; one without sends agent_info directly.
func handshake(conn *websocket.Conn, authToken string) error {
	if err := conn.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return fmt.Errorf("cannot set handshake deadline: %w", err)
	}
	// Clear the deadline so the read pump is not affected by it.
	defer conn.SetReadDeadline(time.Time{})

	var greeting protocol.Message
	if err := conn.ReadJSON(&greeting); err != nil {
		return fmt.Errorf("no greeting received: %w", err)
	}

	switch greeting.Type {
	case protocol.MsgAgentInfo:
		// Agent accepts anonymous connections; nothing else to negotiate.
		if authToken != "" {
			log.Printf("client: agent accepts anonymous connections, token not used")
		}
		return nil
	case protocol.MsgAuthRequired:
		if authToken == "" {
			return ErrAuthRequired
		}
	default:
		return fmt.Errorf("unexpected greeting %q", greeting.Type)
	}

	authMsg := protocol.Message{
		Type:      protocol.MsgAuth,
		Payload:   protocol.AuthPayload{Token: authToken},
		Timestamp: time.Now(),
	}
	if err := conn.WriteJSON(authMsg); err != nil {
		return fmt.Errorf("cannot send credentials: %w", err)
	}

	for {
		var response protocol.Message
		if err := conn.ReadJSON(&response); err != nil {
			return fmt.Errorf("no response to credentials: %w", err)
		}

		switch response.Type {
		case protocol.MsgAuthOk:
			return nil
		case protocol.MsgError:
			detail := response.Error
			if detail == "" {
				detail = "token rejected"
			}
			return fmt.Errorf("authentication failed: %s", detail)
		case protocol.MsgAgentInfo:
			// The agent may send agent_info before auth_ok; keep reading.
			continue
		default:
			return fmt.Errorf("unexpected response %q to credentials", response.Type)
		}
	}
}

// Disconnect gracefully closes the connection to an agent.
func (m *Manager) Disconnect(agentID string) error {
	m.mu.Lock()
	ac, exists := m.agents[agentID]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("agent %s not found", agentID)
	}
	delete(m.agents, agentID)
	m.mu.Unlock()

	ac.cancel()

	// Graceful WebSocket close
	err := ac.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "bye"))
	if err != nil {
		// Ignore write errors on close
	}
	ac.conn.Close()

	// Wait for read pump to finish (with timeout)
	select {
	case <-ac.done:
	case <-time.After(2 * time.Second):
	}

	log.Printf("client: disconnected from agent %s", agentID)
	return nil
}

// SendMessage sends a protocol message to a specific agent.
func (m *Manager) SendMessage(agentID string, msg *protocol.Message) error {
	m.mu.RLock()
	ac, exists := m.agents[agentID]
	m.mu.RUnlock()

	if !exists {
		return fmt.Errorf("agent %s not found", agentID)
	}
	if !ac.info.Connected {
		return fmt.Errorf("agent %s is not connected", agentID)
	}

	return m.writeJSON(ac, msg)
}

// SendRequest sends a request message and waits for a response.
func (m *Manager) SendRequest(agentID string, msgType string, payload interface{}, timeout time.Duration) (*protocol.Message, error) {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}

	msgID := uuid.New().String()
	msg := &protocol.Message{
		ID:        msgID,
		Type:      msgType,
		Payload:   payload,
		Timestamp: time.Now(),
	}

	// Create pending request entry
	pr := &pendingRequest{
		ch:      make(chan *protocol.Message, 1),
		timeout: timeout,
		timer:   time.NewTimer(timeout),
	}

	m.pendingMu.Lock()
	m.pending[msgID] = pr
	m.pendingMu.Unlock()

	defer func() {
		m.pendingMu.Lock()
		delete(m.pending, msgID)
		pr.timer.Stop()
		m.pendingMu.Unlock()
	}()

	if err := m.SendMessage(agentID, msg); err != nil {
		return nil, fmt.Errorf("failed to send message: %w", err)
	}

	// Wait for response or timeout
	select {
	case response := <-pr.ch:
		if response.Error != "" {
			return response, fmt.Errorf("agent error: %s", response.Error)
		}
		return response, nil
	case <-pr.timer.C:
		return nil, fmt.Errorf("request timed out after %v", timeout)
	case <-m.globalCtx.Done():
		return nil, fmt.Errorf("manager is shutting down")
	}
}

// SendAndParse sends a request and parses the response payload into the provided target.
func (m *Manager) SendAndParse(agentID string, msgType string, payload interface{}, target interface{}, timeout time.Duration) error {
	response, err := m.SendRequest(agentID, msgType, payload, timeout)
	if err != nil {
		return err
	}

	payloadBytes, err := json.Marshal(response.Payload)
	if err != nil {
		return fmt.Errorf("failed to marshal response payload: %w", err)
	}

	if err := json.Unmarshal(payloadBytes, target); err != nil {
		return fmt.Errorf("failed to unmarshal response payload: %w", err)
	}

	return nil
}

// GetAgent returns the agent info for a given ID.
func (m *Manager) GetAgent(agentID string) *AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ac, exists := m.agents[agentID]
	if !exists {
		return nil
	}
	return ac.info
}

// ListAgents returns all known agents.
func (m *Manager) ListAgents() []AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AgentInfo, 0, len(m.agents))
	for _, ac := range m.agents {
		result = append(result, *ac.info)
	}
	return result
}

// ConnectedCount returns the number of currently connected agents.
func (m *Manager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, ac := range m.agents {
		if ac.info.Connected {
			count++
		}
	}
	return count
}

// CloseAll disconnects all agents gracefully.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, ac := range m.agents {
		delete(m.agents, id)
		ac.cancel()
		ac.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, "shutdown"))
		ac.conn.Close()
	}

	// Clean up all pending requests
	m.pendingMu.Lock()
	for id, pr := range m.pending {
		pr.timer.Stop()
		delete(m.pending, id)
	}
	m.pendingMu.Unlock()

	m.globalCancel()
}

// --- internal methods ---

func (m *Manager) writeJSON(ac *AgentConnection, msg interface{}) error {
	ac.writeMu.Lock()
	defer ac.writeMu.Unlock()

	ac.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	return ac.conn.WriteJSON(msg)
}

func (m *Manager) readPump(ac *AgentConnection) {
	defer close(ac.done)
	defer func() {
		ac.info.Connected = false
		ac.conn.Close()
	}()

	for {
		select {
		case <-ac.cancelCtx.Done():
			return
		case <-m.globalCtx.Done():
			return
		default:
		}

		ac.conn.SetReadDeadline(time.Now().Add(readTimeout))
		_, messageBytes, err := ac.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("client: read error from agent %s: %v", ac.info.ID, err)
			}
			// Attempt reconnection
			go m.reconnect(ac)
			return
		}

		ac.info.LastSeen = time.Now()

		var msg protocol.Message
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("client: failed to parse message from agent %s: %v", ac.info.ID, err)
			continue
		}

		// Handle special message types internally
		switch msg.Type {
		case protocol.MsgAgentInfo:
			m.handleAgentInfo(ac, messageBytes)
		case protocol.MsgSystemUpdate:
			m.handleSystemUpdate(ac, messageBytes)
		}

		// Check if this message matches a pending request (by message ID)
		if msg.ID != "" {
			m.pendingMu.RLock()
			pr, exists := m.pending[msg.ID]
			m.pendingMu.RUnlock()
			if exists {
				// Non-blocking send to avoid deadlock if channel is full
				select {
				case pr.ch <- &msg:
				default:
				}
				// Don't forward matched responses to the callback
				continue
			}
		}

		// Forward to callback
		if m.onMessage != nil {
			m.onMessage(ac.info.ID, &msg)
		}
	}
}

func (m *Manager) handleAgentInfo(ac *AgentConnection, data []byte) {
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		return
	}

	var info protocol.AgentInfoPayload
	if err := json.Unmarshal(payloadBytes, &info); err != nil {
		return
	}

	if info.Hostname != "" {
		ac.info.Name = info.Hostname
	}
	if info.OS != "" {
		ac.info.OS = info.OS
	}
	if info.Arch != "" {
		ac.info.Arch = info.Arch
	}
}

func (m *Manager) handleSystemUpdate(ac *AgentConnection, data []byte) {
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}

	payloadBytes, err := json.Marshal(msg.Payload)
	if err != nil {
		return
	}

	var sysInfo protocol.SystemInfoPayload
	if err := json.Unmarshal(payloadBytes, &sysInfo); err != nil {
		return
	}

	ac.info.SystemInfo = &sysInfo
	if sysInfo.Hostname != "" {
		ac.info.Name = sysInfo.Hostname
	}
	if sysInfo.OS != "" {
		ac.info.OS = sysInfo.OS
	}
	if sysInfo.Arch != "" {
		ac.info.Arch = sysInfo.Arch
	}
}

func (m *Manager) heartbeat(ac *AgentConnection) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			msg := &protocol.Message{
				Type:      protocol.MsgKeepAlive,
				Timestamp: time.Now(),
			}
			if err := m.writeJSON(ac, msg); err != nil {
				log.Printf("client: heartbeat failed for agent %s: %v", ac.info.ID, err)
				return
			}
		case <-ac.cancelCtx.Done():
			return
		case <-m.globalCtx.Done():
			return
		}
	}
}

func (m *Manager) reconnect(ac *AgentConnection) {
	// Don't reconnect if we were deliberately disconnected
	select {
	case <-ac.cancelCtx.Done():
		return
	default:
	}

	for i := 0; i < maxReconnectRetries; i++ {
		time.Sleep(reconnectDelay)

		select {
		case <-ac.cancelCtx.Done():
			return
		case <-m.globalCtx.Done():
			return
		default:
		}

		log.Printf("client: reconnecting to agent %s at %s:%d (attempt %d/%d)",
			ac.info.ID, ac.info.Host, ac.info.Port, i+1, maxReconnectRetries)

		u := url.URL{
			Scheme: websocketScheme(ac.info.Secure),
			Host:   fmt.Sprintf("%s:%d", ac.info.Host, ac.info.Port),
			Path:   "/ws",
		}

		dialer := newDialer(ac.info.Secure)

		conn, _, err := dialer.DialContext(m.globalCtx, u.String(), nil)
		if err != nil {
			log.Printf("client: reconnect attempt %d failed: %v", i+1, err)
			continue
		}

		// Re-authenticate before installing the connection, for the same
		// reason as in Connect: no concurrent reader may exist yet.
		if err := handshake(conn, ac.info.AuthToken); err != nil {
			log.Printf("client: re-authentication failed for agent %s: %v", ac.info.ID, err)
			conn.Close()
			continue
		}

		// Replace old connection
		oldConn := ac.conn
		ac.conn = conn
		ac.info.Connected = true
		ac.info.LastSeen = time.Now()
		oldConn.Close()

		log.Printf("client: reconnected to agent %s", ac.info.ID)

		// Restart read pump and heartbeat
		go m.readPump(ac)
		go m.heartbeat(ac)
		return
	}

	log.Printf("client: giving up reconnection to agent %s after %d attempts", ac.info.ID, maxReconnectRetries)
	ac.info.Connected = false
}
