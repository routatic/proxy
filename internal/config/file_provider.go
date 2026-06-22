// Package config provides configuration management for the routatic-proxy.
package config

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/routatic/proxy/internal/auth"
	"gopkg.in/yaml.v3"
)

// FileConfigProvider reads configuration from YAML or JSON files and provides
// hot reload capabilities via file watching. It uses atomic pointer swapping
// for thread-safe config updates without blocking readers.
type FileConfigProvider struct {
	configPath string
	mu         sync.RWMutex
	runtime    *RuntimeConfig
	modTime    time.Time
	watcher    *fsnotify.Watcher
	stopCh     chan struct{}
	wg         sync.WaitGroup
}

// FileConfig represents the on-disk configuration file format.
// It supports both YAML and JSON encoding.
type FileConfig struct {
	Version    string                     `json:"version" yaml:"version"`
	Workspaces map[string]WorkspaceConfig `json:"workspaces" yaml:"workspaces"`
}

// WorkspaceConfig defines configuration for a single workspace/tenant.
type WorkspaceConfig struct {
	Supermodels map[string]SupermodelFileConfig `json:"supermodels" yaml:"supermodels"`
	Providers   map[string]ProviderFileConfig   `json:"providers" yaml:"providers"`
	Enforcement EnforcementFileConfig           `json:"enforcement" yaml:"enforcement"`
	Logging     LoggingFileConfig               `json:"logging" yaml:"logging"`
}

// SupermodelFileConfig represents the file format for supermodel configuration.
type SupermodelFileConfig struct {
	Name        string                        `json:"name" yaml:"name"`
	Description string                        `json:"description,omitempty" yaml:"description,omitempty"`
	Default     ModelFileConfig               `json:"default" yaml:"default"`
	Scenarios   map[string]ScenarioFileConfig `json:"scenarios,omitempty" yaml:"scenarios,omitempty"`
}

// ModelFileConfig represents model configuration in file format (with YAML tags).
type ModelFileConfig struct {
	Provider        string  `json:"provider" yaml:"provider"`
	ModelID         string  `json:"model_id" yaml:"model_id"`
	Temperature     float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens       int     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty" yaml:"max_output_tokens,omitempty"`
	ContextWindow   int     `json:"context_window,omitempty" yaml:"context_window,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
	SupportsTools   *bool   `json:"supports_tools,omitempty" yaml:"supports_tools,omitempty"`
	WireFormat      string  `json:"wire_format,omitempty" yaml:"wire_format,omitempty"`
	Vision          bool    `json:"vision,omitempty" yaml:"vision,omitempty"`
}

// ScenarioFileConfig represents scenario configuration in file format (with YAML tags).
type ScenarioFileConfig struct {
	Provider        string  `json:"provider" yaml:"provider"`
	ModelID         string  `json:"model_id" yaml:"model_id"`
	Temperature     float64 `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens       int     `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	MaxOutputTokens int     `json:"max_output_tokens,omitempty" yaml:"max_output_tokens,omitempty"`
	ContextWindow   int     `json:"context_window,omitempty" yaml:"context_window,omitempty"`
	ReasoningEffort string  `json:"reasoning_effort,omitempty" yaml:"reasoning_effort,omitempty"`
}

// ProviderFileConfig represents provider configuration in file format (with YAML tags).
type ProviderFileConfig struct {
	Name             string            `json:"name" yaml:"name"`
	Type             string            `json:"type" yaml:"type"` // "opencode-go", "opencode-zen", "aws-bedrock", etc.
	BaseURL          string            `json:"base_url" yaml:"base_url"`
	AnthropicBaseURL string            `json:"anthropic_base_url,omitempty" yaml:"anthropic_base_url,omitempty"`
	ResponsesBaseURL string            `json:"responses_base_url,omitempty" yaml:"responses_base_url,omitempty"`
	GeminiBaseURL    string            `json:"gemini_base_url,omitempty" yaml:"gemini_base_url,omitempty"`
	APIKey           string            `json:"api_key,omitempty" yaml:"api_key,omitempty"`
	APIKeys          []string          `json:"api_keys,omitempty" yaml:"api_keys,omitempty"`
	TimeoutMs        int               `json:"timeout_ms" yaml:"timeout_ms"`
	StreamTimeoutMs  int               `json:"stream_timeout_ms,omitempty" yaml:"stream_timeout_ms,omitempty"`
	Headers          map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// LoggingFileConfig represents logging policy in file format (with YAML tags).
type LoggingFileConfig struct {
	Level            string   `json:"level" yaml:"level"` // "debug", "info", "warn", "error"
	LogRequests      bool     `json:"log_requests" yaml:"log_requests"`
	LogResponses     bool     `json:"log_responses" yaml:"log_responses"`
	LogLatency       bool     `json:"log_latency" yaml:"log_latency"`
	LogRateLimits    bool     `json:"log_rate_limits" yaml:"log_rate_limits"`
	PIIMasking       bool     `json:"pii_masking" yaml:"pii_masking"`
	SensitiveHeaders []string `json:"sensitive_headers,omitempty" yaml:"sensitive_headers,omitempty"`
}

// EnforcementFileConfig represents enforcement policy in file format (with YAML tags).
type EnforcementFileConfig struct {
	RequireAuth           bool `json:"require_auth" yaml:"require_auth"`
	EnforceModelAllowlist bool `json:"enforce_model_allowlist" yaml:"enforce_model_allowlist"`
	EnforceBudgets        bool `json:"enforce_budgets" yaml:"enforce_budgets"`
	EnforceRateLimits     bool `json:"enforce_rate_limits" yaml:"enforce_rate_limits"`
}

// NewFileConfigProvider creates a new FileConfigProvider that reads from the
// specified config file path. The file can be in YAML or JSON format.
// The provider immediately loads the configuration and starts watching
// the file for changes if hot reload is desired.
func NewFileConfigProvider(configPath string) (*FileConfigProvider, error) {
	if configPath == "" {
		return nil, fmt.Errorf("config path cannot be empty")
	}

	// Expand ~ to home directory if present
	configPath = expandHome(configPath)

	// Verify file exists and is readable
	info, err := os.Stat(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", configPath)
		}
		return nil, fmt.Errorf("cannot stat config file: %w", err)
	}

	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("config path is not a file: %s", configPath)
	}

	p := &FileConfigProvider{
		configPath: configPath,
		stopCh:     make(chan struct{}),
	}

	// Initial load
	if err := p.Reload(); err != nil {
		return nil, fmt.Errorf("initial config load failed: %w", err)
	}

	slog.Info("FileConfigProvider initialized", "path", configPath)

	return p, nil
}

// NewFileConfigProviderWithWatch creates a new FileConfigProvider and starts
// watching the config file for changes. Use this when you want automatic
// hot reload capabilities.
func NewFileConfigProviderWithWatch(configPath string) (*FileConfigProvider, error) {
	p, err := NewFileConfigProvider(configPath)
	if err != nil {
		return nil, err
	}

	if err := p.StartWatching(); err != nil {
		return nil, fmt.Errorf("failed to start file watcher: %w", err)
	}

	return p, nil
}

// GetEffectiveConfig returns the runtime configuration for the authenticated request.
// The auth context determines which workspace configuration to use.
// This method is thread-safe and uses RWMutex for concurrent access.
func (p *FileConfigProvider) GetEffectiveConfig(ctx context.Context, authCtx *auth.AuthContext) (*RuntimeConfig, error) {
	if authCtx == nil {
		return nil, fmt.Errorf("auth context is nil")
	}

	p.mu.RLock()
	rt := p.runtime
	p.mu.RUnlock()

	if rt == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	// Lock for potential modification based on auth context
	p.mu.Lock()
	defer p.mu.Unlock()

	// If we have a workspace-specific config and the auth context specifies a workspace,
	// we could return a modified runtime config here. For now, we return the current
	// runtime config which represents the compiled configuration.

	slog.Debug("GetEffectiveConfig called",
		"workspace_id", authCtx.WorkspaceID,
		"config_version", rt.Version,
	)

	return rt, nil
}

// GetConfigByRef retrieves a specific configuration version by reference.
// For FileConfigProvider, this simply returns the current configuration
// as file-based configs don't support versioning.
func (p *FileConfigProvider) GetConfigByRef(ctx context.Context, ref auth.ConfigRef) (*RuntimeConfig, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.runtime == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	// For file-based config, we only support the current version
	// Version matching is advisory - we return current config regardless
	slog.Debug("GetConfigByRef called",
		"workspace_id", ref.WorkspaceID,
		"requested_version", ref.Version,
		"current_version", p.runtime.Version,
	)

	return p.runtime, nil
}

// Invalidate clears any cached configuration for the specified workspace and version.
// For FileConfigProvider, this triggers a reload from disk.
func (p *FileConfigProvider) Invalidate(ctx context.Context, workspaceID string, version string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	slog.Info("Invalidating config cache, triggering reload",
		"workspace_id", workspaceID,
		"version", version,
	)

	return p.Reload()
}

// HealthCheck verifies the provider is operational by checking:
// 1. The config file is still readable
// 2. The configuration is loaded
// 3. The configuration passes validation
func (p *FileConfigProvider) HealthCheck(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	// Check file is still accessible
	info, err := os.Stat(p.configPath)
	if err != nil {
		return fmt.Errorf("config file not accessible: %w", err)
	}

	p.mu.RLock()
	rt := p.runtime
	modTime := p.modTime
	p.mu.RUnlock()

	if rt == nil {
		return fmt.Errorf("configuration not loaded")
	}

	// Warn if file has been modified since last reload
	if info.ModTime().After(modTime) {
		slog.Warn("config file has been modified but not reloaded",
			"path", p.configPath,
			"file_mod_time", info.ModTime(),
			"loaded_mod_time", modTime,
		)
	}

	return nil
}

// Reload reloads the configuration from disk atomically.
// This method is safe for concurrent use - it loads to a temporary
// RuntimeConfig and swaps the pointer only after successful validation.
func (p *FileConfigProvider) Reload() error {
	slog.Info("Reloading configuration", "path", p.configPath)

	// Get file info before loading
	info, err := os.Stat(p.configPath)
	if err != nil {
		return fmt.Errorf("cannot stat config file: %w", err)
	}

	// Load and parse the file
	fileCfg, err := p.loadFile(p.configPath)
	if err != nil {
		return fmt.Errorf("failed to load config file: %w", err)
	}

	// Validate the file configuration
	if err := p.validateFileConfig(fileCfg); err != nil {
		return fmt.Errorf("config validation failed: %w", err)
	}

	// Compile to RuntimeConfig
	runtimeCfg, err := p.compileToRuntime(fileCfg)
	if err != nil {
		return fmt.Errorf("failed to compile runtime config: %w", err)
	}

	// Atomic swap: lock, update, unlock
	p.mu.Lock()
	oldRuntime := p.runtime
	p.runtime = runtimeCfg
	p.modTime = info.ModTime()
	p.mu.Unlock()

	if oldRuntime != nil {
		slog.Info("Configuration reloaded successfully",
			"version", runtimeCfg.Version,
			"previous_version", oldRuntime.Version,
		)
	} else {
		slog.Info("Configuration loaded successfully",
			"version", runtimeCfg.Version,
		)
	}

	return nil
}

// StartWatching starts watching the config file for changes.
// This enables hot reload when the file is modified.
func (p *FileConfigProvider) StartWatching() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.watcher != nil {
		return fmt.Errorf("file watcher already started")
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to create file watcher: %w", err)
	}

	// Watch the directory, not the file itself, to handle editors that
	// save by renaming/creating a new file
	dir := filepath.Dir(p.configPath)
	if err := watcher.Add(dir); err != nil {
		_ = watcher.Close()
		return fmt.Errorf("failed to watch config directory: %w", err)
	}

	p.watcher = watcher

	// Start the watch goroutine
	p.wg.Add(1)
	go p.watchLoop()

	slog.Info("Started watching config file for changes", "path", p.configPath)
	return nil
}

// StopWatching stops the file watcher.
func (p *FileConfigProvider) StopWatching() error {
	p.mu.Lock()
	watcher := p.watcher
	p.watcher = nil
	p.mu.Unlock()

	if watcher == nil {
		return nil // Not an error if not watching
	}

	close(p.stopCh)

	// Close the watcher to unblock the watchLoop
	_ = watcher.Close()

	// Wait for the watch loop to exit
	p.wg.Wait()

	slog.Info("Stopped watching config file")
	return nil
}

// watchLoop runs in a goroutine and watches for file changes.
func (p *FileConfigProvider) watchLoop() {
	defer p.wg.Done()

	// Get local reference to watcher - it can be nilled by StopWatching
	p.mu.RLock()
	watcher := p.watcher
	p.mu.RUnlock()

	if watcher == nil {
		return
	}

	filename := filepath.Base(p.configPath)
	debounceTimer := time.NewTimer(0)
	<-debounceTimer.C // Drain initial timer

	for {
		select {
		case <-p.stopCh:
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Only care about events for our specific config file
			if filepath.Base(event.Name) != filename {
				continue
			}

			// Filter for relevant event types
			if !event.Has(fsnotify.Write) && !event.Has(fsnotify.Create) && !event.Has(fsnotify.Rename) {
				continue
			}

			// Debounce: cancel pending timer and start a new one
			if !debounceTimer.Stop() {
				select {
				case <-debounceTimer.C:
				default:
				}
			}
			debounceTimer.Reset(500 * time.Millisecond)

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			slog.Error("Config file watcher error", "error", err)

		case <-debounceTimer.C:
			slog.Info("Config file changed, reloading", "path", p.configPath)
			if err := p.Reload(); err != nil {
				slog.Error("Config reload failed", "error", err)
			} else {
				slog.Info("Config reloaded successfully")
			}
		}
	}
}

// loadFile reads and parses the configuration file.
// It automatically detects the format (YAML or JSON) based on file extension.
func (p *FileConfigProvider) loadFile(path string) (*FileConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg FileConfig
	ext := strings.ToLower(filepath.Ext(path))

	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing YAML: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &cfg); err != nil {
			return nil, fmt.Errorf("parsing JSON: %w", err)
		}
	default:
		// Try YAML first, then JSON
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			if err := json.Unmarshal(data, &cfg); err != nil {
				return nil, fmt.Errorf("parsing config (tried YAML and JSON): %w", err)
			}
		}
	}

	return &cfg, nil
}

// validateFileConfig performs validation on the loaded file configuration.
func (p *FileConfigProvider) validateFileConfig(cfg *FileConfig) error {
	if cfg.Version == "" {
		return fmt.Errorf("version is required")
	}

	if len(cfg.Workspaces) == 0 {
		return fmt.Errorf("at least one workspace is required")
	}

	// Validate each workspace
	for wsID, wsCfg := range cfg.Workspaces {
		if err := p.validateWorkspaceConfig(wsID, wsCfg); err != nil {
			return fmt.Errorf("workspace %q: %w", wsID, err)
		}
	}

	return nil
}

// validateWorkspaceConfig validates a single workspace configuration.
func (p *FileConfigProvider) validateWorkspaceConfig(_wsID string, wsCfg WorkspaceConfig) error {
	_ = _wsID // may be used for error messages in future
	if len(wsCfg.Supermodels) == 0 {
		return fmt.Errorf("at least one supermodel is required")
	}

	if len(wsCfg.Providers) == 0 {
		return fmt.Errorf("at least one provider is required")
	}

	// Validate supermodels
	for smName, smCfg := range wsCfg.Supermodels {
		if err := p.validateSupermodelConfig(smName, smCfg); err != nil {
			return fmt.Errorf("supermodel %q: %w", smName, err)
		}
	}

	// Validate providers
	for providerName, providerCfg := range wsCfg.Providers {
		if err := p.validateProviderConfig(providerName, providerCfg); err != nil {
			return fmt.Errorf("provider %q: %w", providerName, err)
		}
	}

	return nil
}

// validateSupermodelConfig validates a single supermodel configuration.
func (p *FileConfigProvider) validateSupermodelConfig(_name string, smCfg SupermodelFileConfig) error {
	_ = _name
	if smCfg.Name == "" {
		return fmt.Errorf("supermodel name is required")
	}

	if smCfg.Default.Provider == "" {
		return fmt.Errorf("default provider is required")
	}

	if smCfg.Default.ModelID == "" {
		return fmt.Errorf("default model_id is required")
	}

	// Validate scenarios if present
	for scenarioName, scenario := range smCfg.Scenarios {
		if scenario.Provider == "" {
			return fmt.Errorf("scenario %q: provider is required", scenarioName)
		}
		if scenario.ModelID == "" {
			return fmt.Errorf("scenario %q: model_id is required", scenarioName)
		}
	}

	return nil
}

// validateProviderConfig validates a single provider configuration.
func (p *FileConfigProvider) validateProviderConfig(_name string, provider ProviderFileConfig) error {
	_ = _name
	if provider.Name == "" {
		return fmt.Errorf("provider name is required")
	}

	if provider.Type == "" {
		return fmt.Errorf("provider type is required")
	}

	if provider.BaseURL == "" {
		return fmt.Errorf("base_url is required")
	}

	if provider.TimeoutMs <= 0 {
		return fmt.Errorf("timeout_ms must be positive")
	}

	return nil
}

// compileToRuntime compiles the file configuration to a RuntimeConfig.
func (p *FileConfigProvider) compileToRuntime(cfg *FileConfig) (*RuntimeConfig, error) {
	// For file-based config, we use the "default" workspace
	// or the first available workspace
	wsConfig, ok := cfg.Workspaces["default"]
	if !ok {
		// Get first workspace
		for _, ws := range cfg.Workspaces {
			wsConfig = ws
			break
		}
	}

	// Compile supermodels
	supermodels := make(map[string]Supermodel)
	for name, smCfg := range wsConfig.Supermodels {
		supermodels[name] = Supermodel{
			Name:        smCfg.Name,
			Description: smCfg.Description,
			Default:     convertModelFileConfigToModelConfig(smCfg.Default),
			Scenarios:   convertScenariosMap(smCfg.Scenarios),
		}
	}

	// Compile providers
	providers := make(map[string]ProviderConfig)
	for name, provCfg := range wsConfig.Providers {
		providers[name] = convertProviderFileConfigToProviderConfig(provCfg)
	}

	// Build capability index from supermodels
	capabilityIndex := make(map[string]ModelCapabilities)
	for _, sm := range supermodels {
		capabilityIndex[sm.Default.ModelID] = p.inferCapabilities(sm.Default)
		for _, scenario := range sm.Scenarios {
			capabilityIndex[scenario.ModelID] = p.inferCapabilitiesFromScenario(scenario)
		}
	}

	// Default routing policies - can be customized based on config
	routingPolicies := []RoutingPolicy{
		{
			Name:     "long_context",
			Priority: 100,
			Conditions: RoutingConditions{
				ContextThreshold: 81920, // 80K tokens
			},
		},
		{
			Name:     "complex",
			Priority: 90,
			Conditions: RoutingConditions{
				HasTools: boolPtr(true),
			},
		},
		{
			Name:     "think",
			Priority: 80,
			Conditions: RoutingConditions{
				Scenarios: []string{"reasoning"},
			},
		},
		{
			Name:     "background",
			Priority: 10,
			Conditions: RoutingConditions{
				Streaming: boolPtr(false),
			},
		},
		{
			Name:       "default",
			Priority:   0,
			Conditions: RoutingConditions{},
		},
	}

	return &RuntimeConfig{
		WorkspaceID:     "default",
		Version:         cfg.Version,
		Supermodels:     supermodels,
		Providers:       providers,
		CapabilityIndex: capabilityIndex,
		RoutingPolicies: routingPolicies,
		LoggingPolicy:   convertLoggingFileConfigToLoggingPolicy(wsConfig.Logging),
		Enforcement:     convertEnforcementFileConfigToEnforcementPolicy(wsConfig.Enforcement),
	}, nil
}

// convertModelFileConfigToModelConfig converts ModelFileConfig to ModelConfig.
func convertModelFileConfigToModelConfig(mfc ModelFileConfig) ModelConfig {
	return ModelConfig{
		Provider:        mfc.Provider,
		ModelID:         mfc.ModelID,
		Temperature:     mfc.Temperature,
		MaxTokens:       mfc.MaxTokens,
		MaxOutputTokens: mfc.MaxOutputTokens,
		ContextWindow:   mfc.ContextWindow,
		ReasoningEffort: mfc.ReasoningEffort,
		SupportsTools:   mfc.SupportsTools,
		WireFormat:      mfc.WireFormat,
		Vision:          mfc.Vision,
	}
}

// convertScenariosMap converts map of ScenarioFileConfig to map of ScenarioConfig.
func convertScenariosMap(sfc map[string]ScenarioFileConfig) map[string]ScenarioConfig {
	scenarios := make(map[string]ScenarioConfig)
	for name, s := range sfc {
		scenarios[name] = ScenarioConfig{
			Provider:        s.Provider,
			ModelID:         s.ModelID,
			Temperature:     s.Temperature,
			MaxTokens:       s.MaxTokens,
			MaxOutputTokens: s.MaxOutputTokens,
			ContextWindow:   s.ContextWindow,
			ReasoningEffort: s.ReasoningEffort,
		}
	}
	return scenarios
}

// inferCapabilities extracts model capabilities from ModelConfig.
func (p *FileConfigProvider) inferCapabilities(mc ModelConfig) ModelCapabilities {
	return ModelCapabilities{
		ModelID:           mc.ModelID,
		Provider:          mc.Provider,
		MaxContextWindow:  mc.ContextWindow,
		MaxOutputTokens:   mc.MaxOutputTokens,
		SupportsTools:     mc.SupportsTools == nil || *mc.SupportsTools,
		SupportsVision:    mc.Vision,
		SupportsStreaming: true, // Assume streaming is supported by default
		SupportsThinking:  mc.ReasoningEffort != "",
		WireFormats:       []string{"openai", "anthropic"},
	}
}

// inferCapabilitiesFromScenario extracts model capabilities from ScenarioConfig.
func (p *FileConfigProvider) inferCapabilitiesFromScenario(sc ScenarioConfig) ModelCapabilities {
	supportsTools := true
	return ModelCapabilities{
		ModelID:           sc.ModelID,
		Provider:          sc.Provider,
		MaxContextWindow:  sc.ContextWindow,
		MaxOutputTokens:   sc.MaxOutputTokens,
		SupportsTools:     supportsTools,
		SupportsVision:    false, // Default to false for scenarios
		SupportsStreaming: true,
		SupportsThinking:  sc.ReasoningEffort != "",
		WireFormats:       []string{"openai", "anthropic"},
	}
}

// convertProviderFileConfigToProviderConfig converts ProviderFileConfig to ProviderConfig.
func convertProviderFileConfigToProviderConfig(pfc ProviderFileConfig) ProviderConfig {
	return ProviderConfig(pfc)
}

// convertLoggingFileConfigToLoggingPolicy converts LoggingFileConfig to LoggingPolicy.
func convertLoggingFileConfigToLoggingPolicy(lfc LoggingFileConfig) LoggingPolicy {
	return LoggingPolicy(lfc)
}

// convertEnforcementFileConfigToEnforcementPolicy converts EnforcementFileConfig to EnforcementPolicy.
func convertEnforcementFileConfigToEnforcementPolicy(efc EnforcementFileConfig) EnforcementPolicy {
	return EnforcementPolicy(efc)
}

// boolPtr returns a pointer to a bool value.
func boolPtr(v bool) *bool {
	return &v
}
