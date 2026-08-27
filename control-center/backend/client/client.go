package client

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
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
	maxMessageSize      = 10 * 1024 * 1024
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

// ConnectOptions controls one agent WebSocket connection and is retained for
// every reconnect of that logical agent.
type ConnectOptions struct {
	AuthToken  string
	TLS        bool
	UseTLS     bool
	CAFile     string
	ServerName string
}

func (o ConnectOptions) tlsEnabled() bool {
	return o.TLS || o.UseTLS
}

// connectionGeneration owns one WebSocket lifecycle. A new socket gets a new
// generation, including a new done channel, so a reconnect can never close a
// channel already closed by the previous read pump.
type connectionGeneration struct {
	conn      *websocket.Conn
	writeMu   sync.Mutex
	done      chan struct{}
	doneOnce  sync.Once
	closeOnce sync.Once
}

func newConnectionGeneration(conn *websocket.Conn) *connectionGeneration {
	return &connectionGeneration{
		conn: conn,
		done: make(chan struct{}),
	}
}

func (g *connectionGeneration) finish() {
	if g == nil {
		return
	}
	g.doneOnce.Do(func() { close(g.done) })
}

func (g *connectionGeneration) closeConn() {
	if g == nil || g.conn == nil {
		return
	}
	g.closeOnce.Do(func() { _ = g.conn.Close() })
}

// AgentConnection holds the stable agent identity and the current socket
// generation. State is protected independently from Manager.mu so no manager
// lock is held while doing network I/O.
type AgentConnection struct {
	info       *AgentInfo
	options    ConnectOptions
	stateMu    sync.RWMutex
	generation *connectionGeneration
	closed     bool
	cancel     context.CancelFunc
	cancelCtx  context.Context
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
	agents         map[string]*AgentConnection // agentId -> connection
	mu             sync.RWMutex
	onMessage      OnAgentMessage
	globalCtx      context.Context
	globalCancel   context.CancelFunc
	pending        map[string]*pendingRequest // msgID -> pendingRequest
	pendingMu      sync.RWMutex
	reconnectDelay time.Duration
}

// NewManager creates a new agent connection manager.
func NewManager(onMessage OnAgentMessage) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		agents:         make(map[string]*AgentConnection),
		onMessage:      onMessage,
		globalCtx:      ctx,
		globalCancel:   cancel,
		pending:        make(map[string]*pendingRequest),
		reconnectDelay: reconnectDelay,
	}
}

func cloneSystemInfo(info *protocol.SystemInfoPayload) *protocol.SystemInfoPayload {
	if info == nil {
		return nil
	}
	clone := *info
	if info.Disks != nil {
		clone.Disks = append([]protocol.DiskInfo(nil), info.Disks...)
	}
	return &clone
}

func cloneAgentInfo(info *AgentInfo) AgentInfo {
	if info == nil {
		return AgentInfo{}
	}
	clone := *info
	clone.SystemInfo = cloneSystemInfo(info.SystemInfo)
	return clone
}

func (ac *AgentConnection) snapshot() AgentInfo {
	ac.stateMu.RLock()
	defer ac.stateMu.RUnlock()
	return cloneAgentInfo(ac.info)
}

func (ac *AgentConnection) currentGeneration() (*connectionGeneration, bool) {
	ac.stateMu.RLock()
	defer ac.stateMu.RUnlock()
	if ac.generation == nil || ac.closed || ac.info == nil || !ac.info.Connected {
		return nil, false
	}
	return ac.generation, true
}

func (ac *AgentConnection) id() string {
	ac.stateMu.RLock()
	defer ac.stateMu.RUnlock()
	if ac.info == nil {
		return ""
	}
	return ac.info.ID
}

func (ac *AgentConnection) connectionDetails() (id, host string, port int, options ConnectOptions) {
	ac.stateMu.RLock()
	defer ac.stateMu.RUnlock()
	if ac.info == nil {
		return "", "", 0, ConnectOptions{}
	}
	return ac.info.ID, ac.info.Host, ac.info.Port, ac.options
}

func (ac *AgentConnection) installGeneration(generation *connectionGeneration) (*connectionGeneration, bool) {
	ac.stateMu.Lock()
	defer ac.stateMu.Unlock()

	if ac.closed || (ac.cancelCtx != nil && ac.cancelCtx.Err() != nil) || ac.info == nil {
		return nil, false
	}

	old := ac.generation
	ac.generation = generation
	ac.info.Connected = true
	ac.info.LastSeen = time.Now()
	return old, true
}

func (ac *AgentConnection) markDisconnected(generation *connectionGeneration) {
	ac.stateMu.Lock()
	defer ac.stateMu.Unlock()
	if ac.generation == generation && ac.info != nil {
		ac.info.Connected = false
	}
}

func (ac *AgentConnection) updateLastSeen(generation *connectionGeneration) {
	ac.stateMu.Lock()
	defer ac.stateMu.Unlock()
	if ac.generation == generation && ac.info != nil {
		ac.info.LastSeen = time.Now()
	}
}

func (ac *AgentConnection) updateAgentInfo(generation *connectionGeneration, info protocol.AgentInfoPayload) {
	ac.stateMu.Lock()
	defer ac.stateMu.Unlock()
	if ac.generation != generation || ac.info == nil || ac.closed {
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

func (ac *AgentConnection) updateSystemInfo(generation *connectionGeneration, sysInfo protocol.SystemInfoPayload) {
	ac.stateMu.Lock()
	defer ac.stateMu.Unlock()
	if ac.generation != generation || ac.info == nil || ac.closed {
		return
	}
	ac.info.SystemInfo = cloneSystemInfo(&sysInfo)
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

func (ac *AgentConnection) shouldReconnect(generation *connectionGeneration) bool {
	ac.stateMu.RLock()
	defer ac.stateMu.RUnlock()
	return ac.generation == generation && !ac.closed && ac.info != nil &&
		(ac.cancelCtx == nil || ac.cancelCtx.Err() == nil)
}

func (ac *AgentConnection) stop() *connectionGeneration {
	ac.stateMu.Lock()
	ac.closed = true
	if ac.info != nil {
		ac.info.Connected = false
	}
	generation := ac.generation
	ac.stateMu.Unlock()

	if ac.cancel != nil {
		ac.cancel()
	}
	return generation
}

// Connect establishes a plain WebSocket connection to an agent at host:port.
// It remains as the compatibility wrapper for existing callers.
func (m *Manager) Connect(host string, port int, authToken string) (string, error) {
	return m.ConnectWithOptions(host, port, ConnectOptions{AuthToken: authToken})
}

// ConnectWithOptions establishes a WebSocket connection using the supplied
// transport and authentication options. TLS uses system roots plus CAFile and
// always performs normal hostname verification.
func (m *Manager) ConnectWithOptions(host string, port int, options ConnectOptions) (string, error) {
	// Check if already connected to this host:port
	m.mu.RLock()
	for _, ac := range m.agents {
		info := ac.snapshot()
		if info.Host == host && info.Port == port && info.Connected {
			m.mu.RUnlock()
			return info.ID, nil
		}
	}
	m.mu.RUnlock()

	// Generate a new agent ID
	agentID := uuid.New().String()

	// Build WebSocket URL
	u := websocketURL(host, port, options)
	dialer, err := dialerForHost(host, options)
	if err != nil {
		return "", fmt.Errorf("failed to configure connection to agent at %s:%d: %w", host, port, err)
	}

	// Dial with timeout
	ctx, cancel := context.WithTimeout(m.globalCtx, handshakeTimeout)
	defer cancel()

	conn, _, err := dialer.DialContext(ctx, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to connect to agent at %s:%d: %w", host, port, err)
	}

	// The authentication handshake must complete before the read pump starts:
	// gorilla/websocket forbids concurrent readers, and the agent sends
	// auth_required as soon as the socket opens. Doing this afterwards raced
	// the pump for the agent's reply and failed nondeterministically.
	if err := handshake(conn, options.AuthToken); err != nil {
		conn.Close()
		return "", fmt.Errorf("agent at %s:%d: %w", host, port, err)
	}

	info := &AgentInfo{
		ID:        agentID,
		Host:      host,
		Port:      port,
		AuthToken: options.AuthToken,
		Secure:    options.tlsEnabled(),
		Connected: true,
		LastSeen:  time.Now(),
	}

	agentCtx, agentCancel := context.WithCancel(m.globalCtx)
	ac := &AgentConnection{
		info:       info,
		options:    options,
		generation: newConnectionGeneration(conn),
		cancel:     agentCancel,
		cancelCtx:  agentCtx,
	}

	m.mu.Lock()
	// A concurrent Connect may have installed the same endpoint while this
	// dial/handshake was in progress. Keep one logical agent per endpoint.
	for _, existing := range m.agents {
		existingInfo := existing.snapshot()
		if existingInfo.Host == host && existingInfo.Port == port && existingInfo.Connected {
			m.mu.Unlock()
			conn.Close()
			return existingInfo.ID, nil
		}
	}
	if m.globalCtx.Err() != nil {
		m.mu.Unlock()
		conn.Close()
		return "", fmt.Errorf("manager is shutting down")
	}
	m.agents[agentID] = ac
	m.mu.Unlock()

	// Start read pump
	go m.readPump(ac, ac.generation)

	// Start heartbeat
	go m.heartbeat(ac, ac.generation)

	log.Printf("client: connected to agent %s at %s:%d", agentID, host, port)
	return agentID, nil
}

func websocketURL(host string, port int, options ConnectOptions) url.URL {
	scheme := "ws"
	if options.tlsEnabled() {
		scheme = "wss"
	}
	return url.URL{
		Scheme: scheme,
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/ws",
	}
}

func dialerForHost(host string, options ConnectOptions) (websocket.Dialer, error) {
	dialer := websocket.Dialer{HandshakeTimeout: handshakeTimeout}
	if !options.tlsEnabled() {
		if options.CAFile != "" || options.ServerName != "" {
			return dialer, fmt.Errorf("CAFile and ServerName require TLS")
		}
		return dialer, nil
	}

	rootCAs, err := x509.SystemCertPool()
	if err != nil {
		return dialer, fmt.Errorf("load system certificate pool: %w", err)
	}
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}
	if options.CAFile != "" {
		caPEM, err := os.ReadFile(options.CAFile)
		if err != nil {
			return dialer, fmt.Errorf("read CA file %q: %w", options.CAFile, err)
		}
		if !rootCAs.AppendCertsFromPEM(caPEM) {
			return dialer, fmt.Errorf("CA file %q contains no certificates", options.CAFile)
		}
	}

	serverName := options.ServerName
	if serverName == "" {
		serverName = host
	}
	dialer.TLSClientConfig = &tls.Config{
		MinVersion: tls.VersionTLS12,
		RootCAs:    rootCAs,
		ServerName: serverName,
	}
	return dialer, nil
}

// ErrAuthRequired reports that the agent demands a token the caller did not
// supply. Callers use it to distinguish "wrong credentials" from "unreachable".
var ErrAuthRequired = errors.New("agent requires an authentication token")

// handshake completes the agent's greeting exchange on a freshly dialed
// connection, before any concurrent reader exists. An agent configured with a
// token greets with auth_required; one without sends agent_info directly.
func handshake(conn *websocket.Conn, authToken string) error {
	// Apply the limit before reading the greeting as well as for all subsequent
	// frames. This prevents an oversized first frame from bypassing the limit.
	conn.SetReadLimit(maxMessageSize)
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

	generation := ac.stop()
	m.closeGeneration(generation, "bye")

	// Wait for read pump to finish (with timeout)
	waitForGeneration(generation, 2*time.Second)

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
	generation, connected := ac.currentGeneration()
	if !connected {
		return fmt.Errorf("agent %s is not connected", agentID)
	}

	return m.writeJSON(generation, msg)
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
	info := ac.snapshot()
	return &info
}

// ListAgents returns all known agents.
func (m *Manager) ListAgents() []AgentInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make([]AgentInfo, 0, len(m.agents))
	for _, ac := range m.agents {
		result = append(result, ac.snapshot())
	}
	return result
}

// ConnectedCount returns the number of currently connected agents.
func (m *Manager) ConnectedCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, ac := range m.agents {
		if ac.snapshot().Connected {
			count++
		}
	}
	return count
}

// CloseAll disconnects all agents gracefully.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	connections := make([]*AgentConnection, 0, len(m.agents))
	for id, ac := range m.agents {
		delete(m.agents, id)
		connections = append(connections, ac)
	}
	m.mu.Unlock()

	// Cancel first so reconnect attempts and pending requests stop promptly.
	m.globalCancel()

	// Clean up all pending requests
	m.pendingMu.Lock()
	for id, pr := range m.pending {
		pr.timer.Stop()
		delete(m.pending, id)
	}
	m.pendingMu.Unlock()

	for _, ac := range connections {
		generation := ac.stop()
		m.closeGeneration(generation, "shutdown")
		waitForGeneration(generation, 2*time.Second)
	}
}

// --- internal methods ---

func (m *Manager) writeJSON(generation *connectionGeneration, msg interface{}) error {
	if generation == nil || generation.conn == nil {
		return fmt.Errorf("connection is not available")
	}

	generation.writeMu.Lock()
	defer generation.writeMu.Unlock()

	if err := generation.conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	return generation.conn.WriteJSON(msg)
}

func (m *Manager) closeGeneration(generation *connectionGeneration, reason string) {
	if generation == nil || generation.conn == nil {
		return
	}

	generation.writeMu.Lock()
	_ = generation.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	_ = generation.conn.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason))
	generation.writeMu.Unlock()
	generation.closeConn()
}

func waitForGeneration(generation *connectionGeneration, timeout time.Duration) {
	if generation == nil {
		return
	}

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-generation.done:
	case <-timer.C:
	}
}

func (m *Manager) readPump(ac *AgentConnection, generation *connectionGeneration) {
	defer func() {
		ac.markDisconnected(generation)
		generation.closeConn()
		generation.finish()
	}()

	if generation == nil || generation.conn == nil {
		return
	}
	generation.conn.SetReadLimit(maxMessageSize)

	for {
		select {
		case <-ac.cancelCtx.Done():
			return
		case <-m.globalCtx.Done():
			return
		default:
		}

		generation.conn.SetReadDeadline(time.Now().Add(readTimeout))
		_, messageBytes, err := generation.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
				log.Printf("client: read error from agent %s: %v", ac.id(), err)
			}
			// Attempt reconnection
			if ac.shouldReconnect(generation) {
				go m.reconnect(ac, generation)
			}
			return
		}

		ac.updateLastSeen(generation)

		var msg protocol.Message
		if err := json.Unmarshal(messageBytes, &msg); err != nil {
			log.Printf("client: failed to parse message from agent %s: %v", ac.id(), err)
			continue
		}

		// Handle special message types internally
		switch msg.Type {
		case protocol.MsgAgentInfo:
			m.handleAgentInfo(ac, generation, messageBytes)
		case protocol.MsgSystemUpdate:
			m.handleSystemUpdate(ac, generation, messageBytes)
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
			m.onMessage(ac.id(), &msg)
		}
	}
}

func (m *Manager) handleAgentInfo(ac *AgentConnection, generation *connectionGeneration, data []byte) {
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

	ac.updateAgentInfo(generation, info)
}

func (m *Manager) handleSystemUpdate(ac *AgentConnection, generation *connectionGeneration, data []byte) {
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

	ac.updateSystemInfo(generation, sysInfo)
}

func (m *Manager) heartbeat(ac *AgentConnection, generation *connectionGeneration) {
	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			msg := &protocol.Message{
				Type:      protocol.MsgKeepAlive,
				Timestamp: time.Now(),
			}
			if err := m.writeJSON(generation, msg); err != nil {
				log.Printf("client: heartbeat failed for agent %s: %v", ac.id(), err)
				return
			}
		case <-generation.done:
			return
		case <-ac.cancelCtx.Done():
			return
		case <-m.globalCtx.Done():
			return
		}
	}
}

func (m *Manager) reconnect(ac *AgentConnection, failedGeneration *connectionGeneration) {
	// Don't reconnect if the agent was deliberately disconnected or another
	// generation has already taken over.
	if !ac.shouldReconnect(failedGeneration) {
		return
	}

	agentID, host, port, options := ac.connectionDetails()
	cancelCtx := ac.cancelCtx
	if cancelCtx == nil {
		cancelCtx = m.globalCtx
	}

	for i := 0; i < maxReconnectRetries; i++ {
		timer := time.NewTimer(m.reconnectDelay)
		select {
		case <-timer.C:
		case <-cancelCtx.Done():
			timer.Stop()
			return
		case <-m.globalCtx.Done():
			timer.Stop()
			return
		}

		if !ac.shouldReconnect(failedGeneration) {
			return
		}

		log.Printf("client: reconnecting to agent %s at %s:%d (attempt %d/%d)",
			agentID, host, port, i+1, maxReconnectRetries)

		u := websocketURL(host, port, options)
		dialer, err := dialerForHost(host, options)
		if err != nil {
			log.Printf("client: reconnect options for agent %s are invalid: %v", agentID, err)
			return
		}

		ctx, cancel := context.WithTimeout(cancelCtx, handshakeTimeout)
		conn, _, err := dialer.DialContext(ctx, u.String(), nil)
		cancel()
		if err != nil {
			log.Printf("client: reconnect attempt %d failed: %v", i+1, err)
			continue
		}

		// Re-authenticate before installing the connection, for the same
		// reason as in Connect: no concurrent reader may exist yet.
		if err := handshake(conn, options.AuthToken); err != nil {
			log.Printf("client: re-authentication failed for agent %s: %v", agentID, err)
			conn.Close()
			continue
		}

		generation := newConnectionGeneration(conn)
		oldGeneration, installed := ac.installGeneration(generation)
		if !installed {
			generation.closeConn()
			return
		}
		if oldGeneration != nil {
			oldGeneration.closeConn()
		}

		log.Printf("client: reconnected to agent %s", agentID)

		// Restart read pump and heartbeat for this generation only.
		go m.readPump(ac, generation)
		go m.heartbeat(ac, generation)
		return
	}

	log.Printf("client: giving up reconnection to agent %s after %d attempts", agentID, maxReconnectRetries)
	ac.markDisconnected(failedGeneration)
}
