package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/storage"
	"github.com/spf13/cobra"
)

// catalogSourceURL is the default models.dev catalog URL. It is a package
// variable so tests can override it with a local server endpoint.
var catalogSourceURL = "https://models.dev/catalog.json"

// catalogCmd returns the top-level catalog command.
func catalogCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "catalog",
		Short: "Manage the models.dev catalog cache",
		Long: `Download and cache the models.dev catalog locally.

The catalog is stored under ~/.config/routatic-proxy/catalog by default and is
used to resolve canonical model names and providers at runtime.`,
	}

	cmd.AddCommand(catalogSyncCmd())
	cmd.AddCommand(catalogExportCmd())
	cmd.PersistentFlags().StringP("config", "c", "", "Path to config file (used to locate the catalog directory)")

	return cmd
}

func catalogExportCmd() *cobra.Command {
	var outputPath string

	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export the SQLite catalog to JSON",
		Long: `Export the catalog from SQLite to a JSON file for backup or debugging.

The output file can be used as a backup or for manual inspection. 
By default exports to the catalog directory as catalog-export.json.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("read config flag: %w", err)
			}

			catalogDir := resolveCatalogDir(configPath)
			if outputPath == "" {
				outputPath = filepath.Join(catalogDir, "catalog-export.json")
			}

			db, err := storage.Open(storage.DefaultConfig)
			if err != nil {
				return fmt.Errorf("open database: %w", err)
			}
			defer func() { _ = db.Close() }()

			ctx := cmd.Context()
			if err := catalog.ExportJSON(ctx, db, outputPath); err != nil {
				return fmt.Errorf("export catalog: %w", err)
			}

			cmd.Printf("Catalog exported to %s\n", outputPath)
			return nil
		},
	}

	cmd.Flags().StringVarP(&outputPath, "output", "o", "", "Output file path (default: catalog/catalog-export.json)")

	return cmd
}

// catalogSyncCmd returns the command to sync the models.dev catalog.
func catalogSyncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Download and cache the models.dev catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("read config flag: %w", err)
			}

			catalogDir := resolveCatalogDir(configPath)
			lock, err := catalog.Sync(catalogSourceURL, catalogDir)
			if err != nil {
				return fmt.Errorf("catalog sync failed: %w", err)
			}

			cmd.Printf("Catalog synced to %s\n", catalogDir)
			cmd.Printf("  SHA256: %s\n", lock.SHA256)
			cmd.Printf("  Bytes:  %d\n", lock.Bytes)
			cmd.Printf("  TTL:    %d hours\n", lock.TTLHours)
			return nil
		},
	}
}

// resolveCatalogDir returns the directory where the catalog should be stored.
// If configPath is provided, the catalog is stored alongside that config file.
// Otherwise the default config directory is used.
func resolveCatalogDir(configPath string) string {
	if configPath != "" {
		return filepath.Join(filepath.Dir(configPath), "catalog")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".config", "routatic-proxy", "catalog")
	}
	return filepath.Join(home, ".config", "routatic-proxy", "catalog")
}

// ensureCatalogSynced checks whether the local models.dev catalog is present
// and fresh. If the lock file is missing, corrupted, or expired relative to the
// configured max_age_hours, it re-downloads the catalog from cfg.Catalog.SourceURL.
// The now parameter makes expiry deterministic in tests.
func ensureCatalogSynced(cfg *config.Config, configPath string, now time.Time) error {
	if cfg.Catalog.Enabled != nil && !*cfg.Catalog.Enabled {
		slog.Info("catalog sync disabled, skipping")
		return nil
	}

	// Skip sync if no source URL is configured.
	if cfg.Catalog.SourceURL == "" {
		slog.Debug("catalog source URL not configured, skipping sync")
		return nil
	}

	catalogDir := resolveCatalogDir(configPath)
	lock, err := catalog.ReadLock(catalogDir)
	if err != nil {
		slog.Info("catalog lock missing or unreadable, syncing", "catalog_dir", catalogDir, "error", err)
		_, err = catalog.Sync(cfg.Catalog.SourceURL, catalogDir)
		return err
	}

	if lock.Expired(now) {
		slog.Info("catalog lock expired, syncing", "catalog_dir", catalogDir, "synced_at", lock.SyncedAt)
		_, err = catalog.Sync(cfg.Catalog.SourceURL, catalogDir)
		return err
	}

	slog.Debug("catalog lock fresh, skipping sync", "catalog_dir", catalogDir, "synced_at", lock.SyncedAt)
	return nil
}

// ensureDatabase ensures the SQLite database exists and is initialized.
// It creates the database directory and schema if missing.
func ensureDatabase() error {
	db, err := storage.Open(storage.DefaultConfig)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer func() { _ = db.Close() }()

	slog.Info("initialized sqlite database", "path", storage.DefaultConfig.DatabasePath)
	return nil
}
