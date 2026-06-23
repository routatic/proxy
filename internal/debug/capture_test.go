package debug

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCaptureLoggerCreation(t *testing.T) {
	dir := t.TempDir()
	config := CaptureConfig{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
		MaxSize:   1024 * 1024,
	}

	logger, err := NewCaptureLogger(config)
	if err != nil {
		t.Fatalf("NewCaptureLogger() error = %v", err)
	}
	defer logger.Close()

	if logger == nil {
		t.Fatal("expected logger to be created")
	}

	if !logger.Enabled() {
		t.Error("expected logger to be enabled")
	}
}

func TestCaptureMethodsAsync(t *testing.T) {
	dir := t.TempDir()
	config := CaptureConfig{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
		MaxSize:   1024 * 1024,
	}

	logger, err := NewCaptureLogger(config)
	if err != nil {
		t.Fatalf("NewCaptureLogger() error = %v", err)
	}
	defer logger.Close()

	// Test CaptureRequest
	requestData := json.RawMessage(`{"model": "test-model", "messages": [{"role": "user", "content": "hello"}]}`)
	logger.CaptureRequest("test-provider", requestData)

	// Test CaptureResponse
	responseData := json.RawMessage(`{"choices": [{"message": {"content": "hi"}}]}`)
	logger.CaptureResponse("test-provider", responseData)

	// Test CaptureTransform
	transformData := json.RawMessage(`{"transformed": true}`)
	logger.CaptureTransform("test-provider", transformData)

	// Give async operations time to complete
	time.Sleep(100 * time.Millisecond)

	// Flush to ensure writes are complete
	logger.Flush()

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
	config := CaptureConfig{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
		MaxSize:   1024 * 1024,
	}

	logger, err := NewCaptureLogger(config)
	if err != nil {
		t.Fatalf("NewCaptureLogger() error = %v", err)
	}
	defer logger.Close()

	// Capture with different providers
	providers := []string{"opencode-go", "opencode-zen", "aws-bedrock"}
	for _, provider := range providers {
		data := json.RawMessage(`{"test": "data"}`)
		logger.CaptureRequest(provider, data)
	}

	// Give async operations time to complete
	time.Sleep(100 * time.Millisecond)
	logger.Flush()

	// Read and verify provider tags
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

func TestCaptureDisabled(t *testing.T) {
	dir := t.TempDir()
	config := CaptureConfig{
		Enabled:   false,
		Directory: dir,
		MaxFiles:  10,
		MaxSize:   1024 * 1024,
	}

	logger, err := NewCaptureLogger(config)
	if err != nil {
		t.Fatalf("NewCaptureLogger() error = %v", err)
	}
	defer logger.Close()

	if logger.Enabled() {
		t.Error("expected logger to be disabled")
	}

	// Capture should be no-op when disabled
	data := json.RawMessage(`{"test": "data"}`)
	logger.CaptureRequest("test-provider", data)
	logger.CaptureResponse("test-provider", data)
	logger.CaptureTransform("test-provider", data)

	// Flush and check no files created
	logger.Flush()

	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read directory: %v", err)
	}

	if len(files) > 0 {
		t.Error("expected no files when capture is disabled")
	}
}

func TestCloseFlushesPending(t *testing.T) {
	dir := t.TempDir()
	config := CaptureConfig{
		Enabled:   true,
		Directory: dir,
		MaxFiles:  10,
		MaxSize:   1024 * 1024,
	}

	logger, err := NewCaptureLogger(config)
	if err != nil {
		t.Fatalf("NewCaptureLogger() error = %v", err)
	}

	// Capture multiple entries
	for i := 0; i < 5; i++ {
		data := json.RawMessage(`{"test": "data"}`)
		logger.CaptureRequest("test-provider", data)
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
