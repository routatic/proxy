//go:build darwin && cgo

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

void triggerOpenWindow();

static inline void DispatchOpenWindow() {
    dispatch_async(dispatch_get_main_queue(), ^{
        triggerOpenWindow();
    });
}

extern void goWindowWillClose();

static inline void registerWindowCloseObserver(void* windowPtr) {
    NSWindow* win = (__bridge NSWindow*)windowPtr;
    [[NSNotificationCenter defaultCenter] addObserverForName:NSWindowWillCloseNotification
                                                      object:win
                                                       queue:[NSOperationQueue mainQueue]
                                                  usingBlock:^(NSNotification *note) {
        goWindowWillClose();
    }];
}

static inline void makeWindowKeyAndActive(void* windowPtr) {
    NSWindow* win = (__bridge NSWindow*)windowPtr;
    [NSApp activateIgnoringOtherApps:YES];
    [win makeKeyAndOrderFront:nil];
}

static inline void setupMacMenus() {
    NSMenu *mainMenu = [[NSMenu alloc] init];

    NSMenuItem *appMenuItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:appMenuItem];
    NSMenu *appMenu = [[NSMenu alloc] init];
    [appMenu addItemWithTitle:@"Quit RoutaticProxy" action:@selector(terminate:) keyEquivalent:@"q"];
    [appMenuItem setSubmenu:appMenu];

    NSMenuItem *editMenuItem = [[NSMenuItem alloc] init];
    [mainMenu addItem:editMenuItem];
    NSMenu *editMenu = [[NSMenu alloc] initWithTitle:@"Edit"];
    [editMenu addItemWithTitle:@"Undo" action:@selector(undo:) keyEquivalent:@"z"];
    [editMenu addItemWithTitle:@"Redo" action:@selector(redo:) keyEquivalent:@"Z"];
    [editMenu addItem:[NSMenuItem separatorItem]];
    [editMenu addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
    [editMenu addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
    [editMenu addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
    [editMenu addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
    [editMenuItem setSubmenu:editMenu];

    [NSApp setMainMenu:mainMenu];
}
*/
import "C"

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
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
	"github.com/webview/webview_go"
)

var (
	globalGUIURL   string
	currentWv      webview.WebView
	wvMu           sync.Mutex
	setupMenusOnce sync.Once
)

//export goWindowWillClose
func goWindowWillClose() {
	wvMu.Lock()
	wv := currentWv
	if wv == nil {
		wvMu.Unlock()
		return
	}
	currentWv = nil
	wvMu.Unlock()

	wv.Destroy()
}

//export triggerOpenWindow
func triggerOpenWindow() {
	openWebview()
}

func openWebview() {
	wvMu.Lock()
	if currentWv != nil {
		winPtr := currentWv.Window()
		C.makeWindowKeyAndActive(winPtr)
		wvMu.Unlock()
		return
	}

	currentWv = webview.New(true)
	currentWv.SetTitle("routatic-proxy Console")
	currentWv.SetSize(860, 560, webview.HintNone)

	setupMenusOnce.Do(func() {
		C.setupMacMenus()
	})

	winPtr := currentWv.Window()
	C.registerWindowCloseObserver(winPtr)
	C.makeWindowKeyAndActive(winPtr)

	currentWv.Navigate(globalGUIURL)
	wvMu.Unlock()
}

var uiCmd = &cobra.Command{
	Use:   "ui",
	Short: "Launch GUI dashboard (macOS only)",
	Long: `Start the proxy server and open the graphical dashboard.
The proxy runs in the background; closing the window keeps it running.
Use the tray icon to reopen the window or quit entirely.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		if initialConfigValid && cfg.APIKey == "" && len(cfg.APIKeys) == 0 &&
			(cfg.OpenCodeGo.APIKey == "" || strings.Contains(cfg.OpenCodeGo.APIKey, "${")) &&
			(cfg.OpenCodeZen.APIKey == "" || strings.Contains(cfg.OpenCodeZen.APIKey, "${")) {
			initialConfigValid = false
			slog.Info("Config has no valid API keys set yet, waiting for GUI configuration")
		}

		atomic := config.NewAtomicConfig(cfg, config.ResolveConfigPath())

		var captureLogger *debug.CaptureLogger
		if cfg.Logging.DebugCapture != nil && cfg.Logging.DebugCapture.Enabled {
			storage, err := debug.NewStorage(*cfg.Logging.DebugCapture)
			if err != nil {
				return fmt.Errorf("failed to create debug storage: %w", err)
			}
			captureLogger = debug.NewCaptureLogger(storage, true)
			defer func() { _ = captureLogger.Close() }()
		}

		proxySrv, err := server.NewServer(atomic, captureLogger)
		if err != nil {
			return fmt.Errorf("create proxy server: %w", err)
		}

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

			healthURL := fmt.Sprintf("http://127.0.0.1:%d/health", currentCfg.Port)
			client := &http.Client{Timeout: 2 * time.Second}
			resp, probeErr := client.Get(healthURL)
			if probeErr == nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					slog.Info("Existing proxy detected on port, connecting to it", "port", currentCfg.Port)
					isProxyRunning = true
					connectedToExisting = true
					if guiSrv != nil {
						guiSrv.SetProxyRunning(true)
					}
					tray.SetRunning(true)
					return nil
				}
			}

			isProxyRunning = true
			connectedToExisting = false
			if guiSrv != nil {
				guiSrv.SetProxyRunning(true)
			}
			tray.SetRunning(true)

			go func() {
				err := proxySrv.Start()
				proxySrvMu.Lock()
				isProxyRunning = false
				proxySrvMu.Unlock()

				if guiSrv != nil {
					guiSrv.SetProxyRunning(false)
				}
				tray.SetRunning(false)

				if err != nil && err != http.ErrServerClosed {
					slog.Error("proxy server stopped with error", "error", err)
					select {
					case proxyErrCh <- err:
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

		globalGUIURL = guiURL

		autostartEnabled := false
		if home, err := os.UserHomeDir(); err == nil {
			plistPath := filepath.Join(home, "Library", "LaunchAgents", daemon.LaunchAgent+".plist")
			_, err = os.Stat(plistPath)
			autostartEnabled = (err == nil)
		}

		go func() {
			time.Sleep(500 * time.Millisecond)
			C.DispatchOpenWindow()
		}()

		tray.Run(tray.Callbacks{
			InitiallyRunning:   proxyInitiallyStarted,
			InitiallyAutostart: autostartEnabled,
			OnOpen: func() {
				C.DispatchOpenWindow()
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
	if len(os.Args) == 1 {
		executable, err := os.Executable()
		if err == nil && strings.Contains(executable, ".app/Contents/MacOS") {
			os.Args = append(os.Args, "ui")
		}
	}
}
