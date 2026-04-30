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

func TestIsProcessRunning_CurrentProcess(t *testing.T) {
	if !IsProcessRunning(os.Getpid()) {
		t.Error("current process should be reported as running")
	}
}

func TestIsProcessRunning_NonexistentPID(t *testing.T) {
	// PID 1 is typically init — but on some systems it may not exist.
	// Use an almost-certainly-invalid PID instead.
	if IsProcessRunning(99999999) {
		t.Error("non-existent PID should not be reported as running")
	}
}
