package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

func startTestWebSocketServer(t *testing.T, token string, readTimeout time.Duration) (*Server, *websocket.Conn) {
	t.Helper()

	s := NewServer("", "", "", token)
	s.readTimeout = readTimeout
	httpServer := httptest.NewServer(httpHandler(s))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial test WebSocket: %v", err)
	}
	t.Cleanup(func() {
		_ = conn.Close()
		s.mu.RLock()
		clients := make([]*Client, 0, len(s.clients))
		for c := range s.clients {
			clients = append(clients, c)
		}
		s.mu.RUnlock()
		for _, c := range clients {
			c.close()
		}
	})

	return s, conn
}

func httpHandler(s *Server) http.Handler {
	return http.HandlerFunc(s.handleWebSocket)
}

func readTestMessage(t *testing.T, conn *websocket.Conn) protocol.Message {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	var msg protocol.Message
	if err := conn.ReadJSON(&msg); err != nil {
		t.Fatalf("read WebSocket message: %v", err)
	}
	return msg
}

func TestAuthenticationRequiresAndAcceptsTheConfiguredToken(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "correct-token", ReadTimeout)

	greeting := readTestMessage(t, conn)
	if greeting.Type != protocol.MsgAuthRequired {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAuthRequired)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "wrong-auth",
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "wrong-token"},
	}); err != nil {
		t.Fatalf("send invalid auth: %v", err)
	}
	invalid := readTestMessage(t, conn)
	if invalid.Type != protocol.MsgError || invalid.Error != "invalid authentication token" {
		t.Fatalf("invalid auth response = %#v", invalid)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "correct-auth",
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "correct-token"},
	}); err != nil {
		t.Fatalf("send valid auth: %v", err)
	}
	accepted := readTestMessage(t, conn)
	if accepted.Type != protocol.MsgAuthOk {
		t.Fatalf("valid auth response = %q, want %q", accepted.Type, protocol.MsgAuthOk)
	}
}

func TestNonEmptyOriginIsRejected(t *testing.T) {
	s := NewServer("", "", "", "")
	httpServer := httptest.NewServer(httpHandler(s))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	dialer := websocket.Dialer{HandshakeTimeout: time.Second}
	conn, response, err := dialer.Dial(wsURL, http.Header{"Origin": []string{"https://browser.example"}})
	if err == nil {
		_ = conn.Close()
		t.Fatal("WebSocket connection with a non-empty Origin was accepted")
	}
	if response == nil || response.StatusCode != http.StatusForbidden {
		t.Fatalf("Origin rejection response = %#v, err = %v; want HTTP 403", response, err)
	}
}

func TestKeepAliveRefreshesReadDeadline(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "correct-token", 400*time.Millisecond)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAuthRequired {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAuthRequired)
	}

	for i := 0; i < 3; i++ {
		time.Sleep(150 * time.Millisecond)
		if err := conn.WriteJSON(protocol.Message{Type: protocol.MsgKeepAlive}); err != nil {
			t.Fatalf("send keep_alive %d: %v", i, err)
		}
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "after-heartbeat",
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "correct-token"},
	}); err != nil {
		t.Fatalf("send auth after heartbeat: %v", err)
	}
	if response := readTestMessage(t, conn); response.Type != protocol.MsgAuthOk {
		t.Fatalf("response after heartbeat = %q, want %q", response.Type, protocol.MsgAuthOk)
	}
}

func TestUnregisterDoesNotCloseTheSendChannel(t *testing.T) {
	s := NewServer("", "", "", "")
	c := &Client{
		server: s,
		send:   make(chan []byte, 1),
		done:   make(chan struct{}),
		id:     "test-client",
	}
	if !s.register(c) {
		t.Fatal("test client was unexpectedly rejected")
	}

	s.unregister(c)

	panicCh := make(chan interface{}, 1)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer func() { panicCh <- recover() }()
		for i := 0; i < 100; i++ {
			c.sendMsg(protocol.Message{Type: protocol.MsgKeepAlive})
		}
	}()
	wg.Wait()

	if panicValue := <-panicCh; panicValue != nil {
		t.Fatalf("sending after unregister panicked: %v", panicValue)
	}
}

func TestRegisterRejectsClientsAboveLimit(t *testing.T) {
	s := NewServer("", "", "", "")
	clients := make([]*Client, 0, MaxClients)
	for i := 0; i < MaxClients; i++ {
		c := &Client{server: s, send: make(chan []byte, 1), done: make(chan struct{})}
		if !s.register(c) {
			t.Fatalf("client %d was rejected before reaching the limit", i)
		}
		clients = append(clients, c)
	}

	extra := &Client{server: s, send: make(chan []byte, 1), done: make(chan struct{})}
	if s.register(extra) {
		t.Fatal("client above MaxClients should be rejected")
	}

	for _, c := range clients {
		s.unregister(c)
	}
}
