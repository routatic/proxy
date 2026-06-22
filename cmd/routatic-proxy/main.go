// Package main is the CLI entry point for the Routatic proxy server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/auth"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/daemon"
	"github.com/routatic/proxy/internal/server"
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
	rootCmd.AddCommand(autostartCmd())

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
			return runServe(configPath, port, background, daemonize)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "Path to config file")
	cmd.Flags().IntVarP(&port, "port", "p", 0, "Override listen port")
	cmd.Flags().BoolVarP(&background, "background", "b", false, "Run as background daemon")
	cmd.Flags().BoolVar(&daemonize, "_daemonize", false, "Internal use only")
	_ = cmd.Flags().MarkHidden("_daemonize")

	return cmd
}

// runServe contains the actual serve logic, extracted for testability.
func runServe(configPath string, port int, background, daemonize bool) error {
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

	var cfg *config.Config
	var err error

	// Check if a config file exists; if not, load entirely from environment.
	// This supports serverless deployments (Railway, etc.) where no config file exists.
	configFilePath := config.ResolveConfigPath()
	if _, statErr := os.Stat(configFilePath); statErr == nil {
		cfg, err = config.Load()
	} else {
		// Config file doesn't exist - try loading from environment via ROUTATIC_PROXY_SERVICE_TOKEN
		// and other cloud-specific environment variables
		cfg, err = config.LoadFromEnv()
	}
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Override port if provided via flag.
	if port != 0 {
		cfg.Port = port
	}

	pidPath := getPIDPath()

	// Skip PID file operations in serverless/cloud environments
	// Detection: no local config file + mode is managed
	isServerless := cfg.Mode == "managed" && cfg.ConfigProv.Provider == "cloud_snapshot"

	if !isServerless && !daemonize {
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
	// Skip PID/config directory operations in serverless mode.
	if !isServerless {
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
	}

	// Initialize providers based on mode.
	authProvider, configProvider, err := initProviders(cfg)
	if err != nil {
		return fmt.Errorf("failed to initialize providers: %w", err)
	}

	// Close providers on shutdown.
	defer func() {
		if authProvider != nil {
			if c, ok := authProvider.(interface{ Close() error }); ok {
				_ = c.Close()
			}
		}
		if configProvider != nil {
			// FileConfigProvider has StopWatching method
			if c, ok := configProvider.(interface{ StopWatching() error }); ok {
				_ = c.StopWatching()
			}
		}
	}()

	// Create atomic config for hot reload support.
	atomicCfg := config.NewAtomicConfig(cfg, config.ResolveConfigPath())

	// Re-apply CLI port override on every reload so it persists.
	if port != 0 {
		atomicCfg.OnReload(func(newCfg *config.Config) {
			newCfg.Port = port
		})
	}

	// Create and start server with providers.
	// Note: The server currently doesn't use these providers directly (auth/config
	// are handled via middleware and RuntimeConfig), but we initialize them here
	// for future extensibility and health checks.
	srv, err := server.NewServer(atomicCfg)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	// Register providers with server for health checks.
	server.RegisterHealthCheckers(srv, authProvider, configProvider)

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
	fmt.Printf("Forwarding to: %s\n", cfg.OpenCodeGo.BaseURL)
	fmt.Println()
	fmt.Println("Configure Claude Code with:")
	fmt.Printf("  export ANTHROPIC_BASE_URL=http://%s:%d\n", cfg.Host, cfg.Port)
	fmt.Println("  export ANTHROPIC_AUTH_TOKEN=unused")
	fmt.Println()

	return srv.Start()
}

// initProviders initializes the authentication and configuration providers
// based on the server mode configuration.
//
// Mode "standalone": Uses local file-based config and local key auth (backward compatible).
// Mode "managed": Uses cloud snapshot or DB-based config with cloud auth.
func initProviders(cfg *config.Config) (auth.AuthProvider, config.ConfigProvider, error) {
	authProvider, err := initAuthProvider(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("auth provider: %w", err)
	}

	configProvider, err := initConfigProvider(cfg)
	if err != nil {
		if c, ok := authProvider.(interface{ Close() error }); ok {
			_ = c.Close()
		}
		return nil, nil, fmt.Errorf("config provider: %w", err)
	}

	return authProvider, configProvider, nil
}

// initAuthProvider creates an AuthProvider based on the configuration.
// Supported providers: "local_keys", "cloud", "none".
func initAuthProvider(cfg *config.Config) (auth.AuthProvider, error) {
	provider := cfg.Auth.Provider
	if provider == "" {
		// Default to "none" for backward compatibility
		provider = "none"
	}

	switch provider {
	case "local_keys":
		if cfg.Auth.ConfigPath == "" {
			return nil, fmt.Errorf("local_keys auth requires config_path")
		}
		return auth.NewLocalKeyAuthProvider(cfg.Auth.ConfigPath)

	case "cloud":
		if cfg.Auth.IntrospectionURL == "" {
			return nil, fmt.Errorf("cloud auth requires introspection_url")
		}
		ttl := cfg.Auth.CacheTTL
		if ttl <= 0 {
			ttl = 30 * time.Second
		}
		// Get optional service token from environment
		serviceToken := os.Getenv("ROUTATIC_PROXY_SERVICE_TOKEN")
		return auth.NewCloudAuthProvider(cfg.Auth.IntrospectionURL, ttl, serviceToken), nil

	case "none":
		workspaceID := "local"
		if cfg.Mode == "standalone" && cfg.APIKey != "" {
			// Use a stable workspace ID based on first API key hash
			workspaceID = hashAPIKeyForWorkspace(cfg.APIKey)
		}
		return auth.NewNoAuthProvider(workspaceID), nil

	default:
		return nil, fmt.Errorf("unknown auth provider: %s", provider)
	}
}

// initConfigProvider creates a ConfigProvider based on the configuration.
// Supported providers: "file", "db", "cloud_snapshot", "" (autodetect).
func initConfigProvider(cfg *config.Config) (config.ConfigProvider, error) {
	provider := cfg.ConfigProv.Provider

	var underlying config.ConfigProvider
	var err error

	switch provider {
	case "":
		// No explicit config provider - autodetect based on whether config file exists.
		// If config file exists, use file provider for hot reload.
		// Otherwise, use static bootstrap config (for simple/serverless deployments).
		configPath := cfg.ConfigProv.Path
		if configPath == "" {
			configPath = config.ResolveConfigPath()
		}
		if _, statErr := os.Stat(configPath); statErr == nil {
			// Config file exists - use file provider for hot reload support
			underlying, err = config.NewFileConfigProvider(configPath)
			if err != nil {
				return nil, err
			}
		} else {
			// No config file - use static bootstrap config
			underlying = config.NewStaticConfigProvider(config.CreateBootstrapRuntimeConfig(cfg))
		}

	case "file":
		path := cfg.ConfigProv.Path
		if path == "" {
			// Use default config path
			path = config.ResolveConfigPath()
		}
		underlying, err = config.NewFileConfigProvider(path)
		if err != nil {
			return nil, err
		}

	case "db":
		if cfg.ConfigProv.DB.Driver == "" {
			return nil, fmt.Errorf("db provider requires driver")
		}
		if cfg.ConfigProv.DB.DSN == "" {
			return nil, fmt.Errorf("db provider requires dsn")
		}
		// Note: DBConfigProvider is not yet implemented in this version
		return nil, fmt.Errorf("db config provider not implemented: driver=%s", cfg.ConfigProv.DB.Driver)

	case "cloud_snapshot":
		if cfg.ConfigProv.SnapshotURL == "" {
			return nil, fmt.Errorf("cloud_snapshot provider requires snapshot_url")
		}
		ttl := cfg.ConfigProv.CacheTTL
		if ttl <= 0 {
			ttl = 15 * time.Second
		}
		// Get optional service token from environment
		serviceToken := os.Getenv("ROUTATIC_PROXY_SERVICE_TOKEN")
		underlying = config.NewCloudSnapshotConfigProvider(cfg.ConfigProv.SnapshotURL, ttl, serviceToken)

	default:
		return nil, fmt.Errorf("unknown config provider: %s", provider)
	}

	// Wrap with cache for providers that benefit from it:
	// - Static bootstrap config: cache to avoid recreating RuntimeConfig on every request
	// - Cloud snapshot: cache to avoid hitting cloud API on every request
	// - File provider: has its own internal caching via hot reload, don't wrap
	needsCache := provider == "" || provider == "cloud_snapshot" || provider == "db"
	if needsCache {
		ttl := cfg.ConfigProv.CacheTTL
		if ttl <= 0 {
			ttl = 15 * time.Second
		}
		underlying = config.NewCachedConfigProvider(underlying, ttl)
	}

	return underlying, nil
}

// hashAPIKeyForWorkspace creates a stable workspace ID from an API key.
// This is used for the "none" auth provider in standalone mode to provide
// a stable identity for logging and metrics.
func hashAPIKeyForWorkspace(apiKey string) string {
	if len(apiKey) <= 8 {
		return apiKey
	}
	return "wk_" + apiKey[:8]
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
	return &cobra.Command{
		Use:   "init",
		Short: "Create default configuration file",
		RunE: func(cmd *cobra.Command, args []string) error {
			configDir := getConfigDir()
			configPath := filepath.Join(configDir, "config.json")

			// Check if config already exists
			if _, err := os.Stat(configPath); err == nil {
				fmt.Printf("Config already exists at %s\n", configPath)
				fmt.Println("Edit the file to update your configuration.")
				return nil
			}

			if err := os.MkdirAll(configDir, 0700); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}

			if err := os.WriteFile(configPath, []byte(getDefaultConfig()), 0600); err != nil {
				return fmt.Errorf("failed to write config file: %w", err)
			}

			fmt.Printf("Created default config at %s\n", configPath)
			fmt.Println("Edit the file and add your OpenCode Go API key.")
			return nil
		},
	}
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
			conflicts += checkClaudeEnv("environment", env, expectedURL)

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
					conflicts += checkClaudeEnv(path, settings.Env, expectedURL)
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
func checkClaudeEnv(source string, env map[string]string, expectedURL string) int {
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
		if value != "unused" {
			fmt.Printf("%s: ANTHROPIC_AUTH_TOKEN is %q, expected \"unused\"\n", source, value)
			conflicts++
		}
	} else {
		fmt.Printf("%s: ANTHROPIC_AUTH_TOKEN is not set (recommended: \"unused\")\n", source)
	}
	return conflicts
}

// modelsCmd returns the command to list available models.
func modelsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "models",
		Short: "List available OpenCode Go models",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Println("Available OpenCode Go models:")
			fmt.Println()
			fmt.Println("  Model ID                   Endpoint Type")
			fmt.Println("  ──────────────────────────────────────────────")
			fmt.Println("  glm-5.1                    OpenAI-compatible")
			fmt.Println("  glm-5                      OpenAI-compatible")
			fmt.Println("  kimi-k2.6                  OpenAI-compatible")
			fmt.Println("  kimi-k2.5                  OpenAI-compatible")
			fmt.Println("  mimo-v2.5-pro              OpenAI-compatible")
			fmt.Println("  mimo-v2.5                  OpenAI-compatible")
			fmt.Println("  minimax-m3                 Anthropic-compatible")
			fmt.Println("  minimax-m2.7               Anthropic-compatible")
			fmt.Println("  minimax-m2.5               Anthropic-compatible")
			fmt.Println("  deepseek-v4-pro            OpenAI-compatible")
			fmt.Println("  deepseek-v4-flash          OpenAI-compatible")
			fmt.Println("  qwen3.7-plus               Anthropic-compatible")
			fmt.Println("  qwen3.7-max                Anthropic-compatible")
			fmt.Println("  qwen3.6-plus               Anthropic-compatible")
			fmt.Println("  qwen3.5-plus               Anthropic-compatible")
			fmt.Println()
			fmt.Println("Available OpenCode Zen models (free tier):")
			fmt.Println()
			fmt.Println("  deepseek-v4-pro            OpenAI-compatible")
			fmt.Println("  deepseek-v4-flash-free     OpenAI-compatible")
			fmt.Println("  grok-build-0.1             OpenAI-compatible")
			fmt.Println("  big-pickle                 OpenAI-compatible")
			fmt.Println("  mimo-v2.5-free             OpenAI-compatible")
			fmt.Println("  north-mini-code-free       OpenAI-compatible")
			fmt.Println("  nemotron-3-ultra-free      OpenAI-compatible")
			fmt.Println()
			fmt.Println("Available OpenCode Zen models (Anthropic endpoint):")
			fmt.Println()
			fmt.Println("  claude-fable-5             Anthropic-compatible")
			fmt.Println("  claude-opus-4-8            Anthropic-compatible")
			fmt.Println("  claude-opus-4-7            Anthropic-compatible")
			fmt.Println("  claude-opus-4-6            Anthropic-compatible")
			fmt.Println("  claude-opus-4-5            Anthropic-compatible")
			fmt.Println("  claude-opus-4-1            Anthropic-compatible")
			fmt.Println("  claude-sonnet-4-6          Anthropic-compatible")
			fmt.Println("  claude-sonnet-4-5          Anthropic-compatible")
			fmt.Println("  claude-sonnet-4            Anthropic-compatible")
			fmt.Println("  claude-haiku-4-5           Anthropic-compatible")
			fmt.Println("  claude-3-5-haiku           Anthropic-compatible")
			fmt.Println()
			fmt.Println("Available OpenCode Zen models (Responses endpoint):")
			fmt.Println()
			fmt.Println("  gpt-5.5                    Responses-compatible")
			fmt.Println("  gpt-5.5-pro                Responses-compatible")
			fmt.Println("  gpt-5.4                    Responses-compatible")
			fmt.Println("  gpt-5.4-pro                Responses-compatible")
			fmt.Println("  gpt-5.4-mini               Responses-compatible")
			fmt.Println("  gpt-5.4-nano               Responses-compatible")
			fmt.Println("  gpt-5.3-codex              Responses-compatible")
			fmt.Println("  gpt-5.3-codex-spark        Responses-compatible")
			fmt.Println("  gpt-5.2                    Responses-compatible")
			fmt.Println("  gpt-5.2-codex              Responses-compatible")
			fmt.Println("  gpt-5.1                    Responses-compatible")
			fmt.Println("  gpt-5.1-codex              Responses-compatible")
			fmt.Println("  gpt-5.1-codex-max          Responses-compatible")
			fmt.Println("  gpt-5.1-codex-mini         Responses-compatible")
			fmt.Println("  gpt-5                      Responses-compatible")
			fmt.Println("  gpt-5-codex                Responses-compatible")
			fmt.Println("  gpt-5-nano                 Responses-compatible")
			fmt.Println()
			fmt.Println("Available OpenCode Zen models (Gemini endpoint):")
			fmt.Println()
			fmt.Println("  gemini-3.5-flash           Gemini-compatible")
			fmt.Println("  gemini-3.1-pro             Gemini-compatible")
			fmt.Println("  gemini-3-flash             Gemini-compatible")
			fmt.Println()
			fmt.Println("Use these model IDs in your config.json file (model_overrides).")
		},
	}
}

// getConfigDir returns the default configuration directory path.
func getConfigDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "routatic-proxy")
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
  "respect_requested_model": true,
  "models": {
    "background": {
      "provider": "opencode-go",
      "model_id": "qwen3.5-plus",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "default": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m2.5",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "qwen3.6-plus",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },
  "fallbacks": {
    "background": [
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" },
      { "provider": "opencode-go", "model_id": "minimax-m2.5" }
    ],
    "default": [
      { "provider": "opencode-go", "model_id": "mimo-v2-pro" },
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" }
    ],
    "long_context": [
      { "provider": "opencode-go", "model_id": "minimax-m2.7" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "think": [
      { "provider": "opencode-go", "model_id": "kimi-k2.6" },
      { "provider": "opencode-go", "model_id": "mimo-v2-pro" }
    ],
    "complex": [
      { "provider": "opencode-go", "model_id": "glm-5" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "fast": [
      { "provider": "opencode-go", "model_id": "qwen3.5-plus" },
      { "provider": "opencode-go", "model_id": "minimax-m2.5" }
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
    }
  },
  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "timeout_ms": 300000
  },
  "opencode_zen": {
    "base_url": "https://opencode.ai/zen/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/v1/messages",
    "responses_base_url": "https://opencode.ai/zen/v1/responses",
    "gemini_base_url": "https://opencode.ai/zen/v1/models",
    "timeout_ms": 300000
  },
  "logging": {
    "level": "info",
    "requests": true
  }
}
`
}
