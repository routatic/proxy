// Package gui provides the embedded HTTP server that serves the GUI dashboard
// and exposes /api/* endpoints for metrics, history, and configuration.
package gui

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/daemon"
	"github.com/routatic/proxy/internal/history"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/storage"
)

//go:embed assets/*
var assets embed.FS

// Config is the GUI-level configuration that the user can toggle at runtime.
type Config struct {
	Autostart bool `json:"autostart"`
	Notify    bool `json:"notify"`
}

// Server is the embedded HTTP server that backs the webview UI.
type Server struct {
	hist              *history.History
	met               *metrics.Metrics
	atomicCfg         *config.AtomicConfig
	cfg               Config
	cfgMu             sync.RWMutex
	proxyRunning      atomic.Bool
	connectedExisting atomic.Bool
	proxyPort         int
	startProxy        func() error
	stopProxy         func() error
	catalogDir        string
	catalogSourceURL  string
	srv               *http.Server
	logger            *slog.Logger
	catalogMu         sync.Mutex

	storage *storage.Database
}

// Options configures the GUI server.
type Options struct {
	History          *history.History
	Metrics          *metrics.Metrics
	AtomicConfig     *config.AtomicConfig
	ProxyPort        int
	StartProxy       func() error
	StopProxy        func() error
	CatalogDir       string
	CatalogSourceURL string
	Logger           *slog.Logger
	Storage          *storage.Database
}

// New creates a new GUI server.
func New(opts Options) *Server {
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	s := &Server{
		hist:             opts.History,
		met:              opts.Metrics,
		atomicCfg:        opts.AtomicConfig,
		proxyPort:        opts.ProxyPort,
		startProxy:       opts.StartProxy,
		stopProxy:        opts.StopProxy,
		catalogDir:       opts.CatalogDir,
		catalogSourceURL: opts.CatalogSourceURL,
		logger:           opts.Logger,

		storage: opts.Storage,
	}
	// Check initial autostart state.
	s.cfg.Autostart = isAutostartEnabled()
	return s
}

// isAutostartEnabled checks whether autostart is currently enabled.
// On macOS it checks ~/Library/LaunchAgents/{LaunchAgent}.plist.
// On Linux it checks ~/.config/autostart/{LaunchAgent}.desktop.
func isAutostartEnabled() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}

	if runtime.GOOS == "darwin" {
		plist := filepath.Join(home, "Library", "LaunchAgents", daemon.LaunchAgent+".plist")
		_, err = os.Stat(plist)
		return err == nil
	}

	if runtime.GOOS == "linux" {
		desktop := filepath.Join(home, ".config", "autostart", daemon.LaunchAgent+".desktop")
		_, err = os.Stat(desktop)
		return err == nil
	}

	return false
}

// SetProxyRunning updates the running state (called by the proxy lifecycle).
func (s *Server) SetProxyRunning(running bool) {
	s.proxyRunning.Store(running)
}

// SetConnectedToExisting updates whether the GUI is monitoring an external proxy
// rather than controlling a locally-started one.
func (s *Server) SetConnectedToExisting(connected bool) {
	s.connectedExisting.Store(connected)
}

// getProxyPort returns the current proxy port from config if available.
func (s *Server) getProxyPort() int {
	if s.atomicCfg != nil {
		return s.atomicCfg.Get().Port
	}
	return s.proxyPort
}

// Start starts the embedded HTTP server on a random localhost port and returns
// the URL that the webview should load.
func (s *Server) Start(ctx context.Context) (string, error) {
	mux := http.NewServeMux()

	// Static assets — strip the "assets/" prefix so index.html is served at /.
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return "", fmt.Errorf("gui assets embed: %w", err)
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	// API endpoints.
	mux.HandleFunc("/api/metrics", s.handleMetrics)
	mux.HandleFunc("/api/history", s.handleHistory)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/proxy/config", s.handleProxyConfig)
	mux.HandleFunc("/api/proxy/start", s.handleProxyStart)
	mux.HandleFunc("/api/proxy/stop", s.handleProxyStop)
	mux.HandleFunc("/api/catalog/lock", s.handleCatalogLock)
	mux.HandleFunc("/api/catalog/sync", s.handleCatalogSync)

	// New endpoints for advanced GUI features

	mux.HandleFunc("/api/config/export", s.handleConfigExport)
	mux.HandleFunc("/api/config/import", s.handleConfigImport)
	mux.HandleFunc("/api/perf/models", s.handlePerformance)
	mux.HandleFunc("/api/perf/aggregate", s.handlePerformanceAggregate)
	mux.HandleFunc("/api/catalog/stats", s.handleCatalogStats)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("gui server listen: %w", err)
	}

	// Wrap with security headers middleware.
	s.srv = &http.Server{Handler: securityHeadersMiddleware(mux)}
	go func() {
		if srvErr := s.srv.Serve(ln); srvErr != nil && srvErr != http.ErrServerClosed {
			s.logger.Error("gui server error", "err", srvErr)
		}
	}()

	go func() {
		<-ctx.Done()
		_ = s.srv.Close()
	}()

	url := "http://" + ln.Addr().String() + "/"
	s.logger.Info("gui server started", "url", url)
	return url, nil
}

// ── API handlers ──────────────────────────────────────────────────────────────

type metricsResponse struct {
	ProxyRunning      bool             `json:"proxy_running"`
	ConnectedExisting bool             `json:"connected_to_existing"`
	Port              int              `json:"port"`
	RequestsReceived  int64            `json:"requests_received"`
	RequestsStreamed  int64            `json:"requests_streamed"`
	RequestsSuccess   int64            `json:"requests_success"`
	RequestsFailed    int64            `json:"requests_failed"`
	ModelCounts       map[string]int64 `json:"model_counts"`
}

func (s *Server) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	var snap metrics.Snapshot
	if s.met != nil {
		snap = s.met.GetSnapshot()
	}
	resp := metricsResponse{
		ProxyRunning:      s.proxyRunning.Load(),
		ConnectedExisting: s.connectedExisting.Load(),
		Port:              s.getProxyPort(),
		RequestsReceived:  snap.RequestsReceived,
		RequestsStreamed:  snap.RequestsStreamed,
		RequestsSuccess:   snap.RequestsSuccess,
		RequestsFailed:    snap.RequestsFailed,
		ModelCounts:       snap.ModelCounts,
	}
	writeJSON(w, resp)
}

type historyEntry struct {
	ID           string `json:"id"`
	Model        string `json:"model"`
	Provider     string `json:"provider"`
	Scenario     string `json:"scenario"`
	StartTime    string `json:"start_time"` // RFC3339
	DurationMs   int64  `json:"duration_ms"`
	InputTokens  int    `json:"input_tokens"`
	OutputTokens int    `json:"output_tokens"`
	Streaming    bool   `json:"streaming"`
	Success      bool   `json:"success"`
	ErrorMsg     string `json:"error_msg,omitempty"`
}

func (s *Server) handleHistory(w http.ResponseWriter, _ *http.Request) {
	if s.hist == nil {
		writeJSON(w, []historyEntry{})
		return
	}
	records := s.hist.Last(200)
	out := make([]historyEntry, len(records))
	for i, rec := range records {
		out[i] = historyEntry{
			ID:           rec.ID,
			Model:        rec.Model,
			Provider:     rec.Provider,
			Scenario:     rec.Scenario,
			StartTime:    rec.StartTime.Format("2006-01-02T15:04:05Z07:00"),
			DurationMs:   rec.Duration.Milliseconds(),
			InputTokens:  rec.InputTokens,
			OutputTokens: rec.OutputTokens,
			Streaming:    rec.Streaming,
			Success:      rec.Success,
			ErrorMsg:     rec.ErrorMsg,
		}
	}
	writeJSON(w, out)
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.cfgMu.RLock()
		cfg := s.cfg
		s.cfgMu.RUnlock()
		writeJSON(w, cfg)

	case http.MethodPost:
		var req struct {
			Autostart *bool `json:"autostart"`
			Notify    *bool `json:"notify"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		s.cfgMu.Lock()
		if req.Autostart != nil {
			s.cfg.Autostart = *req.Autostart
			if *req.Autostart {
				_ = daemon.EnableAutostart("", s.getProxyPort())
			} else {
				_ = daemon.DisableAutostart()
			}
		}
		if req.Notify != nil {
			s.cfg.Notify = *req.Notify
		}
		s.cfgMu.Unlock()
		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleProxyStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.proxyRunning.Load() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.startProxy != nil {
		if err := s.startProxy(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProxyStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.proxyRunning.Load() {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if s.stopProxy != nil {
		if err := s.stopProxy(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleProxyConfig(w http.ResponseWriter, r *http.Request) {
	if s.atomicCfg == nil {
		http.Error(w, "proxy config not available", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		cfg := s.atomicCfg.Get()
		writeJSON(w, cfg)

	case http.MethodPost:
		// Read the current config from disk as the baseline.
		configPath := s.atomicCfg.Path()
		currentCfg, err := config.LoadFromPath(configPath)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to read current config: %v", err), http.StatusInternalServerError)
			return
		}

		// Decode only the fields the client sent (partial update).
		var patch map[string]json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&patch); err != nil {
			http.Error(w, fmt.Sprintf("invalid config format: %v", err), http.StatusBadRequest)
			return
		}

		// Apply each patch field onto the current config.
		patchBytes, err := json.Marshal(patch)
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to re-encode patch: %v", err), http.StatusInternalServerError)
			return
		}
		if err := json.Unmarshal(patchBytes, currentCfg); err != nil {
			http.Error(w, fmt.Sprintf("failed to apply patch: %v", err), http.StatusInternalServerError)
			return
		}

		// Validate essential fields.
		if currentCfg.Host == "" {
			http.Error(w, "host is required", http.StatusBadRequest)
			return
		}
		if currentCfg.Port < 1 || currentCfg.Port > 65535 {
			http.Error(w, "port must be between 1 and 65535", http.StatusBadRequest)
			return
		}

		// Serialize and write the merged config.
		data, err := json.MarshalIndent(currentCfg, "", "  ")
		if err != nil {
			http.Error(w, fmt.Sprintf("failed to serialize config: %v", err), http.StatusInternalServerError)
			return
		}
		if err := os.WriteFile(configPath, data, 0600); err != nil {
			http.Error(w, fmt.Sprintf("failed to write config file: %v", err), http.StatusInternalServerError)
			return
		}

		// Reload configuration atomically so the running proxy picks up changes.
		if err := s.atomicCfg.Reload(); err != nil {
			http.Error(w, fmt.Sprintf("failed to reload config: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type catalogLockResponse struct {
	SyncedAt   *time.Time `json:"synced_at,omitempty"`
	SHA256     string     `json:"sha256,omitempty"`
	Bytes      int64      `json:"bytes,omitempty"`
	TTLHours   int        `json:"ttl_hours,omitempty"`
	AgeSeconds int64      `json:"age_seconds"`
	Synced     bool       `json:"synced"`
}

func (s *Server) handleCatalogLock(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	lock, err := catalog.ReadLock(s.catalogDir)
	if err != nil {
		writeJSON(w, catalogLockResponse{Synced: false, AgeSeconds: -1})
		return
	}

	age := time.Since(lock.SyncedAt)
	resp := catalogLockResponse{
		SyncedAt:   &lock.SyncedAt,
		SHA256:     lock.SHA256,
		Bytes:      lock.Bytes,
		TTLHours:   lock.TTLHours,
		AgeSeconds: int64(age.Seconds()),
		Synced:     true,
	}
	writeJSON(w, resp)
}

func (s *Server) handleCatalogSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.catalogSourceURL == "" || s.catalogDir == "" {
		http.Error(w, "catalog sync is not configured", http.StatusServiceUnavailable)
		return
	}

	// Serialize manual syncs so the lock file and on-disk catalog stay consistent.
	s.catalogMu.Lock()
	defer s.catalogMu.Unlock()

	lock, err := catalog.Sync(s.catalogSourceURL, s.catalogDir)
	if err != nil {
		http.Error(w, fmt.Sprintf("catalog sync failed: %v", err), http.StatusInternalServerError)
		return
	}

	age := time.Since(lock.SyncedAt)
	writeJSON(w, catalogLockResponse{
		SyncedAt:   &lock.SyncedAt,
		SHA256:     lock.SHA256,
		Bytes:      lock.Bytes,
		TTLHours:   lock.TTLHours,
		AgeSeconds: int64(age.Seconds()),
		Synced:     true,
	})
}

func (s *Server) handleCatalogStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.storage == nil {
		writeJSON(w, map[string]any{
			"available": false,
			"error":     "storage not configured",
		})
		return
	}

	repo := storage.NewCatalogRepo(s.storage)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	idx, err := repo.Load(ctx)
	if err != nil {
		writeJSON(w, map[string]any{
			"available": false,
			"error":     err.Error(),
		})
		return
	}

	lastSync, _ := repo.LastSync(ctx)

	providersByEnabled := make(map[string]int)
	for _, p := range idx.Providers {
		enabled := "disabled"
		if p.Enabled != nil && *p.Enabled {
			enabled = "enabled"
		}
		providersByEnabled[enabled]++
	}

	modelsByProvider := make(map[string]int)
	for prov := range idx.Providers {
		modelsByProvider[prov] = len(idx.ProviderModels[prov])
	}

	totalModels := len(idx.Models)
	modelsWithTools := 0
	modelsWithVision := 0
	modelsWithReasoning := 0
	for _, m := range idx.Models {
		if m.ToolCall {
			modelsWithTools++
		}
		if m.Vision {
			modelsWithVision++
		}
		if m.Reasoning {
			modelsWithReasoning++
		}
	}

	resp := map[string]any{
		"available":             true,
		"last_sync":             lastSync,
		"total_providers":       len(idx.Providers),
		"providers_enabled":     providersByEnabled["enabled"],
		"providers_disabled":    providersByEnabled["disabled"],
		"total_models":          totalModels,
		"models_with_tools":     modelsWithTools,
		"models_with_vision":    modelsWithVision,
		"models_with_reasoning": modelsWithReasoning,
		"models_by_provider":    modelsByProvider,
	}

	writeJSON(w, resp)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// securityHeadersMiddleware adds security headers to all responses.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME-type sniffing.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		// Restrict scripts/styles to same origin (local GUI only).
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}
