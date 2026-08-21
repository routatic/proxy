package config

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
)

// WatchConfig monitors the config file for changes and reloads it automatically.
// It watches the directory containing the config file (not the file itself) to
// handle editors that save by renaming/creating a new file. It also listens for
// SIGHUP to allow manual reload triggers on Unix systems.
func WatchConfig(ctx context.Context, atomic *AtomicConfig) error {
	return WatchConfigWithReady(ctx, atomic, nil)
}

// WatchConfigWithReady behaves exactly like WatchConfig, but additionally
// signals ready once the filesystem watch has been registered.
//
// The signal fires immediately after the watch on the config file's directory is
// established — that is, it means "changes from this point on will be observed".
// It does not mean a reload has happened, and it says nothing about the config
// having been re-read: no reload occurs until an actual file change arrives.
// Writes that complete before the signal may be missed entirely, so callers that
// need a change to be observed must wait for ready before writing.
//
// The send is non-blocking, so a nil or unbuffered-and-unread channel never
// stalls the watcher. Use a buffered channel (capacity 1) to receive it reliably.
func WatchConfigWithReady(ctx context.Context, atomic *AtomicConfig, ready chan<- struct{}) error {
	path := atomic.Path()
	absPath, err := filepath.Abs(path)
	if err != nil {
		slog.Error("failed to get absolute path", "error", err)
		return err
	}
	dir := filepath.Dir(absPath)
	filename := filepath.Base(absPath)

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		slog.Error("failed to create watcher", "error", err)
		return err
	}
	defer func() {
		_ = watcher.Close()
	}()

	if err := watcher.Add(dir); err != nil {
		return err
	}

	slog.Info("config watcher started", "path", absPath)

	// The watch is live: changes from here on will be observed.
	if ready != nil {
		select {
		case ready <- struct{}{}:
		default:
		}
	}

	// SIGHUP handler for manual reload triggers
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	defer signal.Stop(sighup)

	var debounceTimer *time.Timer
	defer func() {
		if debounceTimer != nil {
			debounceTimer.Stop()
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event, ok := <-watcher.Events:
			if !ok {
				return nil
			}
			// Only care about events for our specific config file
			if filepath.Base(event.Name) != filename {
				continue
			}
			// Filter for relevant event types
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
				continue
			}

			if debounceTimer != nil {
				debounceTimer.Stop()
			}
			debounceTimer = time.AfterFunc(500*time.Millisecond, func() {
				slog.Info("config file changed, reloading", "path", absPath)
				if err := atomic.Reload(); err != nil {
					slog.Error("config reload failed", "error", err)
				} else {
					slog.Info("config reloaded successfully")
				}
			})

		case err, ok := <-watcher.Errors:
			if !ok {
				return nil
			}
			slog.Error("config watcher error", "error", err)

		case <-sighup:
			slog.Info("received SIGHUP, reloading config")
			if err := atomic.Reload(); err != nil {
				slog.Error("config reload failed", "error", err)
			} else {
				slog.Info("config reloaded successfully")
			}
		}
	}
}
