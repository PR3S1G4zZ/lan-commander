//go:build windows

package executor

import (
	"os/exec"
	"strings"
	"syscall"
)

// applyShellCommandLine hands the command to cmd.exe verbatim. Go would
// otherwise escape embedded double quotes with backslashes, which cmd.exe does
// not understand, so `echo "hi"` would print \"hi\". The /s switch makes cmd
// strip only the outer pair of quotes added here.
func applyShellCommandLine(cmd *exec.Cmd, shellCmd, command string) {
	if !strings.EqualFold(shellCmd, "cmd.exe") {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CmdLine = `cmd.exe /d /s /c "` + command + `"`
}
