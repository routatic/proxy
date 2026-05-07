// Package server manages the HTTP server lifecycle.
package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"oc-go-cc/internal/client"
	"oc-go-cc/internal/config"
	"oc-go-cc/internal/handlers"
	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/router"
	"oc-go-cc/internal/token"
)

// Server represents the proxy server.
type Server struct {
	atomic   *config.AtomicConfig
	httpSrv  *http.Server
	logger   *slog.Logger
	levelVar *slog.LevelVar
}

// NewServer creates a new proxy server.
func NewServer(atomic *config.AtomicConfig) (*Server, error) {
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

	openCodeClient := client.NewOpenCodeClient(atomic)
	modelRouter := router.NewModelRouter(atomic)
	fallbackHandler := router.NewFallbackHandler(logger, 3, 30*time.Second)

	// Create handlers.
	messagesHandler := handlers.NewMessagesHandler(
		openCodeClient,
		modelRouter,
		fallbackHandler,
		tokenCounter,
		metrics,
	)
	healthHandler := handlers.NewHealthHandler(tokenCounter, fallbackHandler, metrics)

	// Setup router.
	mux := http.NewServeMux()

	// API routes.
	mux.HandleFunc("/v1/messages", messagesHandler.HandleMessages)
	mux.HandleFunc("/v1/messages/count_tokens", healthHandler.HandleCountTokens)
	mux.HandleFunc("/health", healthHandler.HandleHealth)

	// Create HTTP server.
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	httpSrv := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  120 * time.Second,
	}

	srv := &Server{
		atomic:   atomic,
		httpSrv:  httpSrv,
		logger:   logger,
		levelVar: levelVar,
	}

	// Register callback to update log level on config reload
	atomic.OnReload(func(newCfg *config.Config) {
		levelVar.Set(parseLogLevel(newCfg.Logging.Level))
		logger.Info("log level updated", "level", newCfg.Logging.Level)
	})

	return srv, nil
}

// Start starts the server with graceful shutdown.
func (s *Server) Start() error {
	cfg := s.atomic.Get()
	s.logger.Info("starting oc-go-cc proxy",
		"host", cfg.Host,
		"port", cfg.Port,
		"base_url", cfg.OpenCodeGo.BaseURL,
	)

	// Graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		s.logger.Info("shutting down server...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := s.httpSrv.Shutdown(shutdownCtx); err != nil {
			s.logger.Error("server shutdown failed", "error", err)
		}
	}()

	if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("server failed: %w", err)
	}

	s.logger.Info("server stopped")
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
