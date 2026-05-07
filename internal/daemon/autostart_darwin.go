//go:build darwin

package daemon

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"text/template"
)

const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Label}}</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.BinaryPath}}</string>
        <string>serve</string>
        <string>--background</string>
        {{- if .ConfigPath}}
        <string>--config</string>
        <string>{{.ConfigPath}}</string>
        {{- end}}
        {{- if .Port}}
        <string>--port</string>
        <string>{{.Port}}</string>
        {{- end}}
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <dict>
        <key>SuccessfulExit</key>
        <false/>
    </dict>

    <key>StandardOutPath</key>
    <string>{{.LogFile}}</string>

    <key>StandardErrorPath</key>
    <string>{{.LogFile}}</string>

    <key>EnvironmentVariables</key>
    <dict>
        <key>PATH</key>
        <string>{{.EnvPath}}</string>
    </dict>
</dict>
</plist>
`

// plistData holds the values interpolated into the launchd plist template.
type plistData struct {
	Label      string
	BinaryPath string
	ConfigPath string
	Port       int
	LogFile    string
	EnvPath    string
}

// EnableAutostart creates the launchd plist and loads it.
func EnableAutostart(configPath string, port int) error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}
	if err := paths.EnsureConfigDir(); err != nil {
		return err
	}

	// Ensure LaunchAgents directory exists
	launchAgentsDir := filepath.Dir(paths.PlistPath)
	if err := os.MkdirAll(launchAgentsDir, 0755); err != nil {
		return fmt.Errorf("cannot create LaunchAgents directory: %w", err)
	}

	envPath := os.Getenv("PATH")
	if envPath == "" {
		envPath = "/usr/local/bin:/usr/bin:/bin"
	}

	data := plistData{
		Label:      LaunchAgent,
		BinaryPath: paths.BinaryPath,
		ConfigPath: configPath,
		Port:       port,
		LogFile:    paths.LogFile,
		EnvPath:    envPath,
	}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		return fmt.Errorf("cannot parse plist template: %w", err)
	}

	f, err := os.Create(paths.PlistPath)
	if err != nil {
		return fmt.Errorf("cannot create plist file: %w", err)
	}
	defer func() { _ = f.Close() }()

	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("cannot render plist: %w", err)
	}

	fmt.Printf("Autostart enabled. %s will start on login.\n", AppName)
	fmt.Printf("  Plist: %s\n", paths.PlistPath)

	if err := loadPlist(paths.PlistPath); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not load plist with launchctl: %v\n", err)
		fmt.Fprintf(os.Stderr, "The plist is installed and will load on next login.\n")
	} else {
		fmt.Println("Service loaded successfully.")
	}

	return nil
}

// DisableAutostart unloads and removes the launchd plist.
func DisableAutostart() error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}

	if _, err := os.Stat(paths.PlistPath); os.IsNotExist(err) {
		fmt.Println("Autostart is not enabled (no plist found)")
		return nil
	}

	if err := unloadPlist(paths.PlistPath); err != nil {
		fmt.Fprintf(os.Stderr, "note: could not unload plist: %v\n", err)
	}

	if err := os.Remove(paths.PlistPath); err != nil {
		return fmt.Errorf("cannot remove plist: %w", err)
	}

	fmt.Printf("Autostart disabled. Plist removed.\n")
	return nil
}

// AutostartStatus reports whether autostart is enabled.
func AutostartStatus() error {
	paths, err := DefaultPaths()
	if err != nil {
		return err
	}

	plistExists := false
	if _, err := os.Stat(paths.PlistPath); err == nil {
		plistExists = true
	}

	if !plistExists {
		fmt.Println("Autostart: disabled (no plist found)")
		return nil
	}

	loaded := isPlistLoaded()

	if loaded {
		fmt.Println("Autostart: enabled (plist installed and loaded)")
	} else {
		fmt.Println("Autostart: enabled (plist installed, not currently loaded)")
	}
	fmt.Printf("  Plist: %s\n", paths.PlistPath)
	return nil
}

func loadPlist(plistPath string) error {
	uid := strconv.Itoa(os.Getuid())
	target := "gui/" + uid + "/" + LaunchAgent

	_ = exec.Command("launchctl", "bootout", target).Run()
	return exec.Command("launchctl", "bootstrap", target, plistPath).Run()
}

func unloadPlist(plistPath string) error {
	uid := strconv.Itoa(os.Getuid())
	target := "gui/" + uid + "/" + LaunchAgent
	return exec.Command("launchctl", "bootout", target).Run()
}

func isPlistLoaded() bool {
	err := exec.Command("launchctl", "list", LaunchAgent).Run()
	return err == nil
}
