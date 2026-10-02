package server

import (
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

func slowShellCommand() string {
	if runtime.GOOS == "windows" {
		return "ping -n 4 127.0.0.1 >nul" // about 3 seconds
	}
	return "sleep 3"
}

// readUntilID reads messages (skipping periodic system_update pushes) until one
// with the given ID arrives.
func readUntilID(t *testing.T, conn *websocket.Conn, id string, within time.Duration) protocol.Message {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(deadline); err != nil {
			t.Fatalf("set read deadline: %v", err)
		}
		var msg protocol.Message
		if err := conn.ReadJSON(&msg); err != nil {
			t.Fatalf("connection dropped while waiting for response %q: %v", id, err)
		}
		if msg.ID == id {
			return msg
		}
	}
	t.Fatalf("no response with ID %q within %v", id, within)
	return protocol.Message{}
}

func TestCommandLongerThanReadTimeoutStillDeliversResultAndKeepsConnection(t *testing.T) {
	// The command takes about 3s but the idle read timeout is 1s: the deadline
	// armed when the request arrived expires while the handler is running.
	_, conn := startTestWebSocketServer(t, "tok", 1*time.Second)
	readTestMessage(t, conn) // auth_required
	if err := conn.WriteJSON(protocol.Message{ID: "auth", Type: protocol.MsgAuth, Payload: protocol.AuthPayload{Token: "tok"}}); err != nil {
		t.Fatalf("send auth: %v", err)
	}
	readUntilID(t, conn, "auth", 2*time.Second)

	if err := conn.WriteJSON(protocol.Message{
		ID:      "slow",
		Type:    protocol.MsgExecCommand,
		Payload: protocol.ExecCommandPayload{Command: slowShellCommand(), Timeout: 10},
	}); err != nil {
		t.Fatalf("send exec_command: %v", err)
	}
	if result := readUntilID(t, conn, "slow", 8*time.Second); result.Type != protocol.MsgCommandResult {
		t.Fatalf("response type = %q (error %q), want %q", result.Type, result.Error, protocol.MsgCommandResult)
	}

	// The connection must still be usable after the long command.
	if err := conn.WriteJSON(protocol.Message{ID: "after", Type: protocol.MsgSystemInfo}); err != nil {
		t.Fatalf("send system_info after long command: %v", err)
	}
	if info := readUntilID(t, conn, "after", 3*time.Second); info.Type != protocol.MsgSystemUpdate {
		t.Fatalf("response type = %q, want %q", info.Type, protocol.MsgSystemUpdate)
	}
}

func dialServerWithAuthTimeout(t *testing.T, token string, authTimeout time.Duration) *websocket.Conn {
	t.Helper()
	s := NewServer("", "", "", token)
	s.authTimeout = authTimeout
	httpServer := httptest.NewServer(httpHandler(s))
	t.Cleanup(httpServer.Close)

	wsURL := "ws" + strings.TrimPrefix(httpServer.URL, "http")
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial test WebSocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func TestUnauthenticatedConnectionIsClosedEvenIfItSendsKeepAlives(t *testing.T) {
	conn := dialServerWithAuthTimeout(t, "tok", 400*time.Millisecond)
	readTestMessage(t, conn) // auth_required

	stop := time.After(3 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	closed := make(chan struct{})
	go func() {
		defer close(closed)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()

	for {
		select {
		case <-closed:
			return // the agent closed the idle anonymous connection
		case <-ticker.C:
			_ = conn.WriteJSON(protocol.Message{Type: protocol.MsgKeepAlive})
		case <-stop:
			t.Fatal("unauthenticated connection stayed open past the authentication timeout")
		}
	}
}

func TestAuthenticatedConnectionSurvivesTheAuthenticationTimeout(t *testing.T) {
	conn := dialServerWithAuthTimeout(t, "tok", 300*time.Millisecond)
	readTestMessage(t, conn) // auth_required
	if err := conn.WriteJSON(protocol.Message{ID: "auth", Type: protocol.MsgAuth, Payload: protocol.AuthPayload{Token: "tok"}}); err != nil {
		t.Fatalf("send auth: %v", err)
	}
	readUntilID(t, conn, "auth", 2*time.Second)

	time.Sleep(700 * time.Millisecond)

	if err := conn.WriteJSON(protocol.Message{ID: "still-here", Type: protocol.MsgSystemInfo}); err != nil {
		t.Fatalf("send after timeout: %v", err)
	}
	readUntilID(t, conn, "still-here", 3*time.Second)
}
