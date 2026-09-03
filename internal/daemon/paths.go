package daemon

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
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

// WritePID is implemented per platform in pid_write_unix.go and pid_write_windows.go.

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

	// Homebrew on macOS: stable symlinks (/opt/homebrew/opt/* and
	// /opt/homebrew/bin/*, plus /usr/local equivalents on Intel) must not
	// be resolved to Cellar/X.Y.Z, which is deleted on `brew upgrade` and
	// would break the launchd plist. Only applies to darwin + brew prefixes;
	// all other installs (go install, direct download, Linux) keep EvalSymlinks.
	if runtime.GOOS == "darwin" && isHomebrewStablePath(execPath) {
		if abs, err := filepath.Abs(execPath); err == nil {
			return abs
		}
		return execPath
	}

	resolved, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		slog.Warn("symlink resolution failed, using raw path", "path", execPath, "err", err)
		return execPath
	}
	return resolved
}

func isHomebrewStablePath(p string) bool {
	return strings.HasPrefix(p, "/opt/homebrew/opt/") ||
		strings.HasPrefix(p, "/opt/homebrew/bin/") ||
		strings.HasPrefix(p, "/usr/local/opt/") ||
		strings.HasPrefix(p, "/usr/local/bin/")
}
