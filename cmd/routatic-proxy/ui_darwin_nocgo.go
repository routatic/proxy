//go:build darwin && !cgo

package main

import (
	"github.com/spf13/cobra"
)

func addPlatformCommands(rootCmd *cobra.Command) {
	// UI not available without CGO on macOS
}

func setupDefaultCommand() {
	// No-op. Without CGO, we can't open a webview or tray on macOS.
}
