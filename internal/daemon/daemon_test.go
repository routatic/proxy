package daemon

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWritePIDAndGetPID_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	pidPath := filepath.Join(dir, "test.pid")

	expected := 12345
	if err := WritePID(pidPath, expected); err != nil {
		t.Fatalf("WritePID: %v", err)
	}

	got, err := GetPID(pidPath)
	if err != nil {
		t.Fatalf("GetPID: %v", err)
	}
	if got != expected {
		t.Errorf("GetPID = %d, want %d", got, expected)
	}
}

func TestGetPID_MissingFile(t *testing.T) {
	_, err := GetPID(filepath.Join(t.TempDir(), "nonexistent.pid"))
	if err == nil {
		t.Error("GetPID on missing file should return error")
	}
}

func TestGetPID_InvalidContent(t *testing.T) {
	pidPath := filepath.Join(t.TempDir(), "bad.pid")
	if err := os.WriteFile(pidPath, []byte("not-a-number"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := GetPID(pidPath)
	if err == nil {
		t.Error("GetPID on invalid content should return error")
	}
}

func TestResolveExecutablePath_CurrentBinary(t *testing.T) {
	execPath, err := os.Executable()
	if err != nil {
		t.Skipf("cannot determine executable: %v", err)
	}

	resolved := resolveExecutablePath(execPath)
	if resolved == "" {
		t.Error("resolveExecutablePath returned empty string")
	}

	// On Unix, resolved should either equal execPath or be a valid symlink target.
	// On Windows, it should return execPath unchanged.
	if runtime.GOOS == "windows" {
		if resolved != execPath {
			t.Errorf("on Windows, resolveExecutablePath should return input unchanged: got %q, want %q", resolved, execPath)
		}
	}
}

func TestResolveExecutablePath_HomebrewStable(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Homebrew stable path preservation is darwin-only")
	}
	cases := []struct {
		name  string
		input string
	}{
		{"opt Homebrew Apple Silicon", "/opt/homebrew/opt/routatic-proxy/bin/routatic-proxy"},
		{"bin Homebrew Apple Silicon", "/opt/homebrew/bin/routatic-proxy"},
		{"opt Homebrew Intel", "/usr/local/opt/routatic-proxy/bin/routatic-proxy"},
		{"bin Homebrew Intel", "/usr/local/bin/routatic-proxy"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveExecutablePath(tc.input)
			if got != tc.input {
				t.Errorf("resolveExecutablePath(%q) = %q, want preserved %q (darwin Homebrew stable path must not be resolved to Cellar)", tc.input, got, tc.input)
			}
			if got != "" && !isHomebrewStablePath(got) {
				t.Errorf("resolveExecutablePath(%q) = %q no longer looks like a Homebrew stable path", tc.input, got)
			}
		})
	}
}

func TestResolveExecutablePath_NonHomebrewSymlinkResolves(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("EvalSymlinks behavior is Windows-skipped")
	}
	dir := t.TempDir()
	real := filepath.Join(dir, "real-bin")
	link := filepath.Join(dir, "link-bin")
	if err := os.WriteFile(real, []byte("x"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	got := resolveExecutablePath(link)
	// Non-brew symlink should still be resolved via EvalSymlinks.
	want, _ := filepath.EvalSymlinks(link)
	if got != want {
		t.Errorf("resolveExecutablePath(%q) = %q, want resolved %q (non-brew symlinks must still resolve)", link, got, want)
	}
	if isHomebrewStablePath(link) {
		t.Errorf("test link %q must not be considered a Homebrew stable path", link)
	}
}

func TestIsHomebrewStablePath(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{
		{"/opt/homebrew/opt/routatic-proxy/bin/routatic-proxy", true},
		{"/opt/homebrew/bin/routatic-proxy", true},
		{"/usr/local/opt/routatic-proxy/bin/routatic-proxy", true},
		{"/usr/local/bin/routatic-proxy", true},
		{"/opt/homebrew/Cellar/routatic-proxy/0.6.4/bin/routatic-proxy", false},
		{"/Users/alec/git/proxy/bin/routatic-proxy", false},
		{"/tmp/link-bin", false},
		{"/home/linuxbrew/.linuxbrew/bin/routatic-proxy", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isHomebrewStablePath(tc.input)
		if got != tc.want {
			t.Errorf("isHomebrewStablePath(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestIsProcessRunning_CurrentProcess(t *testing.T) {
	if !IsProcessRunning(os.Getpid()) {
		t.Error("current process should be reported as running")
	}
}

func TestIsAppProcess_CurrentTestProcessIsNotOCGoCC(t *testing.T) {
	if IsAppProcess(os.Getpid(), AppName) {
		t.Errorf("current test process should not be reported as %s", AppName)
	}
}

func TestIsProcessRunning_NonexistentPID(t *testing.T) {
	// PID 1 is typically init — but on some systems it may not exist.
	// Use an almost-certainly-invalid PID instead.
	if IsProcessRunning(99999999) {
		t.Error("non-existent PID should not be reported as running")
	}
}
