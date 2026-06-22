// Package config provides configuration management for the routatic-proxy.
package config

import "encoding/json"

// RuntimeConfig is compiled for fast request handling.
// It represents the fully-resolved configuration for a specific workspace/version.
type RuntimeConfig struct {
	WorkspaceID     string                       `json:"workspace_id"`
	Version         string                       `json:"version"` // no ETag for v1
	Supermodels     map[string]Supermodel        `json:"supermodels"`
	Providers       map[string]ProviderConfig    `json:"providers"`
	RoutingPolicies []RoutingPolicy              `json:"routing_policies"`
	CapabilityIndex map[string]ModelCapabilities `json:"capability_index"`
	LoggingPolicy   LoggingPolicy                `json:"logging_policy"`
	Enforcement     EnforcementPolicy            `json:"enforcement"`
}

// Supermodel defines a high-level model abstraction that maps to one or more
// concrete model configurations based on routing scenarios.
type Supermodel struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description,omitempty"`
	Default     ModelConfig               `json:"default"`
	Scenarios   map[string]ScenarioConfig `json:"scenarios,omitempty"`
}

// ScenarioConfig defines model configuration for a specific routing scenario.
type ScenarioConfig struct {
	ModelID         string          `json:"model_id"`
	Provider        string          `json:"provider"`
	Temperature     float64         `json:"temperature,omitempty"`
	MaxTokens       int             `json:"max_tokens,omitempty"`
	MaxOutputTokens int             `json:"max_output_tokens,omitempty"`
	ContextWindow   int             `json:"context_window,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Thinking        json.RawMessage `json:"thinking,omitempty"`
}

// ProviderConfig defines an upstream LLM provider.
// This is similar to OpenCodeGoConfig/OpenCodeZenConfig but generalized.
type ProviderConfig struct {
	Name             string            `json:"name"`
	Type             string            `json:"type"` // "opencode-go", "opencode-zen", "aws-bedrock", etc.
	BaseURL          string            `json:"base_url"`
	AnthropicBaseURL string            `json:"anthropic_base_url,omitempty"`
	ResponsesBaseURL string            `json:"responses_base_url,omitempty"`
	GeminiBaseURL    string            `json:"gemini_base_url,omitempty"`
	APIKey           string            `json:"api_key,omitempty"`
	APIKeys          []string          `json:"api_keys,omitempty"`
	TimeoutMs        int               `json:"timeout_ms"`
	StreamTimeoutMs  int               `json:"stream_timeout_ms,omitempty"`
	Headers          map[string]string `json:"headers,omitempty"`
}

// RoutingPolicy defines rules for selecting models based on request characteristics.
type RoutingPolicy struct {
	Name          string            `json:"name"`
	Priority      int               `json:"priority"` // Higher = evaluated first
	Conditions    RoutingConditions `json:"conditions"`
	TargetModel   string            `json:"target_model"`
	FallbackChain []string          `json:"fallback_chain,omitempty"`
}

// RoutingConditions defines matching criteria for routing policies.
type RoutingConditions struct {
	Scenarios        []string `json:"scenarios,omitempty"` // "long_context", "complex", "think", "vision", etc.
	MinTokens        int      `json:"min_tokens,omitempty"`
	MaxTokens        int      `json:"max_tokens,omitempty"`
	HasVision        *bool    `json:"has_vision,omitempty"`
	HasTools         *bool    `json:"has_tools,omitempty"`
	Streaming        *bool    `json:"streaming,omitempty"`
	TokenThreshold   int      `json:"token_threshold,omitempty"`   // Min token count for this policy
	ContextThreshold int      `json:"context_threshold,omitempty"` // Min context size needed
}

// ModelCapabilities describes what a model can do.
type ModelCapabilities struct {
	ModelID           string   `json:"model_id"`
	Provider          string   `json:"provider"`
	MaxContextWindow  int      `json:"max_context_window"`
	MaxOutputTokens   int      `json:"max_output_tokens"`
	SupportsTools     bool     `json:"supports_tools"`
	SupportsVision    bool     `json:"supports_vision"`
	SupportsStreaming bool     `json:"supports_streaming"`
	SupportsThinking  bool     `json:"supports_thinking"`
	ReasoningEfforts  []string `json:"reasoning_efforts,omitempty"` // "low", "medium", "high"
	WireFormats       []string `json:"wire_formats,omitempty"`      // "openai", "anthropic", "responses", "gemini"
}

// LoggingPolicy controls request/response logging behavior.
type LoggingPolicy struct {
	Level            string   `json:"level"` // "debug", "info", "warn", "error"
	LogRequests      bool     `json:"log_requests"`
	LogResponses     bool     `json:"log_responses"`
	LogLatency       bool     `json:"log_latency"`
	LogRateLimits    bool     `json:"log_rate_limits"`
	PIIMasking       bool     `json:"pii_masking"`
	SensitiveHeaders []string `json:"sensitive_headers,omitempty"`
}

// EnforcementPolicy defines security and compliance enforcement rules.
type EnforcementPolicy struct {
	RequireAuth           bool `json:"require_auth"`
	EnforceModelAllowlist bool `json:"enforce_model_allowlist"`
	EnforceBudgets        bool `json:"enforce_budgets"`
	EnforceRateLimits     bool `json:"enforce_rate_limits"`
}

// EffectiveAPIKeys returns the pool of API keys for rotation.
// APIKeys takes precedence; falls back to the single APIKey field.
func (pc *ProviderConfig) EffectiveAPIKeys() []string {
	if len(pc.APIKeys) > 0 {
		return pc.APIKeys
	}
	if pc.APIKey != "" {
		return []string{pc.APIKey}
	}
	return nil
}

// copyStringSlice creates a shallow copy of a string slice.
// Returns nil if the input is nil. Returns an empty slice if the input is empty.
func copyStringSlice(s []string) []string {
	if s == nil {
		return nil
	}
	result := make([]string, len(s))
	copy(result, s)
	return result
}

// CreateBootstrapRuntimeConfig creates a RuntimeConfig from the bootstrap Config.
// This is used when no external ConfigProvider is configured (simple mode).
// The runtime config uses settings from the bootstrap config directly.
func CreateBootstrapRuntimeConfig(cfg *Config) *RuntimeConfig {
	// Build providers map from bootstrap config
	providers := make(map[string]ProviderConfig)

	// Copy API keys to avoid sharing the same slice reference between providers
	apiKeys := copyStringSlice(cfg.EffectiveAPIKeys())

	// Add OpenCode Go provider
	if cfg.OpenCodeGo.BaseURL != "" {
		providers["opencode-go"] = ProviderConfig{
			Name:             "OpenCode Go",
			Type:             "opencode-go",
			BaseURL:          cfg.OpenCodeGo.BaseURL,
			AnthropicBaseURL: cfg.OpenCodeGo.AnthropicBaseURL,
			TimeoutMs:        cfg.OpenCodeGo.TimeoutMs,
			StreamTimeoutMs:  cfg.OpenCodeGo.StreamTimeoutMs,
			APIKeys:          copyStringSlice(apiKeys),
		}
	}

	// Add OpenCode Zen provider
	if cfg.OpenCodeZen.BaseURL != "" {
		providers["opencode-zen"] = ProviderConfig{
			Name:             "OpenCode Zen",
			Type:             "opencode-zen",
			BaseURL:          cfg.OpenCodeZen.BaseURL,
			AnthropicBaseURL: cfg.OpenCodeZen.AnthropicBaseURL,
			ResponsesBaseURL: cfg.OpenCodeZen.ResponsesBaseURL,
			GeminiBaseURL:    cfg.OpenCodeZen.GeminiBaseURL,
			TimeoutMs:        cfg.OpenCodeZen.TimeoutMs,
			StreamTimeoutMs:  cfg.OpenCodeZen.StreamTimeoutMs,
			APIKeys:          copyStringSlice(apiKeys),
		}
	}

	// Add AWS Bedrock provider if configured
	if cfg.AWSBedrock.BaseURL != "" {
		providers["aws-bedrock"] = ProviderConfig{
			Name:      "AWS Bedrock",
			Type:      "aws-bedrock",
			BaseURL:   cfg.AWSBedrock.BaseURL,
			APIKey:    cfg.AWSBedrock.APIKey,
			TimeoutMs: cfg.AWSBedrock.TimeoutMs,
		}
	}

	// Convert bootstrap models to supermodels
	supermodels := make(map[string]Supermodel)
	for name, modelCfg := range cfg.Models {
		supermodels[name] = Supermodel{
			Name: name,
			Default: ModelConfig{
				Provider:        modelCfg.Provider,
				ModelID:         modelCfg.ModelID,
				Temperature:     modelCfg.Temperature,
				MaxTokens:       modelCfg.MaxTokens,
				MaxOutputTokens: modelCfg.MaxOutputTokens,
				ContextWindow:   modelCfg.ContextWindow,
				ReasoningEffort: modelCfg.ReasoningEffort,
				Thinking:        modelCfg.Thinking,
			},
		}
	}

	workspaceID := "bootstrap"
	if cfg.Mode == "managed" && cfg.Auth.Provider == "cloud" {
		// In cloud mode, use a placeholder - actual workspace comes from auth
		workspaceID = "cloud-managed"
	}

	return &RuntimeConfig{
		WorkspaceID:     workspaceID,
		Version:         "1.0",
		Supermodels:     supermodels,
		Providers:       providers,
		CapabilityIndex: make(map[string]ModelCapabilities),
		LoggingPolicy: LoggingPolicy{
			Level:       cfg.Logging.Level,
			LogRequests: cfg.Logging.Requests,
		},
		Enforcement: EnforcementPolicy{
			RequireAuth: cfg.APIKey == "" && len(cfg.APIKeys) == 0,
		},
	}
}
