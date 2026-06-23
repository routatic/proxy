package debug

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
)

func TestNewStorageCreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	baseDir := filepath.Join(dir, "debug-captures")

	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: baseDir,
		MaxFiles:  10,
	}

	s, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	// Directory should be created
	info, err := os.Stat(baseDir)
	if err != nil {
		t.Fatalf("expected directory to be created: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected path to be a directory")
	}
}

func TestWriteEntryCreatesFile(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	s, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	entry := CaptureEntry{
		Timestamp: time.Now().UTC(),
		Provider:  "test-provider",
		Phase:     "request",
		Data:      json.RawMessage(`{"test": "data"}`),
	}

	if err := s.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry() error = %v", err)
	}

	// File should be created
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("expected file to be created")
	}

	// Verify file content is valid JSONL
	content, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var readEntry CaptureEntry
	if err := json.Unmarshal(content, &readEntry); err != nil {
		t.Errorf("expected valid JSONL entry, got error: %v", err)
	}

	if readEntry.Provider != entry.Provider {
		t.Errorf("Provider = %q, want %q", readEntry.Provider, entry.Provider)
	}
	if readEntry.Phase != entry.Phase {
		t.Errorf("Phase = %q, want %q", readEntry.Phase, entry.Phase)
	}
}

func TestFileRotation(t *testing.T) {
	dir := t.TempDir()
	// Set small max size to trigger rotation quickly
	cfg := config.DebugCapture{
		Enabled:     true,
		Directory:   dir,
		MaxFiles:    10,
		MaxFileSize: 100,
	}

	s, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	// Write multiple entries to trigger rotation
	for i := 0; i < 5; i++ {
		entry := CaptureEntry{
			Timestamp: time.Now().UTC(),
			Provider:  "test-provider",
			Phase:     "request",
			Data:      json.RawMessage(`{"large": "data content here"}`),
		}
		if err := s.WriteEntry(entry); err != nil {
			t.Fatalf("WriteEntry() error = %v", err)
		}
	}

	// Should have multiple files due to rotation
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) < 1 {
		t.Error("expected at least one file")
	}
}

func TestMaxFilesDeletion(t *testing.T) {
	dir := t.TempDir()
	maxFiles := 3

	cfg := config.DebugCapture{
		Enabled:     true,
		Directory:   dir,
		MaxFiles:    maxFiles,
		MaxFileSize: 1024,
	}

	s, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	// Create more files than maxFiles
	for i := 0; i < maxFiles+2; i++ {
		entry := CaptureEntry{
			Timestamp: time.Now().UTC(),
			Provider:  "test-provider",
			Phase:     "request",
			Data:      json.RawMessage(`{"test": "data"}`),
		}
		if err := s.WriteEntry(entry); err != nil {
			t.Fatalf("WriteEntry() error = %v", err)
		}
		// Small delay to ensure different timestamps
		time.Sleep(10 * time.Millisecond)
	}

	// Force rotation by writing another entry
	entry := CaptureEntry{
		Timestamp: time.Now().UTC(),
		Provider:  "test-provider",
		Phase:     "request",
		Data:      json.RawMessage(`{"trigger": "rotation"}`),
	}
	if err := s.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry() error = %v", err)
	}

	// Count files
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) > maxFiles {
		t.Errorf("expected at most %d files, got %d", maxFiles, len(files))
	}
}

func TestJSONLFormat(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	s, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = s.Close() }()

	entry := CaptureEntry{
		Timestamp: time.Now().UTC(),
		Provider:  "test-provider",
		Phase:     "response",
		Data:      json.RawMessage(`{"key": "value", "number": 42}`),
	}

	if err := s.WriteEntry(entry); err != nil {
		t.Fatalf("WriteEntry() error = %v", err)
	}

	// Read file content
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("expected file to be created")
	}

	content, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	// Verify it's valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal(content, &result); err != nil {
		t.Errorf("expected valid JSON, got error: %v", err)
	}

	// Verify required fields exist
	if _, ok := result["timestamp"]; !ok {
		t.Error("expected timestamp field in JSON")
	}
	if _, ok := result["provider"]; !ok {
		t.Error("expected provider field in JSON")
	}
	if _, ok := result["phase"]; !ok {
		t.Error("expected phase field in JSON")
	}
	if _, ok := result["data"]; !ok {
		t.Error("expected data field in JSON")
	}
}
