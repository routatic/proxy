// Package server manages the HTTP server lifecycle.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/debug"
	"github.com/routatic/proxy/internal/gui"
	"github.com/routatic/proxy/internal/handlers"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/provider"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/status"
	"github.com/routatic/proxy/internal/storage"
	"github.com/routatic/proxy/internal/token"
)

// Server represents the proxy server.
type Server struct {
	atomic    *config.AtomicConfig
	httpSrv   *http.Server
	mux       http.Handler
	mu        sync.Mutex
	logger    *slog.Logger
	levelVar  *slog.LevelVar
	History   *history.History // exported so the ui command can read it
	metrics   *metrics.Metrics // stored for Metrics() getter
	storage   *storage.Database
	retention *storage.Retention
}

// NewServer creates a new proxy server.
func NewServer(atomic *config.AtomicConfig, captureLogger *debug.CaptureLogger) (*Server, error) {
	cfg := atomic.Get()
	levelVar := new(slog.LevelVar)
	levelVar.Set(parseLogLevel(cfg.Logging.Level))

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: levelVar,
	}))
	slog.SetDefault(logger)

	// Initialize components.
	tokenCounter, err := token.NewCounter()
	if err != nil {
		return nil, fmt.Errorf("failed to create token counter: %w", err)
	}

	// Create metrics
	metrics := metrics.New()

	openCodeClient := client.NewOpenCodeClient(atomic, captureLogger)
	fallbackHandler := router.NewFallbackHandler(logger, 3, 30*time.Second)
	fallbackHandler.SetAtomicConfig(atomic)

	// Register providers.
	providerRegistry := core.NewProviderRegistry()
	_ = providerRegistry.Register(provider.NewOpenCodeGoProvider(atomic))
	_ = providerRegistry.Register(provider.NewOpenCodeZenProvider(atomic))
	_ = providerRegistry.Register(provider.NewAWSBedrockProvider(atomic))

	// Create status store for the statusline endpoint.
	statusStore := status.NewStore(0)

	// Create history ring buffer (1000 entries, in-memory).
	hist := history.New(1000)

	// Initialize SQLite storage first so catalog can use it.
	var db *storage.Database
	var retention *storage.Retention
	storageCfg := storage.DefaultConfig
	if cfg.Storage != nil {
		storageCfg = storage.Config{
			DatabasePath:    cfg.Storage.DatabasePath,
			RetentionDays:   cfg.Storage.RetentionDays,
			VacuumOnStartup: cfg.Storage.VacuumOnStartup,
			WALEnabled:      cfg.Storage.WALEnabled,
		}
	}
	if storageCfg.DatabasePath != "" {
		db, err = storage.Open(storageCfg)
		if err != nil {
			logger.Warn("failed to open storage database, falling back to in-memory", "error", err)
		} else {
			logger.Info("storage database opened", "path", db.Path())
			retention = storage.NewRetention(db, storageCfg.RetentionDays)
			retention.Start()
		}
	}

	// Create model router with SQLite catalog support.
	var modelRouter *router.ModelRouter
	if db != nil {
		modelRouter = router.NewModelRouterWithDB(atomic, db)
	} else {
		modelRouter = router.NewModelRouter(atomic)
	}

	// Create handlers.
	var storageWriter handlers.StorageWriter
	if db != nil {
		storageWriter = handlers.NewStorageAdapter(db)
	}

	messagesHandler := handlers.NewMessagesHandler(
		openCodeClient,
		providerRegistry,
		modelRouter,
		fallbackHandler,
		tokenCounter,
		metrics,
		captureLogger,
		hist,
		storageWriter,
	)
	healthHandler := handlers.NewHealthHandler(tokenCounter, fallbackHandler, metrics, statusStore)
	modelsHandler := handlers.NewModelsHandler(modelRouter)

	// Setup router.
	mux := http.NewServeMux()

	// API routes.
	mux.Handle("/v1/messages", handlers.NewAnthropicFirstHandler(atomic, http.HandlerFunc(messagesHandler.HandleMessages)))
	mux.HandleFunc("/v1/messages/count_tokens", healthHandler.HandleCountTokens)
	mux.HandleFunc("/v1/models", modelsHandler.HandleListModels)
	mux.HandleFunc("/health", healthHandler.HandleHealth)
	mux.HandleFunc("/statusline", healthHandler.HandleStatusline)

	// Analytics endpoints (only when SQLite storage is available)
	if db != nil {
		analyticsHandler := gui.NewAnalyticsHandler(db)
		mux.HandleFunc("/api/analytics/summary", analyticsHandler.Summary)
		mux.HandleFunc("/api/analytics/tokens/trend", analyticsHandler.TokenTrend)
		mux.HandleFunc("/api/analytics/latency", analyticsHandler.LatencyStats)
	}

	// Create HTTP server.
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	httpSrv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		ReadTimeout: 120 * time.Second,
		// WriteTimeout is disabled (zero). Long-running SSE streams must not be
		// killed mid-flight. Stuck upstream connections are handled by the
		// per-stream idle watchdog (transformer/idle.go) which cancels the
		// upstream context when no bytes arrive within the model's idle timeout.
		// IdleTimeout here governs keep-alive between separate HTTP requests on
		// the same TCP connection; it does NOT affect in-stream byte gaps.
		WriteTimeout: 0,
		IdleTimeout:  300 * time.Second,
	}

	srv := &Server{
		atomic:    atomic,
		httpSrv:   httpSrv,
		mux:       mux,
		logger:    logger,
		levelVar:  levelVar,
		History:   hist,
		metrics:   metrics,
		storage:   db,
		retention: retention,
	}

	// Register callback to update log level on config reload
	atomic.OnReload(func(newCfg *config.Config) {
		levelVar.Set(parseLogLevel(newCfg.Logging.Level))
		logger.Info("log level updated", "level", newCfg.Logging.Level)
	})

	return srv, nil
}

// Metrics returns the in-process metrics collector.
func (s *Server) Metrics() *metrics.Metrics {
	return s.metrics
}

// Storage returns the SQLite storage instance.
func (s *Server) Storage() *storage.Database {
	return s.storage
}

// Start starts the server with graceful shutdown.
func (s *Server) Start() error {
	cfg := s.atomic.Get()
	s.logger.Info("starting routatic-proxy",
		"host", cfg.Host,
		"port", cfg.Port,
		"base_url", cfg.OpenCodeGo.BaseURL,
	)

	s.mu.Lock()
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	s.httpSrv = &http.Server{
		Addr:         addr,
		Handler:      s.mux,
		ReadTimeout:  120 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  300 * time.Second,
	}
	s.mu.Unlock()

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		s.logger.Info("shutting down server...")

		if s.retention != nil {
			s.retention.Stop()
		}

		if s.storage != nil {
			_ = s.storage.Close()
		}

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		s.mu.Lock()
		srvToShutdown := s.httpSrv
		s.mu.Unlock()

		if srvToShutdown != nil {
			if err := srvToShutdown.Shutdown(shutdownCtx); err != nil {
				s.logger.Error("server shutdown failed", "error", err)
			}
		}
	}()

	s.mu.Lock()
	srvToStart := s.httpSrv
	s.mu.Unlock()

	if err := srvToStart.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}

	s.logger.Info("server stopped")
	return nil
}

// Shutdown gracefully shuts down the proxy server.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("programmatic shutdown requested")
	s.mu.Lock()
	srvToShutdown := s.httpSrv
	s.mu.Unlock()

	if srvToShutdown != nil {
		return srvToShutdown.Shutdown(ctx)
	}
	return nil
}

// WritePID writes the current PID to a file.
func WritePID(path string) error {
	pid := os.Getpid()
	return os.WriteFile(path, []byte(fmt.Sprintf("%d", pid)), 0644)
}

// ReadPID reads the PID from a file.
func ReadPID(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	var pid int
	_, err = fmt.Sscanf(string(data), "%d", &pid)
	return pid, err
}

// parseLogLevel converts a string log level to slog.Level.
func parseLogLevel(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
