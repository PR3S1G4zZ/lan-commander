//go:build !windows

package executor

import (
	"os/exec"
	"syscall"
)

// configureProcessTree puts the shell in its own process group and makes the
// timeout kill that whole group, so background children do not outlive it.
func configureProcessTree(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
