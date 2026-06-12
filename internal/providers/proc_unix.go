//go:build !windows

package providers

import (
	"os/exec"
	"syscall"
	"time"
)

// setupProcessGroup ensures the CLI and any children it spawns are killed
// together when the context is cancelled.
func setupProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = 10 * time.Second
}
