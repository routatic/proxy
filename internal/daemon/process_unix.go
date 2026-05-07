//go:build !windows

package daemon

import (
	"fmt"
	"os"
	"syscall"
)

// IsProcessRunning checks if a process with the given PID is running.
func IsProcessRunning(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Send signal 0 to check if process exists without actually signaling it.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// StopProcess sends SIGTERM to a process and waits for it to exit.
func StopProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("cannot find process: %w", err)
	}

	if err := process.Signal(os.Signal(syscall.SIGTERM)); err != nil {
		return fmt.Errorf("cannot send SIGTERM: %w", err)
	}

	// Wait for the process to exit.
	_, _ = process.Wait()
	return nil
}
