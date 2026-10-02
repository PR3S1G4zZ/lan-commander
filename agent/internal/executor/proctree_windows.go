//go:build windows

package executor

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
)

// configureProcessTree makes the timeout terminate the shell together with
// every process it started. Killing only cmd.exe leaves its children running.
func configureProcessTree(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		return nil
	}
}
