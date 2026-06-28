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
	// Wait briefly to give the current process time to exit and release its
	// lock on the running executable, then force-delete the backup. timeout.exe
	// is used instead of ping so the deletion still works on networks where ICMP
	// is blocked or filtered.
	cmd := exec.Command("cmd", "/c", fmt.Sprintf("timeout /t 3 /nobreak > nul && del /f %s", windowsQuote(path)))
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
