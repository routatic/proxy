package main

import (
	"fmt"

	"github.com/routatic/proxy/internal/update"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update [check]",
	Short: "Update routatic-proxy to the latest version",
	Long: `Download and install the latest version of routatic-proxy.

The update command respects your configured update channel (stable or beta).
Use 'routatic-proxy update-channel' to switch between channels.

Examples:
  routatic-proxy update         # Download and install latest version
  routatic-proxy update check   # Check for updates without installing`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		channel, err := update.GetChannel()
		if err != nil {
			return fmt.Errorf("failed to get update channel: %w", err)
		}
		fmt.Printf("Checking for updates on %s channel...\n", channel)

		// Get current version (from build info or embedded)
		currentVersion := version // from main.go
		if currentVersion == "" {
			currentVersion = "dev"
		}

		release, err := update.GetLatestRelease(string(channel))
		if err != nil {
			return fmt.Errorf("failed to check for updates: %w", err)
		}

		// Check if update is available
		if release.TagName == currentVersion || release.TagName <= currentVersion {
			fmt.Printf("You are already on the latest version (%s).\n", currentVersion)
			return nil
		}

		fmt.Printf("New version available: %s (current: %s)\n", release.TagName, currentVersion)

		// If just checking, stop here
		if len(args) > 0 && args[0] == "check" {
			fmt.Println("Run 'routatic-proxy update' to install.")
			return nil
		}

		// Get download URL for current platform
		url, filename, err := update.GetAssetURL(release)
		if err != nil {
			return fmt.Errorf("failed to find download for your platform: %w", err)
		}

		fmt.Printf("Downloading %s...\n", filename)

		// Download and install
		if err := update.DownloadAndInstall(url, filename); err != nil {
			return fmt.Errorf("failed to install update: %w", err)
		}

		fmt.Printf("Successfully updated to %s!\n", release.TagName)
		fmt.Println("Please restart routatic-proxy to use the new version.")

		return nil
	},
}

// TODO: wire updateCmd into rootCmd when root command registration is centralized.
// func init() {
// 	rootCmd.AddCommand(updateCmd)
// }
