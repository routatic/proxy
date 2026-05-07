//go:build !windows

package daemon

import (
	"os/exec"
	"syscall"
)

func newBackgroundCommand(binaryPath string, args []string) *exec.Cmd {
	cmd := exec.Command("nohup", append([]string{binaryPath}, args...)...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return cmd
}
