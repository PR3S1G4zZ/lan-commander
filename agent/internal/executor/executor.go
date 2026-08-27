package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

const (
	// MaxOutputSize limits command output to 1MB.
	MaxOutputSize = 1 * 1024 * 1024
	// MaxTimeout is the hard cap on command execution (300 seconds).
	MaxTimeout = 300
)

// Execute runs a command in the native shell and returns the result.
// The shell parameter can be "powershell" on Windows or "bash"/"sh" on Unix.
// timeout is in seconds; 0 means no timeout (but MaxTimeout still applies).
func Execute(cmd string, args []string, timeout int, shell string) (protocol.CommandResultPayload, error) {
	shellCmd, shellArgs := detectShell(shell)

	// Build the full command line
	fullCmd := cmd
	if len(args) > 0 {
		fullCmd += " " + strings.Join(args, " ")
	}
	shellArgs = append(shellArgs, fullCmd)

	// Enforce maximum timeout
	if timeout <= 0 || timeout > MaxTimeout {
		timeout = MaxTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	execCmd := exec.CommandContext(ctx, shellCmd, shellArgs...)

	var stdout, stderr bytes.Buffer
	// Limit output by using capped writers
	execCmd.Stdout = &cappedWriter{w: &stdout, limit: MaxOutputSize}
	execCmd.Stderr = &cappedWriter{w: &stderr, limit: MaxOutputSize}

	start := time.Now()
	err := execCmd.Run()
	durationMs := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			exitCode = -1 // timeout
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -2 // execution error
		}
	}

	return protocol.CommandResultPayload{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode,
		Duration: int(durationMs),
	}, nil
}

// detectShell returns the shell executable and initial args based on OS and preference.
func detectShell(shell string) (cmd string, args []string) {
	switch runtime.GOOS {
	case "windows":
		switch strings.ToLower(shell) {
		case "powershell", "pwsh":
			return "powershell", []string{"-NoProfile", "-NonInteractive", "-Command"}
		default:
			return "cmd.exe", []string{"/c"}
		}
	default:
		// Linux / macOS / Unix
		if shell == "sh" {
			return "/bin/sh", []string{"-c"}
		}
		return "/bin/bash", []string{"-c"}
	}
}

// cappedWriter discards writes that would exceed the limit.
type cappedWriter struct {
	w     *bytes.Buffer
	limit int
	total int
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	originalLen := len(p)
	remaining := c.limit - c.total
	if remaining <= 0 {
		return originalLen, nil // silently drop
	}
	if len(p) > remaining {
		p = p[:remaining]
	}
	n, err := c.w.Write(p)
	c.total += n
	if err != nil {
		return n, err
	}
	if n != len(p) {
		return n, io.ErrShortWrite
	}
	return originalLen, nil // report original length, including discarded bytes
}

// ValidateShell checks if the requested shell is available.
func ValidateShell(shell string) error {
	cmd, _ := detectShell(shell)
	if _, err := exec.LookPath(cmd); err != nil {
		return fmt.Errorf("shell %q not found: %w", cmd, err)
	}
	return nil
}

// IsWindows reports whether the agent runs on Windows.
func IsWindows() bool {
	return runtime.GOOS == "windows"
}
