package executor

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// slowCommand returns a shell command that takes roughly the given number of
// seconds on the current OS.
func slowCommand(seconds int) string {
	if runtime.GOOS == "windows" {
		// ping sends one echo per second; n echoes take about n-1 seconds.
		return "ping -n " + string(rune('0'+seconds+1)) + " 127.0.0.1 >nul"
	}
	return "sleep " + string(rune('0'+seconds))
}

func TestExecuteReturnsOutputAndExitCode(t *testing.T) {
	result, err := Execute("echo hello", nil, 10, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Stdout, "hello") {
		t.Fatalf("result = %#v, want exit 0 and stdout containing hello", result)
	}

	result, err = Execute("exit 3", nil, 10, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != 3 {
		t.Fatalf("exit code = %d, want 3", result.ExitCode)
	}
}

func TestExecuteReportsTimeoutAsMinusOneAndStopsWaiting(t *testing.T) {
	start := time.Now()
	result, err := Execute(slowCommand(8), nil, 1, "")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for a timeout", result.ExitCode)
	}
	if elapsed > 6*time.Second {
		t.Fatalf("Execute took %v; the 1s timeout was not enforced", elapsed)
	}
}

func TestCappedWriterDropsExcessButReportsOriginalLength(t *testing.T) {
	w := &cappedWriter{w: new(bytes.Buffer), limit: 4}
	n, err := w.Write([]byte("abcdefgh"))
	if err != nil || n != 8 {
		t.Fatalf("Write = (%d, %v), want (8, nil): the caller must see the original length", n, err)
	}
	if got := w.w.String(); got != "abcd" {
		t.Fatalf("captured %q, want %q", got, "abcd")
	}
}

func bigOutputCommand() string {
	if runtime.GOOS == "windows" {
		return "for /L %i in (1,1,100000) do @echo 0123456789abcdef0123456789abcdef"
	}
	return "yes 0123456789abcdef0123456789abcdef | head -n 100000"
}

func TestExecuteWithOutputAboveTheCapStillSucceeds(t *testing.T) {
	start := time.Now()
	result, err := Execute(bigOutputCommand(), nil, 20, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	t.Logf("exit=%d stdout=%d bytes elapsed=%v", result.ExitCode, len(result.Stdout), time.Since(start))
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0: output above the cap must be truncated, not turn into a failure", result.ExitCode)
	}
	if len(result.Stdout) != MaxOutputSize {
		t.Fatalf("stdout length = %d, want exactly the cap %d", len(result.Stdout), MaxOutputSize)
	}
}

func TestTimeoutKillsTheWholeProcessTree(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survivor")

	// A grandchild shell that would create the marker after the timeout if it
	// were left running when only the top-level shell is killed.
	var command string
	if runtime.GOOS == "windows" {
		command = `cmd /c "ping -n 4 127.0.0.1 >nul & echo alive> "` + marker + `""`
	} else {
		command = `sh -c 'sleep 3; touch "` + marker + `"' & wait`
	}

	result, err := Execute(command, nil, 1, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for a timeout", result.ExitCode)
	}

	time.Sleep(5 * time.Second)
	if _, statErr := os.Stat(marker); statErr == nil {
		t.Fatal("a child process survived the timeout and kept running")
	}
}

func TestExecutePassesDoubleQuotesToTheShellUnescaped(t *testing.T) {
	result, err := Execute(`echo "hi there"`, nil, 10, "")
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if strings.Contains(result.Stdout, `\`) || !strings.Contains(result.Stdout, "hi there") {
		t.Fatalf("stdout = %q, want the text without backslash-escaped quotes", result.Stdout)
	}
}
