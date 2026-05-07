package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

const (
	AppName     = "oc-go-cc"
	ConfigDir   = ".config/oc-go-cc"
	LaunchAgent = "com.opencode.oc-go-cc"
)

// Paths holds well-known directories and files for the app.
type Paths struct {
	ConfigDir  string // ~/.config/oc-go-cc
	PIDFile    string // ~/.config/oc-go-cc/oc-go-cc.pid
	LogFile    string // ~/.config/oc-go-cc/oc-go-cc.log
	PlistPath  string // ~/Library/LaunchAgents/com.opencode.oc-go-cc.plist
	BinaryPath string // absolute path to the running executable
}

// DefaultPaths computes paths from the user's home directory.
func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("cannot determine home directory: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("cannot determine executable path: %w", err)
	}
	execPath = resolveExecutablePath(execPath)

	configDir := filepath.Join(home, ConfigDir)
	return &Paths{
		ConfigDir:  configDir,
		PIDFile:    filepath.Join(configDir, AppName+".pid"),
		LogFile:    filepath.Join(configDir, AppName+".log"),
		PlistPath:  filepath.Join(home, "Library", "LaunchAgents", LaunchAgent+".plist"),
		BinaryPath: execPath,
	}, nil
}

// EnsureConfigDir creates ~/.config/oc-go-cc/ if it does not exist.
func (p *Paths) EnsureConfigDir() error {
	return os.MkdirAll(p.ConfigDir, 0755)
}

// GetPID reads the PID from the PID file.
func GetPID(pidPath string) (int, error) {
	data, err := os.ReadFile(pidPath)
	if err != nil {
		return 0, err
	}
	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	if err != nil {
		return 0, fmt.Errorf("invalid PID in file: %w", err)
	}
	return pid, nil
}

// WritePID writes the given PID to a file.
func WritePID(pidPath string, pid int) error {
	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// FindBinary returns the absolute path to the oc-go-cc binary.
func FindBinary() (string, error) {
	// First try to use the current executable
	execPath, err := os.Executable()
	if err == nil {
		return resolveExecutablePath(execPath), nil
	}

	// Fallback: search PATH for oc-go-cc
	execPath, err = exec.LookPath(AppName)
	if err != nil {
		return "", fmt.Errorf("cannot find oc-go-cc binary: %w", err)
	}
	return resolveExecutablePath(execPath), nil
}

func resolveExecutablePath(execPath string) string {
	// Scoop on Windows launches applications through shims. Resolving those paths
	// can fail or bypass the shim behavior, so keep the executable path exactly
	// as Windows reported it.
	if runtime.GOOS == "windows" {
		return execPath
	}

	resolved, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		slog.Warn("symlink resolution failed, using raw path", "path", execPath, "err", err)
		return execPath
	}
	return resolved
}
