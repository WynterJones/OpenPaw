//go:build windows

package providers

import (
	"os/exec"
	"time"
)

func setupProcessGroup(cmd *exec.Cmd) {
	cmd.WaitDelay = 10 * time.Second
}
