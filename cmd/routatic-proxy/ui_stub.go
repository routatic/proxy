//go:build !darwin && !linux

package main

import "github.com/spf13/cobra"

func addPlatformCommands(rootCmd *cobra.Command) {
	// No-op for non-macOS, non-linux platforms
}

func setupDefaultCommand() {
	// No-op for non-macOS, non-linux platforms
}
