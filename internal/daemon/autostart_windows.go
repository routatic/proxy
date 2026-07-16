//go:build windows

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	registryRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryValue  = LaunchAgent
)

func buildAutostartArgs(configPath string, port int) string {
	args := "start --background"
	if configPath != "" {
		args += ` --config "` + configPath + `"`
	}
	if port != 0 {
		args += fmt.Sprintf(" --port %d", port)
	}
	return args
}

// registryBinaryRef returns the string to use as the binary token in the
// registry Run value. If binaryPath lives inside a WindowsApps alias directory,
// the bare executable name is returned so Windows resolves it via PATH at
// login. Otherwise the quoted absolute path is returned.
func registryBinaryRef(binaryPath string) string {
	if strings.Contains(strings.ToLower(binaryPath), strings.ToLower(`\WindowsApps\`)) {
		return filepath.Base(binaryPath)
	}
	return `"` + binaryPath + `"`
}

// EnableAutostart adds a registry Run key so routatic-proxy starts on login.
func EnableAutostart(configPath string, port int) error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureConfigDir(); err != nil {
		return err
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, registryRunKey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("cannot open registry key: %w", err)
	}
	defer func() { _ = key.Close() }()

	binRef := registryBinaryRef(paths.BinaryPath)
	if err := validateBinaryRef(binRef); err != nil {
		return err
	}

	value := binRef + " " + buildAutostartArgs(configPath, port)
	if err := key.SetStringValue(registryValue, value); err != nil {
		return fmt.Errorf("cannot set registry value: %w", err)
	}

	fmt.Printf("Autostart enabled. %s will start on login.\n", AppName)
	fmt.Printf("  Registry: HKCU\\%s\\%s\n", registryRunKey, registryValue)
	return nil
}

func validateBinaryRef(binRef string) error {
	// Bare executable name: verify it can be found on PATH.
	if !strings.ContainsAny(binRef, `\/`) {
		if _, err := exec.LookPath(binRef); err != nil {
			return fmt.Errorf("cannot find %s on PATH: %w", binRef, err)
		}
		return nil
	}

	// Quoted absolute path: strip quotes and verify the file exists.
	path := strings.Trim(binRef, `"`)
	if _, err := os.Stat(path); err != nil {
		return fmt.Errorf("cannot access binary at %s: %w", path, err)
	}
	return nil
}

// DisableAutostart removes the registry Run key.
func DisableAutostart() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, registryRunKey, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return fmt.Errorf("cannot open registry key: %w", err)
	}
	defer func() { _ = key.Close() }()

	// Check if the value exists
	_, _, err = key.GetStringValue(registryValue)
	if err != nil {
		fmt.Println("Autostart is not enabled (no registry entry found)")
		return nil
	}

	if err := key.DeleteValue(registryValue); err != nil {
		return fmt.Errorf("cannot delete registry value: %w", err)
	}

	fmt.Printf("Autostart disabled. Registry entry removed.\n")
	return nil
}

// AutostartStatus reports whether autostart is enabled.
func AutostartStatus() error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}

	key, err := registry.OpenKey(registry.CURRENT_USER, registryRunKey, registry.READ)
	if err != nil {
		fmt.Println("Autostart: disabled (cannot read registry)")
		return nil
	}
	defer func() { _ = key.Close() }()

	val, _, err := key.GetStringValue(registryValue)
	if err != nil {
		fmt.Println("Autostart: disabled (no registry entry found)")
		return nil
	}

	// Verify the binary still exists at the recorded path
	binPath := extractBinaryPath(val)
	if !strings.ContainsAny(binPath, `\/`) {
		// Bare executable name (used for WindowsApps aliases) — resolve via PATH.
		if resolved, err := exec.LookPath(binPath); err == nil {
			binPath = resolved
		}
	}
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Printf("Autostart: disabled (binary not found at %s)\n", binPath)
		return nil
	}

	fmt.Println("Autostart: enabled (registry Run key set)")
	fmt.Printf("  Registry: HKCU\\%s\\%s\n", registryRunKey, registryValue)
	fmt.Printf("  Value: %s\n", val)

	// Surface whether the server process is currently alive.
	if pid, err := GetPID(paths.PIDFile); err == nil && IsProcessRunning(pid) {
		fmt.Printf("  Server process: running (PID %d)\n", pid)
	} else {
		fmt.Println("  Server process: not running")
	}

	return nil
}

// extractBinaryPath pulls the executable path from the registry value string.
// The value is formatted as `"path" args...`
func extractBinaryPath(val string) string {
	if len(val) < 2 || val[0] != '"' {
		// No quotes — first space-separated token is the path
		for i, c := range val {
			if c == ' ' {
				return val[:i]
			}
		}
		return val
	}
	// Quoted path
	end := 1
	for end < len(val) {
		if val[end] == '"' && val[end-1] != '\\' {
			break
		}
		end++
	}
	return val[1:end]
}
