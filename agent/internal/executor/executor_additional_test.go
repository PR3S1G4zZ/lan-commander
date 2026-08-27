package executor

import (
	"bytes"
	"fmt"
	"os/exec"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func testShell(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		if err := ValidateShell(""); err != nil {
			t.Skipf("the default Windows shell is unavailable: %v", err)
		}
		return ""
	}
	if err := ValidateShell("sh"); err != nil {
		t.Skipf("the POSIX shell is unavailable: %v", err)
	}
	return "sh"
}

func TestExecuteRunsCommandAndCapturesStdout(t *testing.T) {
	shell := testShell(t)
	result, err := Execute("echo", []string{"lan-commander", "executor"}, 5, shell)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if got := strings.TrimSpace(result.Stdout); got != "lan-commander executor" {
		t.Fatalf("stdout = %q, want %q", got, "lan-commander executor")
	}
	if result.Stderr != "" {
		t.Fatalf("stderr = %q, want empty", result.Stderr)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
	if result.Duration < 0 {
		t.Fatalf("duration = %d ms, want non-negative", result.Duration)
	}
}

func TestExecuteCapturesStderr(t *testing.T) {
	shell := testShell(t)
	result, err := Execute("echo", []string{"lan-commander-stderr", "1>&2"}, 5, shell)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if got := strings.TrimSpace(result.Stderr); got != "lan-commander-stderr" {
		t.Fatalf("stderr = %q, want %q", got, "lan-commander-stderr")
	}
	if result.Stdout != "" {
		t.Fatalf("stdout = %q, want empty", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", result.ExitCode)
	}
}

func TestExecuteReportsNonZeroExitCodeWithoutReturningGoError(t *testing.T) {
	shell := testShell(t)
	result, err := Execute("exit", []string{"7"}, 5, shell)
	if err != nil {
		t.Fatalf("Execute returned an error for a command exit status: %v", err)
	}
	if result.ExitCode != 7 {
		t.Fatalf("exit code = %d, want 7", result.ExitCode)
	}
}

func TestExecuteReportsTimeout(t *testing.T) {
	shell := testShell(t)
	command := "sleep"
	args := []string{"2"}
	if runtime.GOOS == "windows" {
		if err := ValidateShell("powershell"); err != nil {
			t.Skipf("PowerShell is unavailable for the Windows timeout test: %v", err)
		}
		shell = "powershell"
		command = "Start-Sleep"
		args = []string{"-Seconds", "2"}
	} else if _, err := exec.LookPath(command); err != nil {
		t.Skipf("the timeout command is unavailable: %v", err)
	}

	result, err := Execute(command, args, 1, shell)
	if err != nil {
		t.Fatalf("Execute returned an error for a timed-out command: %v", err)
	}
	if result.ExitCode != -1 {
		t.Fatalf("exit code = %d, want -1 for timeout", result.ExitCode)
	}
	if result.Duration < 500 {
		t.Fatalf("duration = %d ms, want evidence the timeout was reached", result.Duration)
	}
}

func TestExecuteCapsStdoutAtMaxOutputSize(t *testing.T) {
	shell := testShell(t)
	command := "head"
	outputSize := MaxOutputSize
	args := []string{"-c", fmt.Sprint(outputSize), "/dev/zero"}
	if runtime.GOOS == "windows" {
		if err := ValidateShell("powershell"); err != nil {
			t.Skipf("PowerShell is unavailable for the output-cap test: %v", err)
		}
		shell = "powershell"
		command = "Write-Output"
		// Write-Output appends CRLF, so keep the command output exactly at the cap.
		outputSize -= 2
		args = []string{fmt.Sprintf("('x' * %d)", outputSize)}
	} else if _, err := exec.LookPath(command); err != nil {
		t.Skipf("the output generator is unavailable: %v", err)
	}

	result, err := Execute(command, args, 10, shell)
	if err != nil {
		t.Fatalf("Execute returned an error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 for a successful capped command (stderr %q)", result.ExitCode, result.Stderr)
	}
	if len(result.Stdout) != MaxOutputSize {
		t.Fatalf("stdout length = %d, want %d", len(result.Stdout), MaxOutputSize)
	}
}

func TestCappedWriterDiscardsBytesPastLimitAndReportsThemAsAccepted(t *testing.T) {
	var buffer bytes.Buffer
	writer := &cappedWriter{w: &buffer, limit: 5}

	if n, err := writer.Write([]byte("abc")); err != nil || n != 3 {
		t.Fatalf("first Write = (%d, %v), want (3, nil)", n, err)
	}
	if n, err := writer.Write([]byte("def")); err != nil || n != 3 {
		t.Fatalf("overflowing Write = (%d, %v), want (3, nil)", n, err)
	}
	if n, err := writer.Write([]byte("g")); err != nil || n != 1 {
		t.Fatalf("Write after limit = (%d, %v), want (1, nil)", n, err)
	}
	if got := buffer.String(); got != "abcde" {
		t.Fatalf("buffer = %q, want %q", got, "abcde")
	}
	if writer.total != 5 {
		t.Fatalf("total = %d, want 5", writer.total)
	}
}

func TestDetectShellUsesPlatformDefaultsAndExplicitShell(t *testing.T) {
	defaultCommand, defaultArgs := detectShell("")
	if runtime.GOOS == "windows" {
		if defaultCommand != "cmd.exe" || !reflect.DeepEqual(defaultArgs, []string{"/c"}) {
			t.Fatalf("default shell = %q %v, want cmd.exe [/c]", defaultCommand, defaultArgs)
		}
		command, args := detectShell("pwsh")
		if command != "powershell" || !reflect.DeepEqual(args, []string{"-NoProfile", "-NonInteractive", "-Command"}) {
			t.Fatalf("PowerShell selection = %q %v, want powershell [-NoProfile -NonInteractive -Command]", command, args)
		}
		return
	}

	if defaultCommand != "/bin/bash" || !reflect.DeepEqual(defaultArgs, []string{"-c"}) {
		t.Fatalf("default shell = %q %v, want /bin/bash [-c]", defaultCommand, defaultArgs)
	}
	command, args := detectShell("sh")
	if command != "/bin/sh" || !reflect.DeepEqual(args, []string{"-c"}) {
		t.Fatalf("POSIX sh selection = %q %v, want /bin/sh [-c]", command, args)
	}
}

func TestIsWindowsMatchesRuntime(t *testing.T) {
	if got, want := IsWindows(), runtime.GOOS == "windows"; got != want {
		t.Fatalf("IsWindows() = %v, want %v for GOOS %q", got, want, runtime.GOOS)
	}
}
