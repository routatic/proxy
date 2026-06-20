// Package config handles application configuration loading and validation.
package config

import "encoding/json"

// Config holds the complete application configuration.
type Config struct {
	APIKey                         string                   `json:"api_key"`
	APIKeys                        []string                 `json:"api_keys"`
	Host                           string                   `json:"host"`
	Port                           int                      `json:"port"`
	HotReload                      bool                     `json:"hot_reload"`
	EnableStreamingScenarioRouting bool                     `json:"enable_streaming_scenario_routing"`
	RespectRequestedModel          *bool                    `json:"respect_requested_model,omitempty"`
	Models                         map[string]ModelConfig   `json:"models"`
	Fallbacks                      map[string][]ModelConfig `json:"fallbacks"`
	ModelOverrides                 map[string]ModelConfig   `json:"model_overrides"`
	OpenCodeGo                     OpenCodeGoConfig         `json:"opencode_go"`
	OpenCodeZen                    OpenCodeZenConfig        `json:"opencode_zen"`
	Logging                        LoggingConfig            `json:"logging"`
}

// ModelConfig defines routing rules for a specific model.
type ModelConfig struct {
	Provider               string          `json:"provider"`
	ModelID                string          `json:"model_id"`
	WireFormat             string          `json:"wire_format,omitempty"` // "auto" (default), "openai", "anthropic", "responses", "gemini"
	Temperature            float64         `json:"temperature"`
	MaxTokens              int             `json:"max_tokens"`
	MaxOutputTokens        int             `json:"max_output_tokens,omitempty"`
	ContextWindow          int             `json:"context_window,omitempty"`
	ContextMargin          int             `json:"context_margin,omitempty"`
	ContextThreshold       int             `json:"context_threshold"`
	SupportsTools          *bool           `json:"supports_tools,omitempty"`
	ReasoningEffort        string          `json:"reasoning_effort"`
	Thinking               json.RawMessage `json:"thinking,omitempty"`
	Vision                 bool            `json:"vision"`
	AnthropicToolsDisabled bool            `json:"anthropic_tools_disabled"`
}

// OpenCodeGoConfig holds the upstream OpenCode Go API settings.
type OpenCodeGoConfig struct {
	BaseURL            string `json:"base_url"`
	AnthropicBaseURL   string `json:"anthropic_base_url"`
	TimeoutMs          int    `json:"timeout_ms"`
	StreamTimeoutMs    int    `json:"stream_timeout_ms"`
	StreamingTimeoutMs int    `json:"streaming_timeout_ms,omitempty"`
}

// OpenCodeZenConfig holds the upstream OpenCode Zen API settings.
type OpenCodeZenConfig struct {
	BaseURL            string `json:"base_url"`
	AnthropicBaseURL   string `json:"anthropic_base_url"`
	ResponsesBaseURL   string `json:"responses_base_url"`
	GeminiBaseURL      string `json:"gemini_base_url"`
	TimeoutMs          int    `json:"timeout_ms"`
	StreamTimeoutMs    int    `json:"stream_timeout_ms"`
	StreamingTimeoutMs int    `json:"streaming_timeout_ms,omitempty"`
}

// LoggingConfig controls application logging behavior.
type LoggingConfig struct {
	Level    string `json:"level"`
	Requests bool   `json:"requests"`
}

// EffectiveAPIKeys returns the pool of API keys for rotation.
// APIKeys takes precedence; falls back to the single APIKey field.
func (c *Config) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}
