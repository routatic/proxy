package main

import (
	"fmt"
	"strings"

	"github.com/routatic/proxy/internal/daemon"
	"github.com/routatic/proxy/internal/updater"
	"github.com/spf13/cobra"
)

// updateCmd returns the Cobra command that updates routatic-proxy to the
// latest GitHub release.
func updateCmd() *cobra.Command {
	var (
		checkOnly    bool
		yes          bool
		force        bool
		skipChecksum bool
	)

	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update routatic-proxy to the latest release",
		Long: `Check GitHub for the latest routatic-proxy release and, if a newer
version is available, download the matching asset for this OS/arch,
verify its SHA256 checksum, and replace the running binary in place.

A .old backup of the previous binary is written next to the running
executable on every platform. On Windows the backup is scheduled for
deletion after the process exits because the running executable is
locked until then.

If the current binary reports its version as "dev" (e.g. when built
from source without a version tag) the command refuses to update
unless --force is passed.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			info, err := updater.Check(ctx)
			if err != nil {
				return err
			}

			if checkOnly {
				needs, err := updater.NeedsUpdate(version, info.TagName, false)
				if err != nil {
					return err
				}
				if needs {
					fmt.Printf("Update available: %s -> %s\n", version, info.TagName)
				} else {
					fmt.Printf("Already up to date (%s)\n", version)
				}
				return nil
			}

			needs, err := updater.NeedsUpdate(version, info.TagName, force)
			if err != nil {
				return err
			}
			if !needs {
				fmt.Printf("Already up to date (%s)\n", version)
				return nil
			}

			if !yes {
				fmt.Printf("Update %s -> %s? [y/N] ", version, info.TagName)
				var resp string
				if _, err := fmt.Scanln(&resp); err != nil {
					return fmt.Errorf("aborted")
				}
				if strings.ToLower(strings.TrimSpace(resp)) != "y" {
					return fmt.Errorf("update cancelled")
				}
			}

			currentPath, err := daemon.FindBinary()
			if err != nil {
				return fmt.Errorf("cannot locate current binary: %w", err)
			}

			result, err := updater.Apply(ctx, updater.Options{
				CurrentVersion:    version,
				CurrentBinaryPath: currentPath,
				Force:             force,
				SkipChecksum:      skipChecksum,
			})
			if err != nil {
				return err
			}

			if !result.Updated {
				fmt.Printf("Already up to date (%s)\n", version)
				return nil
			}

			fmt.Printf("Updated %s -> %s\n", result.OldVersion, result.NewVersion)
			fmt.Printf("New binary: %s\n", result.NewPath)
			if result.BackupPath != "" {
				fmt.Printf("Backup: %s\n", result.BackupPath)
			}
			return nil
		},
	}

	cmd.Flags().BoolVarP(&checkOnly, "check", "c", false, "Only check for updates; do not install")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip the confirmation prompt")
	cmd.Flags().BoolVarP(&force, "force", "f", false, "Update even if already on the latest version (required when current version is 'dev')")
	cmd.Flags().BoolVar(&skipChecksum, "skip-checksum", false, "Skip SHA256 checksum verification of the downloaded asset")

	return cmd
}
