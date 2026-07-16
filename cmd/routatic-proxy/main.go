// Package main is the CLI entry point for the Routatic proxy server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/daemon"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/gui"
	"github.com/routatic/proxy/internal/server"
	"github.com/routatic/proxy/internal/storage"
	"github.com/spf13/cobra"
)

const (
	appName     = "routatic-proxy"
	pidFileName = "routatic-proxy.pid"
)

// Version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	rootCmd := &cobra.Command{
		Use:     appName,
		Aliases: []string{"oc-go-cc"},
		Short:   "Proxy Claude Code requests to OpenCode Go API",
		Long: `routatic-proxy is a CLI proxy tool that allows you to use your OpenCode Go
subscription with Claude Code. It intercepts Claude Code's Anthropic API requests,
transforms them to OpenAI format, and forwards them to OpenCode Go.

Configuration is stored at ~/.config/routatic-proxy/config.json.
Legacy ~/.config/oc-go-cc/config.json and OC_GO_CC_* environment variables are still supported.`,
		Version: version,
	}

	// Add subcommands.
	rootCmd.AddCommand(serveCmd())
	rootCmd.AddCommand(stopCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(initCmd())
	rootCmd.AddCommand(validateCmd())
	rootCmd.AddCommand(checkCmd())
	rootCmd.AddCommand(modelsCmd())
	rootCmd.AddCommand(catalogCmd())
	rootCmd.AddCommand(autostartCmd())
	rootCmd.AddCommand(updateCmd)
	rootCmd.AddCommand(startCmd())
	rootCmd.AddCommand(updateChannelCmd)

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// serveCmd returns the command to start the proxy server.
func serveCmd() *cobra.Command {
	var configPath string
	var port int
	var background bool
	var daemonize bool // hidden internal flag

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle background mode: fork and exit parent
			if background && !daemonize {
				opts := daemon.BackgroundOpts{
					ConfigPath: configPath,
					Port:       port,
				}
				return daemon.ForkIntoBackground(opts)
			}

			// Override config path if provided.
			if configPath != "" {
				_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := ensureCatalogSynced(cfg, configPath, time.Now().UTC()); err != nil {
				return fmt.Errorf("failed to sync catalog: %w", err)
			}

			// Ensure SQLite database exists.
			if err := ensureDatabase(); err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}

			var captureLogger *debug.CaptureLogger
			if cfg.Logging.DebugCapture != nil && cfg.Logging.DebugCapture.Enabled {
				storage, err := debug.NewStorage(*cfg.Logging.DebugCapture)
				if err != nil {
					return fmt.Errorf("failed to create debug storage: %w", err)
				}
				captureLogger = debug.NewCaptureLogger(storage, true)
				defer func() { _ = captureLogger.Close() }()
			}

			// Override port if provided via flag.
			if port != 0 {
				cfg.Port = port
			}

			pidPath := getPIDPath()

			// Check if already running before writing this process' PID.
			if !daemonize {
				if pid, err := daemon.GetPID(pidPath); err == nil {
					// Check if process is still running.
					if daemon.IsProcessRunning(pid) {
						return fmt.Errorf("server is already running (PID %d)", pid)
					}
					// Stale PID file, clean up.
					_ = os.Remove(pidPath)
				}
			}

			// Daemonize setup (child process after re-exec).
			if daemonize {
				paths, err := daemon.DefaultPaths()
				if err != nil {
					return err
				}
				if err := paths.EnsureConfigDir(); err != nil {
					return err
				}
				if err := daemon.DaemonizeSetup(paths); err != nil {
					return err
				}
			} else {
				// Ensure config directory exists before writing PID file.
				paths, err := daemon.DefaultPaths()
				if err != nil {
					return err
				}
				if err := paths.EnsureConfigDir(); err != nil {
					return err
				}
				// Write PID file for foreground mode.
				if err := daemon.WritePID(pidPath, os.Getpid()); err != nil {
					return fmt.Errorf("failed to write PID file: %w", err)
				}
			}
			defer func() { _ = os.Remove(pidPath) }()

			// Create atomic config for hot reload support.
			atomicCfg := config.NewAtomicConfig(cfg, config.ResolveConfigPath())

			// Re-apply CLI port override on every reload so it persists.
			if port != 0 {
				atomicCfg.OnReload(func(newCfg *config.Config) {
					newCfg.Port = port
				})
			}

			// Create and start server.
			srv, err := server.NewServer(atomicCfg, captureLogger)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}

			// Start config watcher for hot reload (only if enabled in config).
			if cfg.HotReload {
				watchCtx, watchCancel := context.WithCancel(context.Background())
				defer watchCancel()
				go func() {
					if err := config.WatchConfig(watchCtx, atomicCfg); err != nil && err != context.Canceled {
						slog.Error("config watcher failed", "error", err)
					}
				}()
			}

			fmt.Printf("Starting %s v%s\n", appName, version)
			fmt.Printf("Listening on %s:%d\n", cfg.Host, cfg.Port)
			if cfg.AnthropicFirst.Enabled {
				fmt.Printf("Forwarding to Anthropic first: %s\n", cfg.AnthropicFirst.BaseURL)
				fmt.Printf("OpenCode fallback: %s\n", cfg.OpenCodeGo.BaseURL)
			} else {
				fmt.Printf("Forwarding to: %s\n", cfg.OpenCodeGo.BaseURL)
			}
			fmt.Println()
			fmt.Println("Configure Claude Code with:")
			fmt.Printf("  export ANTHROPIC_BASE_URL=http://%s:%d\n", cfg.Host, cfg.Port)
			if cfg.AnthropicFirst.Enabled {
				fmt.Println("  unset ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY")
			} else {
				fmt.Println("  export ANTHROPIC_AUTH_TOKEN=unused")
			}
			fmt.Println()

			return srv.Start()
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Override listen port")
	cmd.Flags().BoolVarP(&background, "background", "b", false, "Run as background daemon")
	cmd.Flags().BoolVar(&daemonize, "_daemonize", false, "Internal use only")
	_ = cmd.Flags().MarkHidden("_daemonize")

	return cmd
}

// startCmd returns the command to start both the proxy server and GUI dashboard.
func startCmd() *cobra.Command {
	var configPath string
	var port int
	var background bool
	var headless bool
	var daemonize bool // hidden internal flag

	cmd := &cobra.Command{
		Use:   "start",
		Short: "Start the proxy server (with optional dashboard)",
		Long: `Start the proxy server and optionally the GUI dashboard.

The proxy runs on the configured port (default 3456). Use --headless to
skip the dashboard (equivalent to the legacy "serve" command). The dashboard
is available at http://127.0.0.1:3445 when not headless. All usage data
is persisted to SQLite.

Press Ctrl+C to stop the server.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Handle background mode: fork and exit parent
			if background && !daemonize {
				opts := daemon.BackgroundOpts{
					ConfigPath: configPath,
					Port:       port,
					Command:    "start",
				}
				return daemon.ForkIntoBackground(opts)
			}

			// Override config path if provided.
			if configPath != "" {
				_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("failed to load config: %w", err)
			}

			if err := ensureCatalogSynced(cfg, configPath, time.Now().UTC()); err != nil {
				return fmt.Errorf("failed to sync catalog: %w", err)
			}

			// Ensure SQLite database exists.
			if err := ensureDatabase(); err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}

			// Override port if provided via flag.
			if port != 0 {
				cfg.Port = port
			}

			pidPath := getPIDPath()

			// Check if already running before writing this process' PID.
			if !daemonize {
				if pid, err := daemon.GetPID(pidPath); err == nil {
					// Check if process is still running.
					if daemon.IsProcessRunning(pid) {
						return fmt.Errorf("server is already running (PID %d)", pid)
					}
					// Stale PID file, clean up.
					_ = os.Remove(pidPath)
				}
			}

			// Daemonize setup (child process after re-exec).
			if daemonize {
				paths, err := daemon.DefaultPaths()
				if err != nil {
					return err
				}
				if err := paths.EnsureConfigDir(); err != nil {
					return err
				}
				if err := daemon.DaemonizeSetup(paths); err != nil {
					return err
				}
			} else {
				// Ensure config directory exists before writing PID file.
				paths, err := daemon.DefaultPaths()
				if err != nil {
					return err
				}
				if err := paths.EnsureConfigDir(); err != nil {
					return err
				}
				// Write PID file for foreground mode.
				if err := daemon.WritePID(pidPath, os.Getpid()); err != nil {
					return fmt.Errorf("failed to write PID file: %w", err)
				}
			}
			defer func() { _ = os.Remove(pidPath) }()

			// Create atomic config for hot reload support.
			atomicCfg := config.NewAtomicConfig(cfg, config.ResolveConfigPath())

			// Re-apply CLI port override on every reload.
			if port != 0 {
				atomicCfg.OnReload(func(newCfg *config.Config) {
					newCfg.Port = port
				})
			}

			// Create and start proxy server.
			srv, err := server.NewServer(atomicCfg, nil)
			if err != nil {
				return fmt.Errorf("failed to create server: %w", err)
			}

			// Start config watcher for hot reload.
			if cfg.HotReload {
				watchCtx, watchCancel := context.WithCancel(context.Background())
				defer watchCancel()
				go func() {
					if err := config.WatchConfig(watchCtx, atomicCfg); err != nil && err != context.Canceled {
						slog.Error("config watcher failed", "error", err)
					}
				}()
			}

			// Context for graceful shutdown.
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			var guiSrv *gui.Server

			// Start proxy in background.
			go func() {
				if err := srv.Start(); err != nil && err != http.ErrServerClosed {
					slog.Error("proxy server error", "error", err)
					cancel()
				}
			}()

			// Print startup info.
			fmt.Printf("Starting %s v%s\n", appName, version)
			fmt.Printf("Proxy listening on %s:%d\n", cfg.Host, cfg.Port)
			fmt.Println()
			fmt.Println("Configure Claude Code with:")
			fmt.Printf("  export ANTHROPIC_BASE_URL=http://%s:%d\n", cfg.Host, cfg.Port)
			if cfg.AnthropicFirst.Enabled {
				fmt.Println("  unset ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY")
			} else {
				fmt.Println("  export ANTHROPIC_AUTH_TOKEN=unused")
			}

			if !headless {
				// Start GUI server on port 3445.
				guiSrv = gui.New(gui.Options{
					History:      srv.History,
					Metrics:      srv.Metrics(),
					AtomicConfig: atomicCfg,
					ProxyPort:    cfg.Port,
					Storage:      srv.Storage(),
				})
				guiSrv.SetProxyRunning(true)

				guiURL, err := guiSrv.Start(ctx)
				if err != nil {
					cancel()
					return fmt.Errorf("start gui server: %w", err)
				}

				// Open GUI (macOS: native webview, Linux/Windows: print URL)
				if err := openGUI(guiURL); err != nil {
					slog.Warn("GUI error", "error", err)
					fmt.Printf("\nDashboard: %s\n", guiURL)
					fmt.Println("\nPress Ctrl+C to stop.")
				}
			} else {
				fmt.Println("\nRunning in headless mode (no dashboard). Press Ctrl+C to stop.")
			}

			// Wait for signal.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			select {
			case <-sigCh:
				fmt.Println("\nShutting down...")
			case <-ctx.Done():
			}

			// Graceful shutdown.
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			_ = srv.Shutdown(shutdownCtx)
			if guiSrv != nil {
				_ = guiSrv.Shutdown(shutdownCtx)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Override proxy listen port")
	cmd.Flags().BoolVarP(&background, "background", "b", false, "Run as background daemon")
	cmd.Flags().BoolVarP(&headless, "headless", "H", false, "Skip dashboard (run proxy only)")
	cmd.Flags().BoolVar(&daemonize, "_daemonize", false, "Internal use only")
	_ = cmd.Flags().MarkHidden("_daemonize")

	return cmd
}

// stopCmd returns the command to stop the proxy server.
func stopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stop",
		Short: "Stop the proxy server",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidPath := getPIDPath()
			pid, err := daemon.GetPID(pidPath)
			if err != nil {
				return fmt.Errorf("server is not running (no PID file)")
			}

			if err := daemon.StopProcess(pid); err != nil {
				return fmt.Errorf("failed to stop server: %w", err)
			}

			fmt.Printf("Sent stop signal to server (PID %d)\n", pid)
			_ = os.Remove(pidPath)
			return nil
		},
	}
}

// statusCmd returns the command to check server status.
func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Check server status",
		RunE: func(cmd *cobra.Command, args []string) error {
			pidPath := getPIDPath()
			pid, err := daemon.GetPID(pidPath)
			if err != nil {
				fmt.Println("Server is not running")
				return nil
			}

			if !daemon.IsProcessRunning(pid) {
				fmt.Println("Server is not running (stale PID file)")
				_ = os.Remove(pidPath)
				return nil
			}

			fmt.Printf("Server is running (PID %d)\n", pid)
			return nil
		},
	}
}

// initCmd returns the command to create a default configuration file.
func initCmd() *cobra.Command {
	var provider string

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		Long: `Create a default configuration file optimized for a specific provider.

The --provider flag pre-configures the config with provider-specific defaults:
  - opencode-go: OpenCode Go subscription ($5/month, powerful coding models)
  - opencode-zen: OpenCode Zen (pay-as-you-go, Claude/GPT/Gemini)
  - aws-bedrock: AWS Bedrock Mantle (run models on your AWS infrastructure)
  - openrouter: OpenRouter (unified API for 100+ models)

Without --provider, a default config optimized for OpenCode Go is created.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Resolve config path, respecting ROUTATIC_PROXY_CONFIG env var.
			configPath := config.ResolveConfigPath()

			// Check if config already exists
			if _, err := os.Stat(configPath); err == nil {
				fmt.Printf("Config already exists at %s\n", configPath)
				fmt.Println("Edit the file to update your configuration.")
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(configPath), 0700); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			// Get the appropriate config template
			var configContent string
			var err error
			if provider != "" {
				configContent, err = getProviderConfig(provider)
				if err != nil {
					return err
				}
			} else {
				configContent = getDefaultConfig()
			}

			if err := os.WriteFile(configPath, []byte(configContent), 0600); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			// Initialize database.
			db, err := storage.Open(storage.DefaultConfig)
			if err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}
			defer func() { _ = db.Close() }()

			// Print helpful message based on provider
			fmt.Printf("Created config at %s\n", configPath)
			fmt.Printf("Initialized database at %s\n", storage.DefaultConfig.DatabasePath)
			if provider != "" {
				preset, ok := providerPresets[provider]
				if ok {
					fmt.Printf("\nProvider: %s\n", preset.Name)
					fmt.Printf("Set your API key: export %s=your-api-key-here\n", preset.EnvVarName)
				}
			} else {
				fmt.Println("\nEdit the file and add your OpenCode Go API key.")
				fmt.Println("Or use --provider to generate a provider-specific config.")
			}
			return nil
		},
	}

	cmd.Flags().StringVar(&provider, "provider", "",
		"Provider preset: opencode-go, opencode-zen, aws-bedrock, openrouter")

	return cmd
}

// validateCmd returns the command to validate the configuration file.
func validateCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" {
				_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				return fmt.Errorf("invalid config: %w", err)
			}

			// Ensure SQLite database exists.
			if err := ensureDatabase(); err != nil {
				return fmt.Errorf("failed to initialize database: %w", err)
			}

			fmt.Println("Configuration is valid!")
			fmt.Printf("  Host: %s\n", cfg.Host)
			fmt.Printf("  Port: %d\n", cfg.Port)
			if keys := cfg.EffectiveAPIKeys(); len(keys) > 1 {
				fmt.Printf("  API Keys: %d keys (round-robin)\n", len(keys))
			} else if len(keys) == 1 {
				fmt.Printf("  API Key: %s...\n", maskString(keys[0], 8))
			}
			fmt.Printf("  Base URL: %s\n", cfg.OpenCodeGo.BaseURL)
			fmt.Printf("  Models configured: %d\n", len(cfg.Models))
			fmt.Printf("  Fallback chains: %d\n", len(cfg.Fallbacks))
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	return cmd
}

// checkCmd returns the command to check Claude Code environment conflicts.
func checkCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:          "check",
		Short:        "Check Claude Code env conflicts",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath != "" {
				_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
			}

			cfg, err := config.Load()
			if err != nil {
				cfg = &config.Config{Host: "127.0.0.1", Port: 3456}
				fmt.Printf("Warning: could not load config (%v), using defaults for check\n", err)
			}

			expectedURL := strings.TrimRight(fmt.Sprintf("http://%s:%d", cfg.Host, cfg.Port), "/")
			conflicts := 0

			env := map[string]string{}
			for _, key := range []string{"ANTHROPIC_BASE_URL", "ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN"} {
				if value, ok := os.LookupEnv(key); ok {
					env[key] = value
				}
			}
			conflicts += checkClaudeEnv("environment", env, expectedURL, cfg.AnthropicFirst.Enabled)

			home, err := os.UserHomeDir()
			if err != nil {
				fmt.Printf("Warning: cannot determine home directory: %v\n", err)
			} else {
				for _, path := range []string{
					filepath.Join(home, ".claude", "settings.json"),
					filepath.Join(home, ".claude.json"),
				} {
					data, err := os.ReadFile(path)
					if err != nil {
						if !os.IsNotExist(err) {
							fmt.Printf("%s: %v\n", path, err)
						}
						continue
					}

					var settings struct {
						Env map[string]string `json:"env"`
					}
					if err := json.Unmarshal(data, &settings); err != nil {
						fmt.Printf("%s: %v\n", path, err)
						continue
					}
					conflicts += checkClaudeEnv(path, settings.Env, expectedURL, cfg.AnthropicFirst.Enabled)
				}
			}

			if conflicts > 0 {
				return fmt.Errorf("found %d Claude Code env conflict(s)", conflicts)
			}
			fmt.Println("No Claude Code env conflicts found.")
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	return cmd
}

// checkClaudeEnv checks a single environment map for conflicting Claude Code settings.
// Returns the number of conflicts found.
func checkClaudeEnv(source string, env map[string]string, expectedURL string, anthropicFirst bool) int {
	conflicts := 0
	if value, ok := env["ANTHROPIC_BASE_URL"]; ok {
		normalized := strings.TrimRight(value, "/")
		if normalized != expectedURL {
			fmt.Printf("%s: ANTHROPIC_BASE_URL is %q, expected %q\n", source, value, expectedURL)
			conflicts++
		}
	}
	if _, ok := env["ANTHROPIC_API_KEY"]; ok {
		fmt.Printf("%s: ANTHROPIC_API_KEY is set\n", source)
		conflicts++
	}
	if value, ok := env["ANTHROPIC_AUTH_TOKEN"]; ok {
		if anthropicFirst {
			fmt.Printf("%s: ANTHROPIC_AUTH_TOKEN is set; unset it to keep the saved Claude subscription login active\n", source)
			conflicts++
		} else if value != "unused" {
			fmt.Printf("%s: ANTHROPIC_AUTH_TOKEN is %q, expected \"unused\"\n", source, value)
			conflicts++
		}
	} else if !anthropicFirst {
		fmt.Printf("%s: ANTHROPIC_AUTH_TOKEN is not set (recommended: \"unused\")\n", source)
	}
	return conflicts
}

// modelsCmd returns the command to list available models.
func modelsCmd() *cobra.Command {
	var configPath string
	var provider string

	cmd := &cobra.Command{
		Use:   "models",
		Short: "List available models from the catalog",
		Long: `List available models from the catalog.

Without a subcommand, all enabled providers are shown. Use "models list" to
filter by provider with the --provider flag.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModelsList(cmd, configPath, provider)
		},
	}

	cmd.PersistentFlags().StringVarP(&configPath, "config", "c", "", "Path to config file (used to locate the catalog directory)")
	cmd.PersistentFlags().StringVar(&provider, "provider", "", "Filter models by provider")

	cmd.AddCommand(modelsListCmd())

	return cmd
}

// modelsListCmd returns the "models list" subcommand.
func modelsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available models from the catalog",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, _ := cmd.Flags().GetString("config")
			provider, _ := cmd.Flags().GetString("provider")
			return runModelsList(cmd, configPath, provider)
		},
	}
}

// runModelsList prints models from the catalog, optionally filtered by provider.
func runModelsList(cmd *cobra.Command, configPath, provider string) error {
	if configPath != "" {
		_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
	}

	cfgPath := config.ResolveConfigPath()
	cfg, err := config.LoadFromPath(cfgPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	storageCfg := storage.DefaultConfig
	if cfg.Storage != nil {
		storageCfg = storage.Config{
			DatabasePath:    cfg.Storage.DatabasePath,
			RetentionDays:   cfg.Storage.RetentionDays,
			VacuumOnStartup: cfg.Storage.VacuumOnStartup,
			WALEnabled:      cfg.Storage.WALEnabled,
		}
	}

	db, err := storage.Open(storageCfg)
	if err != nil {
		return fmt.Errorf("failed to open storage: %w", err)
	}
	defer func() { _ = db.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cat, err := catalog.LoadFromSQLite(ctx, db)
	if err != nil {
		return fmt.Errorf("catalog not found; run 'routatic-proxy catalog sync' first")
	}

	providers := selectProviders(provider, cfg)
	if len(providers) == 0 {
		if provider != "" {
			cmd.Printf("No models found for provider %q.\n", provider)
		} else {
			cmd.Println("No enabled providers found.")
		}
		return nil
	}

	var lines []string
	for _, p := range providers {
		models := cat.ListProviderModels(p)
		if len(models) == 0 {
			continue
		}
		ids := make([]string, len(models))
		for i, m := range models {
			ids[i] = m.ModelID
		}
		sort.Strings(ids)
		for _, id := range ids {
			lines = append(lines, fmt.Sprintf("%s/%s", p, id))
		}
	}

	if len(lines) == 0 {
		if provider != "" {
			cmd.Printf("No models found for provider %q.\n", provider)
		} else {
			cmd.Println("No models found for enabled providers.")
		}
		return nil
	}

	for _, line := range lines {
		cmd.Println(line)
	}

	cmd.Println()
	cmd.Println("Use these model IDs in your config.json file (model_overrides).")
	return nil
}

// selectProviders returns the providers to display. If provider is non-empty,
// only that provider is returned when it exists in the catalog; otherwise all
// configured (enabled) providers are returned.
func selectProviders(provider string, cfg *config.Config) []string {
	if provider != "" {
		return []string{provider}
	}

	globalKeys := cfg.EffectiveAPIKeys()
	providerKeys := map[string][]string{
		"opencode-go":  cfg.OpenCodeGo.EffectiveAPIKeys(),
		"opencode-zen": cfg.OpenCodeZen.EffectiveAPIKeys(),
		"aws-bedrock":  cfg.AWSBedrock.EffectiveAPIKeys(),
		"openrouter":   cfg.OpenRouter.EffectiveAPIKeys(),
	}

	var enabled []string
	for p, keys := range providerKeys {
		if len(keys) > 0 || len(globalKeys) > 0 {
			enabled = append(enabled, p)
		}
	}
	sort.Strings(enabled)
	return enabled
}

// autostartCmd returns the command to manage autostart on login.
func autostartCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "autostart",
		Short: "Manage auto-start on login",
	}

	cmd.AddCommand(&cobra.Command{
		Use:   "enable",
		Short: "Enable auto-start on login",
		RunE: func(cmd *cobra.Command, args []string) error {
			var configPath string
			var port int
			if cmd.Flags().Changed("config") {
				configPath, _ = cmd.Flags().GetString("config")
			}
			if cmd.Flags().Changed("port") {
				port, _ = cmd.Flags().GetInt("port")
			}
			return daemon.EnableAutostart(configPath, port)
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "disable",
		Short: "Disable auto-start on login",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.DisableAutostart()
		},
	})

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check auto-start status",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.AutostartStatus()
		},
	})

	cmd.PersistentFlags().StringP("config", "c", "", "Path to config file")
	cmd.PersistentFlags().IntP("port", "p", 0, "Override listen port")

	return cmd
}

// getPIDPath returns the path to the PID file.
func getPIDPath() string {
	paths, err := daemon.DefaultPaths()
	if err != nil {
		// Fallback to temp dir if home dir cannot be determined
		return filepath.Join(os.TempDir(), pidFileName)
	}
	return paths.PIDFile
}

// maskString masks all but the first `visible` characters of a string.
func maskString(s string, visible int) string {
	if len(s) <= visible {
		return s
	}
	return s[:visible] + "..."
}

// getDefaultConfig returns a default configuration JSON template.
// Optimized for cost-efficiency: uses cheaper models by default, expensive ones only when needed.
func getDefaultConfig() string {
	return `{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "enable_streaming_scenario_routing": false,
  "respect_requested_model": false,
  "anthropic_first": {
    "enabled": false,
    "base_url": "https://api.anthropic.com"
  },
  "models": {
    "background": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "default": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5.2",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "glm-5.2": {
      "provider": "opencode-go",
      "model_id": "glm-5.2",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "kimi-k2.7-code": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.7-code",
      "temperature": 0.7,
      "max_tokens": 32768
    },
    "qwen3.7-plus": {
      "provider": "opencode-go",
      "model_id": "qwen3.7-plus",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "qwen3.7-max": {
      "provider": "opencode-go",
      "model_id": "qwen3.7-max",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },
  "fallbacks": {
    "background": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "default": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "long_context": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "think": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "complex": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "fast": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "glm-5.2": [
      { "provider": "opencode-go", "model_id": "glm-5.1" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "kimi-k2.7-code": [
      { "provider": "opencode-go", "model_id": "kimi-k2.6" },
      { "provider": "opencode-go", "model_id": "glm-5.1" }
    ],
    "qwen3.7-plus": [
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "qwen3.7-max": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ]
  },
  "model_overrides": {
    "deepseek-v4-pro": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": {
        "type": "enabled"
      }
    },
    "deepseek-v4-flash-free": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-flash-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "grok-build-0.1": {
      "provider": "opencode-zen",
      "model_id": "grok-build-0.1",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "big-pickle": {
      "provider": "opencode-zen",
      "model_id": "big-pickle",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "mimo-v2.5-free": {
      "provider": "opencode-zen",
      "model_id": "mimo-v2.5-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "north-mini-code-free": {
      "provider": "opencode-zen",
      "model_id": "north-mini-code-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "nemotron-3-ultra-free": {
      "provider": "opencode-zen",
      "model_id": "nemotron-3-ultra-free",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "claude-fable-5": {
      "provider": "opencode-zen",
      "model_id": "claude-fable-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-8": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-8",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-6": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-6",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-5": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-opus-4-1": {
      "provider": "opencode-zen",
      "model_id": "claude-opus-4-1",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "claude-sonnet-4": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3.5-flash": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.5-flash",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3.1-pro": {
      "provider": "opencode-zen",
      "model_id": "gemini-3.1-pro",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "gemini-3-flash": {
      "provider": "opencode-zen",
      "model_id": "gemini-3-flash",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },
  "model_family_overrides": {
    "opus": { "provider": "opencode-go", "model_id": "glm-5.1", "temperature": 0.7, "max_tokens": 8192, "vision": true },
    "sonnet": { "provider": "opencode-go", "model_id": "kimi-k2.6", "temperature": 0.7, "max_tokens": 8192, "vision": true },
    "haiku": { "provider": "opencode-go", "model_id": "qwen3.7-plus", "temperature": 0.7, "max_tokens": 4096, "vision": true }
  },
  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "api_key": "",
    "api_keys": [],
    "timeout_ms": 300000
  },
  "opencode_zen": {
    "base_url": "https://opencode.ai/zen/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/v1/messages",
    "responses_base_url": "https://opencode.ai/zen/v1/responses",
    "gemini_base_url": "https://opencode.ai/zen/v1/models",
    "api_key": "",
    "api_keys": [],
    "timeout_ms": 300000
  },
  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}
