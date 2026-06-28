//go:build !darwin || !cgo

package main

import "github.com/spf13/cobra"

func addPlatformCommands(rootCmd *cobra.Command) {
	// No-op for non-macOS platforms
}

func setupDefaultCommand() {
	// No-op for non-macOS platforms
}
