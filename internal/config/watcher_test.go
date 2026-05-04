package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchConfig_DetectsFileChange(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	initialJSON := `{"api_key": "watcher-test"}`
	if err := os.WriteFile(path, []byte(initialJSON), 0644); err != nil {
		t.Fatalf("failed to write initial config: %v", err)
	}

	cfg, err := LoadFromPath(path)
	if err != nil {
		t.Fatalf("LoadFromPath failed: %v", err)
	}

	atomic := NewAtomicConfig(cfg, path)

	// Start watcher in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		if err := WatchConfig(ctx, atomic); err != nil && err != context.Canceled {
			t.Logf("WatchConfig returned: %v", err)
		}
	}()

	// Give watcher time to set up
	time.Sleep(200 * time.Millisecond)

	// Modify config file
	updatedJSON := `{"api_key": "watcher-updated"}`
	if err := os.WriteFile(path, []byte(updatedJSON), 0644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	// Wait for reload with timeout
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if atomic.Get().APIKey == "watcher-updated" {
			return // success
		}
		time.Sleep(50 * time.Millisecond)
	}

	t.Errorf("config was not reloaded after file change, got APIKey = %q", atomic.Get().APIKey)
}
