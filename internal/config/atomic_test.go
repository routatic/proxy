package config

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestAtomicConfig_Get(t *testing.T) {
	cfg := &Config{APIKey: "test-key"}
	atomic := NewAtomicConfig(cfg, "/tmp/config.json")

	got := atomic.Get()
	if got.APIKey != "test-key" {
		t.Errorf("Get().APIKey = %q, want %q", got.APIKey, "test-key")
	}
}

func TestAtomicConfig_Reload(t *testing.T) {
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initialJSON := `{"api_key": "initial-key"}`
	if err := os.WriteFile(path, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	atomic := NewAtomicConfig(cfg, path)
	if atomic.Get().APIKey != "initial-key" {
		t.Fatalf("initial APIKey mismatch: got %q", atomic.Get().APIKey)
	}

	// Update file on disk
	updatedJSON := `{"api_key": "updated-key"}`
	if err := os.WriteFile(path, []byte(updatedJSON), 0644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	if err := atomic.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if atomic.Get().APIKey != "updated-key" {
		t.Errorf("after Reload, APIKey = %q, want %q", atomic.Get().APIKey, "updated-key")
	}
}

func TestAtomicConfig_Reload_PreservesOldOnError(t *testing.T) {
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initialJSON := `{"api_key": "initial-key"}`
	if err := os.WriteFile(path, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	atomic := NewAtomicConfig(cfg, path)

	// Write invalid JSON
	if err := os.WriteFile(path, []byte("not-json"), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	if err := atomic.Reload(); err == nil {
		t.Fatal("Reload() expected error for invalid JSON, got nil")
	}

	// Old config should be preserved
	if atomic.Get().APIKey != "initial-key" {
		t.Errorf("after failed Reload, APIKey = %q, want %q", atomic.Get().APIKey, "initial-key")
	}
}

func TestAtomicConfig_OnReload(t *testing.T) {
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initialJSON := `{"api_key": "initial-key"}`
	if err := os.WriteFile(path, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	atomic := NewAtomicConfig(cfg, path)

	callbackCalled := make(chan *Config, 1)
	atomic.OnReload(func(newCfg *Config) {
		callbackCalled <- newCfg
	})

	updatedJSON := `{"api_key": "updated-key"}`
	if err := os.WriteFile(path, []byte(updatedJSON), 0644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	if err := atomic.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	select {
	case newCfg := <-callbackCalled:
		if newCfg.APIKey != "updated-key" {
			t.Errorf("callback received APIKey = %q, want %q", newCfg.APIKey, "updated-key")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnReload callback was not invoked within timeout")
	}
}

func TestAtomicConfig_OnReload_MultipleCallbacks(t *testing.T) {
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initialJSON := `{"api_key": "initial-key"}`
	if err := os.WriteFile(path, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	atomic := NewAtomicConfig(cfg, path)

	var callCount int
	atomic.OnReload(func(_ *Config) {
		callCount++
	})
	atomic.OnReload(func(_ *Config) {
		callCount++
	})

	updatedJSON := `{"api_key": "updated-key"}`
	if err := os.WriteFile(path, []byte(updatedJSON), 0644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	if err := atomic.Reload(); err != nil {
		t.Fatalf("Reload() error = %v", err)
	}

	if callCount != 2 {
		t.Errorf("callback call count = %d, want 2", callCount)
	}
}

func TestAtomicConfig_ConcurrentGetAndReload(t *testing.T) {
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initialJSON := `{"api_key": "initial-key"}`
	if err := os.WriteFile(path, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	at := NewAtomicConfig(cfg, path)

	var wg sync.WaitGroup
	done := make(chan struct{})

	// Spawn readers.
	for range 10 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
					_ = at.Get()
				}
			}
		})
	}

	// Writer: reload repeatedly.
	wg.Go(func() {
		for {
			select {
			case <-done:
				return
			default:
				_ = at.Reload()
			}
		}
	})

	// Let it race for a bit.
	time.Sleep(500 * time.Millisecond)
	close(done)
	wg.Wait()
}
