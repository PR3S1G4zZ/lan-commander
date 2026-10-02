//go:build !windows

package executor

import "os/exec"

func applyShellCommandLine(cmd *exec.Cmd, shellCmd, command string) {}
