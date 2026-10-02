package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readEvents(t *testing.T, path string) []Event {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()
	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var e Event
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("log line is not JSON: %q: %v", scanner.Text(), err)
		}
		events = append(events, e)
	}
	return events
}

func TestNilAndDisabledLoggerAreSafe(t *testing.T) {
	var l *Logger
	l.Log(Event{Action: "x"})
	l.Close()

	disabled, err := Open("")
	if err != nil || disabled != nil {
		t.Fatalf("Open(\"\") = (%v, %v), want (nil, nil)", disabled, err)
	}
}

func TestLogWritesOneJSONLinePerEventAndTruncatesDetail(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "audit.log")
	l, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	l.Log(Event{Action: "auth", Result: ResultDenied, Remote: "10.0.0.5:4000", Detail: "line1\nline2"})
	l.Log(Event{Action: "exec_command", Result: ResultOK, Detail: strings.Repeat("é", 2000)})
	l.Close()

	events := readEvents(t, path)
	if len(events) != 2 {
		t.Fatalf("got %d events, want 2", len(events))
	}
	if events[0].Action != "auth" || events[0].Result != ResultDenied || events[0].Time.IsZero() {
		t.Fatalf("first event = %+v", events[0])
	}
	if events[0].Detail != "line1\nline2" {
		t.Fatalf("newline in detail must round-trip through JSON, got %q", events[0].Detail)
	}
	if got := len([]rune(events[1].Detail)); got != maxDetailRunes+1 {
		t.Fatalf("detail length = %d runes, want %d (limit plus ellipsis)", got, maxDetailRunes+1)
	}
}

func TestLogRotatesOncePastMaxSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	l, err := open(path, 400)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	for i := 0; i < 20; i++ {
		l.Log(Event{Action: "tick", Result: ResultOK, Detail: strings.Repeat("a", 50)})
	}
	l.Close()

	if _, err := os.Stat(path + ".1"); err != nil {
		t.Fatalf("expected a rotated generation: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("current log missing: %v", err)
	}
	if info.Size() > 400 {
		t.Fatalf("current log is %d bytes, want <= 400 after rotation", info.Size())
	}
	if len(readEvents(t, path)) == 0 {
		t.Fatal("current log is empty after rotation")
	}
}

func TestReopenAppendsToExistingLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit.log")
	for i := 0; i < 2; i++ {
		l, err := Open(path)
		if err != nil {
			t.Fatalf("Open: %v", err)
		}
		l.Log(Event{Action: "start", Result: ResultOK})
		l.Close()
	}
	if got := len(readEvents(t, path)); got != 2 {
		t.Fatalf("got %d events after reopening, want 2 (append, not truncate)", got)
	}
}
