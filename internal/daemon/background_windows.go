//go:build windows

package daemon

import (
	"os/exec"
	"syscall"
)

const (
	windowsDetachedProcess       = 0x00000008
	windowsCreateNewProcessGroup = 0x00000200
)

func newBackgroundCommand(binaryPath string, args []string) *exec.Cmd {
	cmd := exec.Command(binaryPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windowsDetachedProcess | windowsCreateNewProcessGroup,
		HideWindow:    true,
	}
	return cmd
}
