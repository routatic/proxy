//go:build windows

package updater

import (
	"fmt"
	"os/exec"
	"syscall"
)

func init() {
	cleanupBackup = scheduleDeleteWindows
}

// scheduleDeleteWindows removes a file after a short delay.
// This is needed because Windows keeps the running executable locked
// until the process exits, so the backup cannot be deleted immediately.
func scheduleDeleteWindows(path string) {
	// Ping gives the current process time to exit before attempting deletion.
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("ping 127.0.0.1 -n 3 > nul & del %s", windowsQuote(path)))
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	_ = cmd.Start()
}

func windowsQuote(s string) string {
	if s == "" {
		return `""`
	}
	return `"` + s + `"`
}
