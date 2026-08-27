package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

func TestAuthAttemptsAreLimitedPerConnection(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "correct-token", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAuthRequired {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAuthRequired)
	}

	const maxFailedAttempts = 5
	for attempt := 1; attempt < maxFailedAttempts; attempt++ {
		id := fmt.Sprintf("invalid-auth-%d", attempt)
		if err := conn.WriteJSON(protocol.Message{
			ID:      id,
			Type:    protocol.MsgAuth,
			Payload: protocol.AuthPayload{Token: "wrong-token"},
		}); err != nil {
			t.Fatalf("send invalid auth attempt %d: %v", attempt, err)
		}
		response := readTestMessage(t, conn)
		if response.ID != id || response.Type != protocol.MsgError || response.Error != "invalid authentication token" {
			t.Fatalf("invalid auth response %d = %#v", attempt, response)
		}
	}

	// The server may close the socket immediately after consuming this final frame.
	_ = conn.WriteJSON(protocol.Message{
		ID:      fmt.Sprintf("invalid-auth-%d", maxFailedAttempts),
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "wrong-token"},
	})

	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set close read deadline: %v", err)
	}
	for {
		if _, _, err := conn.ReadMessage(); err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				t.Fatalf("connection remained open after %d failed auth attempts", maxFailedAttempts)
			}
			return
		}
	}
}

func TestSuccessfulAuthenticationSendsAuthenticatedAgentInfo(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "correct-token", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAuthRequired {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAuthRequired)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "valid-auth",
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "correct-token", Username: "test-user"},
	}); err != nil {
		t.Fatalf("send valid auth: %v", err)
	}
	authOK := readTestMessage(t, conn)
	if authOK.ID != "valid-auth" || authOK.Type != protocol.MsgAuthOk {
		t.Fatalf("auth response = %#v, want auth_ok for valid-auth", authOK)
	}

	infoMessage := readTestMessage(t, conn)
	if infoMessage.Type != protocol.MsgAgentInfo {
		t.Fatalf("post-auth message type = %q, want %q", infoMessage.Type, protocol.MsgAgentInfo)
	}
	encoded, err := json.Marshal(infoMessage.Payload)
	if err != nil {
		t.Fatalf("marshal agent info payload: %v", err)
	}
	var info protocol.AgentInfoPayload
	if err := json.Unmarshal(encoded, &info); err != nil {
		t.Fatalf("decode agent info payload: %v", err)
	}
	if !info.Authenticated {
		t.Fatal("agent info reports Authenticated=false after successful authentication")
	}
}

func TestUnauthenticatedMessageIsRejectedBeforeHandlerDispatch(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "correct-token", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAuthRequired {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAuthRequired)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "pre-auth-system-info",
		Type:    protocol.MsgSystemInfo,
		Payload: nil,
	}); err != nil {
		t.Fatalf("send unauthenticated message: %v", err)
	}
	rejected := readTestMessage(t, conn)
	if rejected.ID != "pre-auth-system-info" || rejected.Type != protocol.MsgError || rejected.Error != "authentication required" {
		t.Fatalf("pre-auth response = %#v", rejected)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "auth-after-rejection",
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "correct-token"},
	}); err != nil {
		t.Fatalf("send auth after pre-auth rejection: %v", err)
	}
	if response := readTestMessage(t, conn); response.Type != protocol.MsgAuthOk {
		t.Fatalf("auth after pre-auth rejection = %q, want %q", response.Type, protocol.MsgAuthOk)
	}
	if info := readTestMessage(t, conn); info.Type != protocol.MsgAgentInfo {
		t.Fatalf("agent info after auth = %q, want %q", info.Type, protocol.MsgAgentInfo)
	}
}

func TestInvalidAuthPayloadIsRejectedWithoutDisconnecting(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "correct-token", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAuthRequired {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAuthRequired)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "malformed-auth",
		Type:    protocol.MsgAuth,
		Payload: "not-an-auth-object",
	}); err != nil {
		t.Fatalf("send malformed auth: %v", err)
	}
	malformed := readTestMessage(t, conn)
	if malformed.ID != "malformed-auth" || malformed.Type != protocol.MsgError || malformed.Error != "invalid auth payload format" {
		t.Fatalf("malformed auth response = %#v", malformed)
	}

	if err := conn.WriteJSON(protocol.Message{
		ID:      "valid-auth-after-malformed",
		Type:    protocol.MsgAuth,
		Payload: protocol.AuthPayload{Token: "correct-token"},
	}); err != nil {
		t.Fatalf("send valid auth after malformed auth: %v", err)
	}
	if response := readTestMessage(t, conn); response.Type != protocol.MsgAuthOk {
		t.Fatalf("auth after malformed payload = %q, want %q", response.Type, protocol.MsgAuthOk)
	}
}

func TestUnknownMessageReturnsCorrelatedErrorAfterAuthentication(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAgentInfo)
	}

	if err := conn.WriteJSON(protocol.Message{Type: "unsupported-message", Payload: nil}); err != nil {
		t.Fatalf("send unknown message: %v", err)
	}
	response := readTestMessage(t, conn)
	if response.ID == "" {
		t.Fatal("unknown-message error did not receive a generated request ID")
	}
	if response.Type != protocol.MsgError || response.Error != "unknown message type: unsupported-message" {
		t.Fatalf("unknown-message response = %#v", response)
	}
}

func TestMalformedHandlerPayloadsReturnStableErrors(t *testing.T) {
	_, conn := startTestWebSocketServer(t, "", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAgentInfo)
	}

	cases := []struct {
		name        string
		messageType string
		wantError   string
	}{
		{name: "exec", messageType: protocol.MsgExecCommand, wantError: "invalid exec payload format"},
		{name: "list_dir", messageType: protocol.MsgListDir, wantError: "invalid list_dir payload format"},
		{name: "get_file", messageType: protocol.MsgGetFile, wantError: "invalid get_file payload format"},
		{name: "send_file", messageType: protocol.MsgSendFile, wantError: "invalid send_file payload format"},
		{name: "cancel_file", messageType: protocol.MsgCancelFile, wantError: "invalid cancel_file payload format"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id := "malformed-" + tc.name
			if err := conn.WriteJSON(protocol.Message{
				ID:      id,
				Type:    tc.messageType,
				Payload: "not-an-object",
			}); err != nil {
				t.Fatalf("send malformed %s message: %v", tc.name, err)
			}
			response := readTestMessage(t, conn)
			if response.ID != id || response.Type != protocol.MsgError || response.Error != tc.wantError {
				t.Fatalf("malformed %s response = %#v", tc.name, response)
			}
		})
	}
}

func TestEmptyOriginIsAcceptedForNativeClients(t *testing.T) {
	s := NewServer("", "", "", "")
	httpServer := httptest.NewServer(httpHandler(s))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, response, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial WebSocket without Origin: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if response == nil || response.StatusCode != http.StatusSwitchingProtocols {
		t.Fatalf("empty Origin handshake response = %#v, want HTTP 101", response)
	}
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAgentInfo)
	}
}

func TestDisconnectUnregistersClientAndClosesDone(t *testing.T) {
	s, conn := startTestWebSocketServer(t, "", ReadTimeout)
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAgentInfo)
	}

	client := registeredClient(t, s)
	if err := conn.Close(); err != nil {
		t.Fatalf("close client WebSocket: %v", err)
	}
	select {
	case <-client.done:
	case <-time.After(2 * time.Second):
		t.Fatal("server did not close the client done channel after disconnect")
	}

	s.mu.RLock()
	_, stillRegistered := s.clients[client]
	s.mu.RUnlock()
	if stillRegistered {
		t.Fatal("disconnected client remained registered")
	}
}

func TestHealthEndpointReportsOK(t *testing.T) {
	_, addr, stop := startRunningServerForTest(t, "")

	client := &http.Client{Timeout: time.Second}
	resp, err := client.Get("http://" + addr + "/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if got := resp.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("health Content-Type = %q, want application/json", got)
	}
	var body map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if body["status"] != "ok" {
		t.Fatalf("health body = %#v, want status=ok", body)
	}

	stop()
}

func TestStartStopsOnContextCancellation(t *testing.T) {
	_, addr, stop := startRunningServerForTest(t, "")
	stop()

	client := &http.Client{Timeout: time.Second, Transport: &http.Transport{DisableKeepAlives: true}}
	resp, err := client.Get("http://" + addr + "/health")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("health endpoint remained reachable after server cancellation; status=%d", resp.StatusCode)
	}
}

func TestShutdownClosesActiveWebSocketClient(t *testing.T) {
	server, addr, stop := startRunningServerForTest(t, "")
	wsURL := "ws" + strings.TrimPrefix("http://"+addr, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial running WebSocket server: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if greeting := readTestMessage(t, conn); greeting.Type != protocol.MsgAgentInfo {
		t.Fatalf("greeting type = %q, want %q", greeting.Type, protocol.MsgAgentInfo)
	}
	client := registeredClient(t, server)

	stop()
	select {
	case <-client.done:
	case <-time.After(2 * time.Second):
		t.Fatal("server shutdown did not close the active client")
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set client close deadline: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("client WebSocket remained readable after server shutdown")
	}
}

func TestConcurrentRegistrationsRespectMaxClients(t *testing.T) {
	s := NewServer("", "", "", "")
	const registrationAttempts = MaxClients * 2
	accepted := make(chan *Client, registrationAttempts)
	var wg sync.WaitGroup
	for i := 0; i < registrationAttempts; i++ {
		client := &Client{
			server: s,
			send:   make(chan []byte, 1),
			done:   make(chan struct{}),
		}
		wg.Add(1)
		go func(c *Client) {
			defer wg.Done()
			if s.register(c) {
				accepted <- c
			}
		}(client)
	}
	wg.Wait()
	close(accepted)

	clients := make([]*Client, 0, MaxClients)
	for client := range accepted {
		clients = append(clients, client)
	}
	if len(clients) != MaxClients {
		t.Fatalf("concurrent accepted client count = %d, want %d", len(clients), MaxClients)
	}

	s.mu.RLock()
	active := len(s.clients)
	s.mu.RUnlock()
	if active != MaxClients {
		t.Fatalf("concurrent active client count = %d, want %d", active, MaxClients)
	}
	for _, client := range clients {
		s.unregister(client)
	}
}

func registeredClient(t *testing.T, s *Server) *Client {
	t.Helper()
	s.mu.RLock()
	defer s.mu.RUnlock()
	if len(s.clients) != 1 {
		t.Fatalf("registered client count = %d, want 1", len(s.clients))
	}
	for client := range s.clients {
		return client
	}
	t.Fatal("registered client map was unexpectedly empty")
	return nil
}

func startRunningServerForTest(t *testing.T, authToken string) (*Server, string, func()) {
	t.Helper()
	addr := reserveTCPAddress(t)
	server := NewServer(addr, "", "", authToken)
	ctx, cancel := context.WithCancel(context.Background())
	startDone := make(chan struct{})
	var startErr error
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			select {
			case <-startDone:
				if startErr != nil {
					t.Errorf("server Start returned error during cleanup: %v", startErr)
				}
			case <-time.After(2 * time.Second):
				t.Error("server Start did not return after context cancellation")
			}
		})
	}
	t.Cleanup(stop)
	go func() {
		defer close(startDone)
		startErr = server.Start(ctx)
	}()

	waitForHealthEndpoint(t, addr, startDone, &startErr)
	return server, addr, stop
}

func reserveTCPAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve TCP address: %v", err)
	}
	addr := listener.Addr().String()
	if err := listener.Close(); err != nil {
		t.Fatalf("release reserved TCP address: %v", err)
	}
	return addr
}

func waitForHealthEndpoint(t *testing.T, addr string, startDone <-chan struct{}, startErr *error) {
	t.Helper()
	client := &http.Client{Timeout: 100 * time.Millisecond}
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	retry := time.NewTicker(10 * time.Millisecond)
	defer retry.Stop()

	for {
		resp, err := client.Get("http://" + addr + "/health")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
		}

		select {
		case <-startDone:
			t.Fatalf("server Start returned before becoming ready: %v", *startErr)
		case <-deadline.C:
			t.Fatalf("server did not expose /health at %s", addr)
		case <-retry.C:
		}
	}
}
