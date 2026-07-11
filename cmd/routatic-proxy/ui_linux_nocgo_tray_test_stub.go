//go:build nocgo_tray

package main

// This file is a test stub for Linux UI when running tests without CGO
// and without the tray dependency. It provides a no-op implementation
// for the UI functions used in tests.

import (
	"log/slog"
)

func showMainWindow(configPath string, port int) {
	slog.Info("UI not available in nocgo_tray mode")
}

func handleConfigSaved() {
	// No-op in stub mode
}
