package debug

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
)

func TestCaptureLoggerCreation(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	storage, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	logger := NewCaptureLogger(storage, true)
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
	defer func() { _ = logger.Close() }()

	if !logger.enabled {
		t.Error("expected logger to be enabled")
	}
}

func TestCaptureLoggerDisabled(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	storage, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	logger := NewCaptureLogger(storage, false)
	if logger != nil {
		t.Error("expected nil logger when disabled")
		_ = logger.Close()
	}
}

func TestCaptureMethodsAsync(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	storage, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	logger := NewCaptureLogger(storage, true)
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
	defer func() { _ = logger.Close() }()

	// Test CaptureOriginal
	requestData := []byte(`{"model": "test-model", "messages": [{"role": "user", "content": "hello"}]}`)
	logger.CaptureOriginal("req-123", requestData)

	// Test CaptureNormalized
	logger.CaptureNormalized("req-123", "opencode-go", requestData)

	// Test CaptureUpstreamRequest
	logger.CaptureUpstreamRequest("req-123", "opencode-go", requestData)

	// Test CaptureUpstreamResponse
	responseData := []byte(`{"choices": [{"message": {"content": "hi"}}]}`)
	logger.CaptureUpstreamResponse("req-123", "opencode-go", responseData)

	// Test CaptureTransformed
	logger.CaptureTransformed("req-123", "opencode-go", responseData)

	// Give async operations time to complete
	time.Sleep(100 * time.Millisecond)

	// Close to ensure writes are complete
	_ = logger.Close()

	// Verify files were created
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) == 0 {
		t.Error("expected capture files to be created")
	}
}

func TestProviderTagging(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	storage, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	logger := NewCaptureLogger(storage, true)
	if logger == nil {
		t.Fatal("expected logger to be created")
	}
	defer func() { _ = logger.Close() }()

	// Capture with different providers
	providers := []string{"opencode-go", "opencode-zen", "aws-bedrock"}
	for _, provider := range providers {
		data := []byte(`{"test": "data"}`)
		logger.CaptureNormalized("req-"+provider, provider, data)
	}

	// Give async operations time to complete
	time.Sleep(100 * time.Millisecond)
	_ = logger.Close()

	// Verify files were created
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) == 0 {
		t.Fatal("expected capture files to be created")
	}

	// Read the first file and verify provider field
	content, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var entry CaptureEntry
	if err := json.Unmarshal(content, &entry); err != nil {
		t.Fatalf("failed to unmarshal entry: %v", err)
	}

	if entry.Provider == "" {
		t.Error("expected provider to be tagged")
	}
}

func TestCaptureWithNilLogger(t *testing.T) {
	var logger *CaptureLogger

	// These should not panic when logger is nil
	logger.CaptureOriginal("req-123", []byte(`{"test": "data"}`))
	logger.CaptureNormalized("req-123", "test-provider", []byte(`{"test": "data"}`))
	logger.CaptureUpstreamRequest("req-123", "test-provider", []byte(`{"test": "data"}`))
	logger.CaptureUpstreamResponse("req-123", "test-provider", []byte(`{"test": "data"}`))
	logger.CaptureTransformed("req-123", "test-provider", []byte(`{"test": "data"}`))

	// Close should not panic when logger is nil
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestCloseFlushesPending(t *testing.T) {
	dir := t.TempDir()
	cfg := config.DebugCapture{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
	}

	storage, err := NewStorage(cfg)
	if err != nil {
		t.Fatalf("NewStorage() error = %v", err)
	}
	defer func() { _ = storage.Close() }()

	logger := NewCaptureLogger(storage, true)
	if logger == nil {
		t.Fatal("expected logger to be created")
	}

	// Capture multiple entries
	for i := 0; i < 5; i++ {
		data := []byte(`{"test": "data"}`)
		logger.CaptureNormalized("req-"+string(rune('0'+i)), "test-provider", data)
	}

	// Close should flush pending entries
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	// Verify files were created
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) == 0 {
		t.Error("expected capture files to be created after close")
	}

	// Verify entries are valid JSONL
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(dir, file.Name()))
		if err != nil {
			t.Fatalf("failed to read file: %v", err)
		}

		var entry CaptureEntry
		if err := json.Unmarshal(content, &entry); err != nil {
			t.Errorf("expected valid JSONL entry in %s: %v", file.Name(), err)
		}
	}
}

func TestRedactIfNeeded(t *testing.T) {
	tests := []struct {
		name          string
		data          []byte
		redactEnabled bool
		wantRedacted  bool
	}{
		{
			name:          "redaction disabled returns original",
			data:          []byte(`{"api_key": "secret123"}`),
			redactEnabled: false,
			wantRedacted:  false,
		},
		{
			name:          "redaction enabled redacts api_key",
			data:          []byte(`{"api_key": "secret123"}`),
			redactEnabled: true,
			wantRedacted:  true,
		},
		{
			name:          "redaction enabled redacts authorization",
			data:          []byte(`{"authorization": "Bearer token123"}`),
			redactEnabled: true,
			wantRedacted:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactIfNeeded(tt.data, tt.redactEnabled)

			if tt.wantRedacted {
				if string(result) == string(tt.data) {
					t.Error("expected data to be redacted but it was unchanged")
				}
				if string(result) == string(tt.data) {
					t.Logf("result: %s", string(result))
				}
			} else {
				if string(result) != string(tt.data) {
					t.Errorf("expected data to be unchanged, got %s", string(result))
				}
			}
		})
	}
}
