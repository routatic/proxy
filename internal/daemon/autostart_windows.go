//go:build windows

package daemon

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	registryRunKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	registryValue  = LaunchAgent
)

func buildAutostartArgs(configPath string, port int) string {
	args := `"serve" "--background"`
	if configPath != "" {
		args += ` "--config" "` + configPath + `"`
	}
	if port != 0 {
		args += ` "--port" "` + fmt.Sprintf("%d", port) + `"`
	}
	return args
}

// EnableAutostart adds a registry Run key so oc-go-cc starts on login.
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

	value := `"` + paths.BinaryPath + `" ` + buildAutostartArgs(configPath, port)
	if err := key.SetStringValue(registryValue, value); err != nil {
		return fmt.Errorf("cannot set registry value: %w", err)
	}

	fmt.Printf("Autostart enabled. %s will start on login.\n", AppName)
	fmt.Printf("  Registry: HKCU\\%s\\%s\n", registryRunKey, registryValue)
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
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		fmt.Printf("Autostart: disabled (binary not found at %s)\n", binPath)
		return nil
	}

	fmt.Println("Autostart: enabled (registry Run key set)")
	fmt.Printf("  Registry: HKCU\\%s\\%s\n", registryRunKey, registryValue)
	fmt.Printf("  Value: %s\n", val)
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
