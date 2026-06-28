//go:build windows

package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuildAutostartArgs(t *testing.T) {
	tests := []struct {
		name       string
		configPath string
		port       int
		want       string
	}{
		{
			name: "no options",
			want: "serve --background",
		},
		{
			name:       "config only",
			configPath: `C:\Users\Me\.config\routatic-proxy\config.json`,
			want:       `serve --background --config "C:\Users\Me\.config\routatic-proxy\config.json"`,
		},
		{
			name: "port only",
			port: 8080,
			want: "serve --background --port 8080",
		},
		{
			name:       "config and port",
			configPath: `C:\Program Files\routatic-proxy\config.json`,
			port:       9090,
			want:       `serve --background --config "C:\Program Files\routatic-proxy\config.json" --port 9090`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAutostartArgs(tt.configPath, tt.port)
			if got != tt.want {
				t.Errorf("buildAutostartArgs() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRegistryBinaryRef(t *testing.T) {
	tests := []struct {
		name       string
		binaryPath string
		want       string
	}{
		{
			name:       "normal absolute path with spaces",
			binaryPath: `C:\Program Files\routatic-proxy\routatic-proxy.exe`,
			want:       `"C:\Program Files\routatic-proxy\routatic-proxy.exe"`,
		},
		{
			name:       "windows apps alias lower case",
			binaryPath: `C:\Users\Me\AppData\Local\Microsoft\WindowsApps\routatic-proxy.exe`,
			want:       `routatic-proxy.exe`,
		},
		{
			name:       "windows apps alias mixed case",
			binaryPath: `C:\Users\Me\AppData\Local\Microsoft\windowsapps\routatic-proxy.exe`,
			want:       `routatic-proxy.exe`,
		},
		{
			name:       "path without spaces",
			binaryPath: `C:\tools\routatic-proxy.exe`,
			want:       `"C:\tools\routatic-proxy.exe"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := registryBinaryRef(tt.binaryPath)
			if got != tt.want {
				t.Errorf("registryBinaryRef(%q) = %q, want %q", tt.binaryPath, got, tt.want)
			}
		})
	}
}

func TestExtractBinaryPath(t *testing.T) {
	tests := []struct {
		name string
		val  string
		want string
	}{
		{
			name: "quoted path with args",
			val:  `"C:\tools\routatic-proxy.exe" serve --background`,
			want: `C:\tools\routatic-proxy.exe`,
		},
		{
			name: "quoted path with spaces",
			val:  `"C:\Program Files\routatic-proxy\routatic-proxy.exe" serve --background`,
			want: `C:\Program Files\routatic-proxy\routatic-proxy.exe`,
		},
		{
			name: "unquoted path with args",
			val:  `C:\tools\routatic-proxy.exe serve --background`,
			want: `C:\tools\routatic-proxy.exe`,
		},
		{
			name: "bare name with args",
			val:  `routatic-proxy.exe serve --background`,
			want: `routatic-proxy.exe`,
		},
		{
			name: "only path quoted",
			val:  `"C:\tools\routatic-proxy.exe"`,
			want: `C:\tools\routatic-proxy.exe`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractBinaryPath(tt.val)
			if got != tt.want {
				t.Errorf("extractBinaryPath(%q) = %q, want %q", tt.val, got, tt.want)
			}
		})
	}
}

func TestValidateBinaryRef(t *testing.T) {
	execPath, err := os.Executable()
	if err != nil {
		t.Skipf("cannot determine executable: %v", err)
	}
	execPath = resolveExecutablePath(execPath)

	if runtime.GOOS != "windows" {
		t.Skip("Windows-only test")
	}

	// Absolute/quoted path should pass.
	if err := validateBinaryRef(`"` + execPath + `"`); err != nil {
		t.Errorf("validateBinaryRef(absolute) unexpected error: %v", err)
	}

	// Bare name of the current binary is usually not on PATH, so expect a lookup error.
	base := filepath.Base(execPath)
	if err := validateBinaryRef(base); err == nil {
		t.Logf("validateBinaryRef(%q) succeeded; test binary is on PATH", base)
	} else if !strings.Contains(err.Error(), "cannot find") {
		t.Errorf("validateBinaryRef(bare) unexpected error: %v", err)
	}

	// Non-existent quoted path should fail.
	if err := validateBinaryRef(`"C:\nonexistent\routatic-proxy.exe"`); err == nil {
		t.Error("validateBinaryRef(non-existent) expected error, got nil")
	} else if !strings.Contains(err.Error(), "cannot access binary") {
		t.Errorf("validateBinaryRef(non-existent) unexpected error: %v", err)
	}
}
