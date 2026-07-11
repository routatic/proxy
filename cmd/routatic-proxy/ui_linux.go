//go:build linux && cgo

package main

import (
	"context"
	"fmt"
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
	"github.com/routatic/proxy/internal/daemon"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/gui"
	"github.com/routatic/proxy/internal/server"
	"github.com/routatic/proxy/internal/tray"
	"github.com/spf13/cobra"
)

// globalGUIURL is set after the GUI server starts, so tray.OnOpen can reopen it.
var globalGUIURL string

// openBrowser opens the default browser to the given URL using xdg-open.
func openBrowser(target string) error {
	// Detach the browser process so killing the proxy doesn't close the browser tab.
	cmd := exec.Command("xdg-open", target)
	cmd.Stdin = nil
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Start()
}

// uiCmd is the "routatic-proxy ui" command (Linux).
// It starts the proxy in the same process, opens the dashboard in the default
// browser, and adds a system tray icon.
var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch GUI dashboard",
	Long: `Start the proxy server and open the graphical dashboard in your browser.
The proxy runs in the background; closing the browser leaves it running.
Use the tray icon to reopen the window or quit entirely.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ── 1. Load config ──────────────────────────────────────────
		configPath, _ := cmd.Flags().GetString("config")
		if configPath == "" {
			configPath = config.ResolveConfigPath()
		} else {
			_ = os.Setenv("ROUTATIC_PROXY_CONFIG", configPath)
		}

		// Auto-initialize config file if it does not exist.
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
				Host: "127.0.0.1",
				Port: 3456,
				Logging: config.LoggingConfig{
					Level: "info",
				},
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

		if initialConfigValid {
			if cfg.APIKey == "" && len(cfg.APIKeys) == 0 &&
				(cfg.OpenCodeGo.APIKey == "" || strings.Contains(cfg.OpenCodeGo.APIKey, "${")) &&
				(cfg.OpenCodeZen.APIKey == "" || strings.Contains(cfg.OpenCodeZen.APIKey, "${")) {
				initialConfigValid = false
				slog.Info("Config has no valid API keys set yet, waiting for GUI configuration")
			}
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
			tray.Quit()
		}()

		// ── 5. Start proxy ──────────────────────────────────────────
		proxyErrCh := make(chan error, 1)
		var isProxyRunning bool
		var connectedToExisting bool
		var proxySrvMu sync.Mutex
		var guiSrv *gui.Server

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
			}
			tray.SetRunning(true)

			go func() {
				srvErr := proxySrv.Start()
				proxySrvMu.Lock()
				isProxyRunning = false
				proxySrvMu.Unlock()

				if guiSrv != nil {
					guiSrv.SetProxyRunning(false)
				}
				tray.SetRunning(false)

				if srvErr != nil && srvErr != http.ErrServerClosed {
					slog.Error("proxy server stopped with error", "error", srvErr)
					select {
					case proxyErrCh <- srvErr:
					default:
					}
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
			}
			tray.SetRunning(false)

			if !wasConnected {
				shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer shutdownCancel()
				return proxySrv.Shutdown(shutdownCtx)
			}
			return nil
		}

		proxyInitiallyStarted := false
		if initialConfigValid {
			if err := startProxy(); err == nil {
				proxyInitiallyStarted = true
			} else {
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
		guiSrv.SetProxyRunning(proxyInitiallyStarted)

		guiURL, err := guiSrv.Start(ctx)
		if err != nil {
			return fmt.Errorf("start gui server: %w", err)
		}

		// ── 7. Open browser ─────────────────────────────────────────
		slog.Info("Opening browser", "url", guiURL)

		if err := openBrowser(guiURL); err != nil {
			slog.Warn("Failed to open browser (xdg-open may not be available)", "error", err)
			fmt.Printf("\nDashboard URL: %s\n", guiURL)
			fmt.Println("Open this URL in your browser to access the dashboard.")
		}

		// Save the GUI URL globally so tray.OnOpen can reopen it.
		globalGUIURL = guiURL

		// ── 8. System tray ──────────────────────────────────────────
		autostartEnabled := isAutostartEnabled()

		tray.Run(tray.Callbacks{
			InitiallyRunning:   proxyInitiallyStarted,
			InitiallyAutostart: autostartEnabled,
			OnOpen: func() {
				if err := openBrowser(globalGUIURL); err != nil {
					slog.Warn("Failed to open browser", "error", err)
				}
			},
			OnStart: func() {
				if err := startProxy(); err == nil {
					guiSrv.SetProxyRunning(true)
					tray.SetRunning(true)
				} else {
					guiSrv.SetProxyRunning(false)
					tray.SetRunning(false)
				}
			},
			OnStop: func() {
				_ = stopProxy()
				guiSrv.SetProxyRunning(false)
				tray.SetRunning(false)
			},
			OnAutostart: func(enabled bool) {
				if enabled {
					_ = daemon.EnableAutostart(configPath, atomic.Get().Port)
				} else {
					_ = daemon.DisableAutostart()
				}
			},
			OnQuit: func() {
				_ = stopProxy()
				cancel()
			},
		})

		return nil
	},
}

func addPlatformCommands(rootCmd *cobra.Command) {
	uiCmd.Flags().String("config", "", "Config file path")
	rootCmd.AddCommand(uiCmd)
}

func setupDefaultCommand() {
	// No-op for Linux — the binary doesn't auto-launch GUI from Finder.
}

// isAutostartEnabled checks whether autostart is enabled via .desktop file.
func isAutostartEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	desktopPath := filepath.Join(home, ".config", "autostart", daemon.LaunchAgent+".desktop")
	_, err = os.Stat(desktopPath)
	return err == nil
}
