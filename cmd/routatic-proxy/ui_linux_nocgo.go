//go:build linux

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/gui"
	"github.com/routatic/proxy/internal/server"
	"github.com/spf13/cobra"
)

func openBrowser(target string) error {
	cmd := exec.Command("xdg-open", target)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// uiCmd is the "routatic-proxy ui" command (Linux, no CGO / no tray).
// It starts the proxy in the same process, opens the dashboard in the default
// browser, and waits for SIGINT/SIGTERM.
var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch GUI dashboard",
	Long: `Start the proxy server and open the graphical dashboard in your browser.
The proxy runs in the background; closing the browser leaves it running.
Press Ctrl+C to stop.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ── 1. Load config ──────────────────────────────────────────
		configPath, _ := cmd.Flags().GetString("config")
		if configPath == "" {
			configPath = config.ResolveConfigPath()
		} else {
			_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
		}

		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			slog.Info("Config file not found, auto-initializing default config", "path", configPath)
			configDir := filepath.Dir(configPath)
			if err := os.MkdirAll(configDir, 0700); err != nil {
				return fmt.Errorf("failed to create config directory: %w", err)
			}
			if err := os.WriteFile(configPath, []byte(getDefaultConfig()), 0600); err != nil {
				return fmt.Errorf("failed to write default config file: %w", err)
			}
		}

		cfg, err := config.Load()
		initialConfigValid := true
		if err != nil {
			initialConfigValid = false
			slog.Warn("Failed to load config (will require GUI configuration)", "error", err)
			cfg = &config.Config{
				Host: "127.0.0.1", Port: 3456,
				Logging: config.LoggingConfig{Level: "info"},
				OpenCodeGo: config.OpenCodeGoConfig{
					BaseURL:          "https://opencode.ai/zen/go/v1/chat/completions",
					AnthropicBaseURL: "https://opencode.ai/zen/go/v1/messages",
					TimeoutMs:        300000,
				},
				OpenCodeZen: config.OpenCodeZenConfig{
					BaseURL:          "https://opencode.ai/zen/v1/chat/completions",
					AnthropicBaseURL: "https://opencode.ai/zen/v1/messages",
					ResponsesBaseURL: "https://opencode.ai/zen/v1/responses",
					GeminiBaseURL:    "https://opencode.ai/zen/v1/models",
					TimeoutMs:        300000,
				},
			}
		}

		if initialConfigValid &&
			cfg.APIKey == "" && len(cfg.APIKeys) == 0 &&
			(cfg.OpenCodeGo.APIKey == "" || strings.Contains(cfg.OpenCodeGo.APIKey, "${")) &&
			(cfg.OpenCodeZen.APIKey == "" || strings.Contains(cfg.OpenCodeZen.APIKey, "${")) {
			initialConfigValid = false
			slog.Info("Config has no valid API keys set yet, waiting for GUI configuration")
		}

		atomic := config.NewAtomicConfig(cfg, config.ResolveConfigPath())

		// ── 2. Debug capture (optional) ─────────────────────────────
		var captureLogger *debug.CaptureLogger
		if cfg.Logging.DebugCapture != nil && cfg.Logging.DebugCapture.Enabled {
			storage, err := debug.NewStorage(*cfg.Logging.DebugCapture)
			if err != nil {
				return fmt.Errorf("failed to create debug storage: %w", err)
			}
			captureLogger = debug.NewCaptureLogger(storage, true)
			defer func() { _ = captureLogger.Close() }()
		}

		// ── 3. Create proxy server ──────────────────────────────────
		proxySrv, err := server.NewServer(atomic, captureLogger)
		if err != nil {
			return fmt.Errorf("create proxy server: %w", err)
		}

		// ── 4. Context + signals ────────────────────────────────────
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var startProxy func() error
		var stopProxy func() error

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-sigCh
			slog.Info("Received signal, exiting...")
			if stopProxy != nil {
				_ = stopProxy()
			}
			cancel()
		}()

		// ── 5. Start proxy ──────────────────────────────────────────
		var isProxyRunning bool
		var connectedToExisting bool
		var proxySrvMu sync.Mutex
		var guiSrv *gui.Server

		// Function to check and connect to existing proxy
		checkExistingProxy := func() bool {
			currentCfg := atomic.Get()
			healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", currentCfg.Port)
			client := &http.Client{Timeout: 2 * time.Second}
			resp, probeErr := client.Get(healthURL)
			if probeErr == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					slog.Info("Existing proxy detected on port, connecting to it", "port", currentCfg.Port)
					return true
				}
			}
			return false
		}

		// Check for existing proxy BEFORE creating GUI server
		connectedToExisting = checkExistingProxy()
		isProxyRunning = connectedToExisting

		startProxy = func() error {
			proxySrvMu.Lock()
			defer proxySrvMu.Unlock()
			if isProxyRunning {
				return nil
			}
			currentCfg := atomic.Get()
			if currentCfg.APIKey == "" && len(currentCfg.APIKeys) == 0 &&
				(currentCfg.OpenCodeGo.APIKey == "" || strings.Contains(currentCfg.OpenCodeGo.APIKey, "${")) &&
				(currentCfg.OpenCodeZen.APIKey == "" || strings.Contains(currentCfg.OpenCodeZen.APIKey, "${")) {
				return fmt.Errorf("API Key is empty. Please set it in Settings first")
			}

			isProxyRunning = true
			connectedToExisting = false
			if guiSrv != nil {
				guiSrv.SetProxyRunning(true)
				guiSrv.SetConnectedToExisting(false)
			}
			go func() {
				srvErr := proxySrv.Start()
				proxySrvMu.Lock()
				isProxyRunning = false
				proxySrvMu.Unlock()
				if guiSrv != nil {
					guiSrv.SetProxyRunning(false)
				}
				if srvErr != nil && srvErr != http.ErrServerClosed {
					slog.Error("proxy server stopped with error", "error", srvErr)
				}
			}()
			return nil
		}

		stopProxy = func() error {
			proxySrvMu.Lock()
			defer proxySrvMu.Unlock()
			if !isProxyRunning {
				return nil
			}
			wasConnected := connectedToExisting
			isProxyRunning = false
			connectedToExisting = false
			if guiSrv != nil {
				guiSrv.SetProxyRunning(false)
				guiSrv.SetConnectedToExisting(false)
			}
			if !wasConnected {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()
				return proxySrv.Shutdown(shutdownCtx)
			}
			return nil
		}

		if initialConfigValid {
			if err := startProxy(); err != nil {
				slog.Warn("Failed to auto-start proxy on boot", "error", err)
			}
		}

		// ── 6. Start GUI HTTP server ────────────────────────────────
		guiSrv = gui.New(gui.Options{
			History:          proxySrv.History,
			Metrics:          proxySrv.Metrics(),
			AtomicConfig:     atomic,
			ProxyPort:        cfg.Port,
			StartProxy:       startProxy,
			StopProxy:        stopProxy,
			CatalogDir:       resolveCatalogDir(configPath),
			CatalogSourceURL: cfg.Catalog.SourceURL,
		})

		// Set the connected flag now that guiSrv exists
		guiSrv.SetProxyRunning(isProxyRunning)
		guiSrv.SetConnectedToExisting(connectedToExisting)

		guiURL, err := guiSrv.Start(ctx)
		if err != nil {
			return fmt.Errorf("start gui server: %w", err)
		}

		// ── 7. Open browser ─────────────────────────────────────────
		slog.Info("Opening browser", "url", guiURL)
		if err := openBrowser(guiURL); err != nil {
			slog.Warn("Failed to open browser (xdg-open may not be available)", "error", err)
			fmt.Printf("\nDashboard URL: %s\n", guiURL)
		}

		// ── 8. Wait for signal ──────────────────────────────────────
		fmt.Println("\nPress Ctrl+C to stop the proxy and exit.")
		<-ctx.Done()
		return nil
	},
}

func addPlatformCommands(rootCmd *cobra.Command) {
	uiCmd.Flags().String("config", "", "Config file path")
	rootCmd.AddCommand(uiCmd)
}

func setupDefaultCommand() {}
