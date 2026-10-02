package server

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/mediacode/lan-commander/agent/internal/audit"
	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

func wsURLFor(ts *httptest.Server) string {
	return "ws" + strings.TrimPrefix(ts.URL, "http")
}

func TestConnectionsFromOneAddressAreCappedAndSlotsAreReleased(t *testing.T) {
	s := NewServer("", "", "", "tok")
	s.guard.maxActive = 2
	ts := httptest.NewServer(httpHandler(s))
	t.Cleanup(ts.Close)

	first, _, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
	if err != nil {
		t.Fatalf("first dial: %v", err)
	}
	second, _, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
	if err != nil {
		t.Fatalf("second dial: %v", err)
	}
	t.Cleanup(func() { _ = second.Close() })

	_, resp, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
	if err == nil {
		t.Fatal("third connection from the same address should have been refused")
	}
	if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("refusal status = %v, want 429", resp)
	}

	_ = first.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		conn, _, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
		if err == nil {
			_ = conn.Close()
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("slot was not released after the first client left: %v", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestRepeatedAuthFailuresAcrossConnectionsBlockTheAddress(t *testing.T) {
	s := NewServer("", "", "", "right")
	s.guard.maxFailures = 4
	ts := httptest.NewServer(httpHandler(s))
	t.Cleanup(ts.Close)

	for i := 0; i < 4; i++ {
		conn, _, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		readTestMessage(t, conn) // auth_required
		if err := conn.WriteJSON(protocol.Message{ID: "a", Type: protocol.MsgAuth, Payload: protocol.AuthPayload{Token: "wrong"}}); err != nil {
			t.Fatalf("send auth: %v", err)
		}
		readUntilID(t, conn, "a", 2*time.Second)
		_ = conn.Close()
	}

	// Failures are counted on the server after the response is queued; allow
	// the last one to land before probing.
	deadline := time.Now().Add(2 * time.Second)
	for {
		conn, resp, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
		if err != nil {
			if resp == nil || resp.StatusCode != http.StatusTooManyRequests {
				t.Fatalf("unexpected refusal: %v (resp %v)", err, resp)
			}
			return
		}
		_ = conn.Close()
		if time.Now().After(deadline) {
			t.Fatal("address was not blocked after repeated authentication failures")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func readAuditFile(t *testing.T, path string) []audit.Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open audit log: %v", err)
	}
	defer f.Close()
	var events []audit.Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e audit.Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("bad audit line %q: %v", scanner.Text(), err)
		}
		events = append(events, e)
	}
	return events
}

func TestAuditTrailRecordsAuthenticationAndRemoteOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	logger, err := audit.Open(path)
	if err != nil {
		t.Fatalf("open audit: %v", err)
	}

	s := NewServer("", "", "", "tok")
	s.SetAuditLogger(logger)
	ts := httptest.NewServer(httpHandler(s))
	t.Cleanup(ts.Close)

	conn, _, err := websocket.DefaultDialer.Dial(wsURLFor(ts), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	readTestMessage(t, conn)

	send := func(id, typ string, payload interface{}) {
		if err := conn.WriteJSON(protocol.Message{ID: id, Type: typ, Payload: payload}); err != nil {
			t.Fatalf("send %s: %v", typ, err)
		}
		readUntilID(t, conn, id, 5*time.Second)
	}

	send("bad", protocol.MsgAuth, protocol.AuthPayload{Token: "nope", Username: "mallory"})
	send("ok", protocol.MsgAuth, protocol.AuthPayload{Token: "tok", Username: "alice"})
	send("ls", protocol.MsgListDir, protocol.ListDirPayload{Path: t.TempDir()})
	send("ex", protocol.MsgExecCommand, protocol.ExecCommandPayload{Command: "echo audited", Timeout: 10})
	logger.Close()

	events := readAuditFile(t, path)
	byKey := map[string]audit.Event{}
	for _, e := range events {
		byKey[e.Action+"/"+e.Result] = e
	}

	if e, ok := byKey["auth/denied"]; !ok || e.User != "mallory" || !strings.HasPrefix(e.Remote, "127.0.0.1:") {
		t.Errorf("failed login not recorded correctly: %+v (all: %+v)", e, events)
	}
	if e, ok := byKey["auth/ok"]; !ok || e.User != "alice" {
		t.Errorf("successful login not recorded correctly: %+v", e)
	}
	if _, ok := byKey["list_dir/ok"]; !ok {
		t.Errorf("list_dir not recorded: %+v", events)
	}
	if e, ok := byKey["exec_command/ok"]; !ok || !strings.Contains(e.Detail, "echo audited") {
		t.Errorf("exec_command not recorded with its command line: %+v", e)
	}
}

func TestSlowConsumerIsDisconnectedInsteadOfLosingResponsesSilently(t *testing.T) {
	s := NewServer("", "", "", "")
	c := &Client{
		server: s,
		send:   make(chan []byte, 2),
		done:   make(chan struct{}),
		id:     "slow",
	}

	c.sendMsg(protocol.Message{ID: "1", Type: protocol.MsgDirContents})
	c.sendMsg(protocol.Message{ID: "2", Type: protocol.MsgDirContents})
	select {
	case <-c.done:
		t.Fatal("connection closed while the queue still had room")
	default:
	}

	c.sendMsg(protocol.Message{ID: "3", Type: protocol.MsgDirContents})
	select {
	case <-c.done:
	default:
		t.Fatal("a response that does not fit in the queue must close the connection, not vanish")
	}
}

func TestPeriodicUpdatesAreSkippedWithoutClosingWhenTheQueueIsFull(t *testing.T) {
	s := NewServer("", "", "", "")
	c := &Client{
		server: s,
		send:   make(chan []byte, 1),
		done:   make(chan struct{}),
		id:     "busy",
	}
	c.send <- []byte("occupied")

	c.pushSystemUpdate()

	select {
	case <-c.done:
		t.Fatal("a skipped periodic push must not drop the connection")
	default:
	}
}
