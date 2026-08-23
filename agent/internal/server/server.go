package server

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
	"github.com/mediacode/lan-commander/agent/internal/system"
)

const (
	// ReadTimeout is the idle read timeout for WebSocket connections.
	ReadTimeout = 60 * time.Second
	// WriteTimeout is the write deadline for outgoing messages.
	WriteTimeout = 30 * time.Second
	// MaxMessageSize is the maximum allowed message size (10 MB).
	MaxMessageSize = 10 * 1024 * 1024
	// ReadBufferSize is the WebSocket read buffer (64 KB).
	ReadBufferSize = 64 * 1024
	// WriteBufferSize is the WebSocket write buffer (64 KB).
	WriteBufferSize = 64 * 1024
	// PushInterval is how often to push system updates (2 seconds).
	PushInterval = 2 * time.Second
	// MaxClients limits the number of active WebSocket connections.
	MaxClients = 128
)

// Server manages WebSocket connections and routes messages to handlers.
type Server struct {
	addr      string
	certFile  string
	keyFile   string
	authToken string
	monitor   *system.Monitor

	upgrader    websocket.Upgrader
	clients     map[*Client]bool
	mu          sync.RWMutex
	readTimeout time.Duration

	httpServer *http.Server
	done       chan struct{}
}

// Client represents a single WebSocket connection.
type Client struct {
	conn        *websocket.Conn
	server      *Server
	send        chan []byte
	done        chan struct{}
	id          string
	readTimeout time.Duration
	authed      atomic.Bool
	closeOnce   sync.Once
}

// NewServer creates a new WebSocket server.
func NewServer(addr, certFile, keyFile, authToken string) *Server {
	return &Server{
		addr:      addr,
		certFile:  certFile,
		keyFile:   keyFile,
		authToken: authToken,
		monitor:   system.NewMonitor(),
		upgrader: websocket.Upgrader{
			ReadBufferSize:  ReadBufferSize,
			WriteBufferSize: WriteBufferSize,
			CheckOrigin: func(r *http.Request) bool {
				return r.Header.Get("Origin") == ""
			},
		},
		clients:     make(map[*Client]bool),
		done:        make(chan struct{}),
		readTimeout: ReadTimeout,
	}
}

// Start begins listening for WebSocket connections.
// Blocks until the server stops or an error occurs.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleWebSocket)

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	listener, err := s.createListener()
	if err != nil {
		return fmt.Errorf("cannot listen on %s: %w", s.addr, err)
	}

	s.httpServer = &http.Server{
		Handler:      mux,
		ReadTimeout:  ReadTimeout + 10*time.Second,
		WriteTimeout: WriteTimeout + 10*time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if s.certFile != "" && s.keyFile != "" {
			log.Printf("[server] TLS listening on %s", s.addr)
			errCh <- s.httpServer.ServeTLS(listener, s.certFile, s.keyFile)
		} else {
			log.Printf("[server] Listening on ws://%s", s.addr)
			errCh <- s.httpServer.Serve(listener)
		}
	}()

	select {
	case <-ctx.Done():
		return s.shutdown()
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
		return nil
	}
}

func (s *Server) createListener() (net.Listener, error) {
	if s.certFile != "" && s.keyFile != "" {
		cert, err := tls.LoadX509KeyPair(s.certFile, s.keyFile)
		if err != nil {
			return nil, fmt.Errorf("cannot load TLS cert/key: %w", err)
		}
		tlsCfg := &tls.Config{
			Certificates: []tls.Certificate{cert},
			MinVersion:   tls.VersionTLS12,
		}
		return tls.Listen("tcp", s.addr, tlsCfg)
	}
	return net.Listen("tcp", s.addr)
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[server] Upgrade error: %v", err)
		return
	}

	client := &Client{
		conn:        conn,
		server:      s,
		send:        make(chan []byte, 64),
		done:        make(chan struct{}),
		id:          uuid.New().String(),
		readTimeout: s.readTimeout,
	}

	if !s.register(client) {
		log.Printf("[server] Rejecting client %s: maximum of %d active clients reached", client.id, MaxClients)
		client.close()
		return
	}

	// If auth is required, send auth_required immediately
	if s.authToken != "" {
		client.sendMsg(protocol.Message{
			Type:      protocol.MsgAuthRequired,
			Timestamp: time.Now(),
			Payload:   map[string]string{"message": "authentication required"},
		})
	} else {
		client.authed.Store(true)
		client.sendAgentInfo()
	}

	go client.writePump()
	go client.readPump()
}

func (s *Server) register(c *Client) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.clients) >= MaxClients {
		return false
	}
	s.clients[c] = true
	log.Printf("[server] Client connected: %s (%d active)", c.id, len(s.clients))
	return true
}

func (s *Server) unregister(c *Client) {
	s.mu.Lock()
	_, ok := s.clients[c]
	if ok {
		delete(s.clients, c)
	}
	active := len(s.clients)
	s.mu.Unlock()

	if ok {
		c.close()
		log.Printf("[server] Client disconnected: %s (%d active)", c.id, active)
	}
}

func (s *Server) shutdown() error {
	log.Println("[server] Shutting down...")

	s.mu.RLock()
	clients := make([]*Client, 0, len(s.clients))
	for c := range s.clients {
		clients = append(clients, c)
	}
	s.mu.RUnlock()

	for _, c := range clients {
		c.close()
	}

	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(ctx)
	}
	close(s.done)
	return nil
}

// readPump reads messages from the WebSocket connection.
func (c *Client) readPump() {
	defer func() {
		c.server.unregister(c)
		c.conn.Close()
	}()

	c.conn.SetReadLimit(MaxMessageSize)
	c.refreshReadDeadline()
	c.conn.SetPongHandler(func(string) error {
		return c.refreshReadDeadline()
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Printf("[client %s] Read error: %v", c.id, err)
			}
			break
		}

		var msg protocol.Message
		if err := json.Unmarshal(message, &msg); err != nil {
			log.Printf("[client %s] Invalid message: %v", c.id, err)
			continue
		}
		if err := c.refreshReadDeadline(); err != nil {
			log.Printf("[client %s] Cannot refresh read deadline: %v", c.id, err)
			break
		}

		if msg.ID == "" {
			msg.ID = uuid.New().String()
		}
		if msg.Timestamp.IsZero() {
			msg.Timestamp = time.Now()
		}

		c.handleMessage(msg)
	}
}

// writePump sends messages to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(PushInterval)
	defer func() {
		ticker.Stop()
		c.close()
	}()

	for {
		select {
		case <-c.done:
			return
		case message := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(WriteTimeout))
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				log.Printf("[client %s] Write error: %v", c.id, err)
				return
			}

		case <-ticker.C:
			if c.authed.Load() {
				c.pushSystemUpdate()
			}
		}
	}
}

// pushSystemUpdate sends a system update to the client.
func (c *Client) pushSystemUpdate() {
	info := c.server.monitor.GetSystemInfo()
	c.sendMsg(protocol.Message{
		Type:      protocol.MsgSystemUpdate,
		Timestamp: time.Now(),
		Payload:   info,
	})
}

// sendAgentInfo sends the AgentInfo payload to the client.
func (c *Client) sendAgentInfo() {
	info := c.server.monitor.GetSystemInfo()
	c.sendMsg(protocol.Message{
		Type:      protocol.MsgAgentInfo,
		Timestamp: time.Now(),
		Payload: protocol.AgentInfoPayload{
			Hostname:      info.Hostname,
			OS:            info.OS,
			Arch:          info.Arch,
			AgentVersion:  info.AgentVersion,
			Port:          0, // filled by caller if needed
			Authenticated: c.authed.Load(),
		},
	})
}

// sendMsg marshals and queues a message for sending.
func (c *Client) sendMsg(msg protocol.Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[client %s] Marshal error: %v", c.id, err)
		return
	}
	select {
	case <-c.done:
		return
	case c.send <- data:
	default:
		log.Printf("[client %s] Send buffer full, dropping message", c.id)
	}
}

// sendError sends an error message for a given request ID.
func (c *Client) sendError(requestID string, errMsg string) {
	c.sendMsg(protocol.Message{
		ID:        requestID,
		Type:      protocol.MsgError,
		Timestamp: time.Now(),
		Error:     errMsg,
	})
}

// sendResponse sends a success response for a given request.
func (c *Client) sendResponse(requestID string, respType string, payload interface{}) {
	c.sendMsg(protocol.Message{
		ID:        requestID,
		Type:      respType,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		close(c.done)
		if c.conn != nil {
			_ = c.conn.Close()
		}
	})
}

// sendJSON is a convenience for sending raw JSON bytes.
func (c *Client) sendJSON(data []byte) {
	select {
	case <-c.done:
		return
	case c.send <- data:
	default:
	}
}

func (c *Client) refreshReadDeadline() error {
	timeout := c.readTimeout
	if timeout <= 0 {
		timeout = ReadTimeout
	}
	return c.conn.SetReadDeadline(time.Now().Add(timeout))
}
