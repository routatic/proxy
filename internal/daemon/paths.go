package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
)

const (
	AppName       = "routatic-proxy"
	LegacyAppName = "oc-go-cc"
	ConfigDir     = ".config/routatic-proxy"
	LaunchAgent   = "com.routatic.proxy"
)

// Paths holds well-known directories and files for the app.
type Paths struct {
	ConfigDir  string // ~/.config/routatic-proxy
	PIDFile    string // ~/.config/routatic-proxy/routatic-proxy.pid
	LogFile    string // ~/.config/routatic-proxy/routatic-proxy.log
	PlistPath  string // ~/Library/LaunchAgents/com.routatic.proxy.plist
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
	paths := &Paths{
		ConfigDir:  configDir,
		PIDFile:    filepath.Join(configDir, AppName+".pid"),
		LogFile:    filepath.Join(configDir, AppName+".log"),
		BinaryPath: execPath,
	}
	if runtime.GOOS == "darwin" {
		paths.PlistPath = filepath.Join(home, "Library", "LaunchAgents", LaunchAgent+".plist")
	}
	return paths, nil
}

// EnsureConfigDir creates ~/.config/routatic-proxy/ if it does not exist.
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
// Uses O_NOFOLLOW to atomically reject symlinks at open time,
// preventing symlink-traversal attacks (CWE-59).
func WritePID(pidPath string, pid int) (err error) {
	f, err := os.OpenFile(pidPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC|syscall.O_NOFOLLOW, 0644)
	if err != nil {
		return fmt.Errorf("refusing to write PID file: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	_, err = fmt.Fprintf(f, "%d", pid)
	return err
}

// FindBinary returns the absolute path to the routatic-proxy binary.
func FindBinary() (string, error) {
	// First try to use the current executable
	execPath, err := os.Executable()
	if err == nil {
		return resolveExecutablePath(execPath), nil
	}

	// Fallback: search PATH for routatic-proxy, then the legacy oc-go-cc alias.
	execPath, err = exec.LookPath(AppName)
	if err != nil {
		execPath, err = exec.LookPath(LegacyAppName)
		if err != nil {
			return "", fmt.Errorf("cannot find %s binary: %w", AppName, err)
		}
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
