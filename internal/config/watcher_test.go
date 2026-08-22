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

	at := NewAtomicConfig(cfg, path)

	// Watch for reload via callback instead of polling. The callback receives the
	// freshly loaded config; AtomicConfig.Reload invokes callbacks *before* it
	// swaps the pointer in, so at.Get() is not guaranteed to be updated yet when
	// the callback fires. Assert on the config handed to the callback and wait
	// separately for the swap to become visible.
	reloaded := make(chan *Config, 1)
	at.OnReload(func(newCfg *Config) {
		select {
		case reloaded <- newCfg:
		default:
		}
	})

	// Start watcher in background. ready closes the gap that made this test flaky:
	// the fsnotify watch is registered asynchronously, so a write issued before
	// registration produces no event at all and is lost.
	ready := make(chan struct{}, 1)
	go func() {
		if err := WatchConfigWithReady(t.Context(), at, ready); err != nil && err != context.Canceled {
			t.Logf("WatchConfig returned: %v", err)
		}
	}()

	select {
	case <-ready:
	case <-time.After(10 * time.Second):
		t.Fatal("config watcher never became ready")
	}

	// The watch is live, so a single write is guaranteed to be observed.
	updatedJSON := `{"api_key": "watcher-updated"}`
	if err := os.WriteFile(path, []byte(updatedJSON), 0644); err != nil {
		t.Fatalf("failed to write updated config: %v", err)
	}

	select {
	case newCfg := <-reloaded:
		if newCfg.APIKey != "watcher-updated" {
			t.Errorf("reloaded config APIKey = %q, want %q", newCfg.APIKey, "watcher-updated")
		}
		waitForAPIKey(t, at, "watcher-updated")
	case <-time.After(10 * time.Second):
		t.Fatal("config was not reloaded after file change")
	}
}

// waitForAPIKey waits until the atomically published config exposes the expected
// API key. The pointer swap happens right after reload callbacks return, so this
// resolves almost immediately; the bound only guards against a missing swap.
func waitForAPIKey(t *testing.T, at *AtomicConfig, want string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for {
		got := at.Get().APIKey
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Errorf("after reload, APIKey = %q, want %q", got, want)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}
