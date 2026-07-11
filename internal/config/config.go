// Package config handles application configuration loading and validation.
package config

import "encoding/json"

// Config is the root configuration loaded from ~/.config/routatic-proxy/config.json.
// It defines the server settings (host, port), provider connections (OpenCode Go,
// OpenCode Zen, AWS Bedrock), model definitions, and fallback chains. The config
// supports environment variable interpolation via ${VAR} syntax and hot-reloading
// when hot_reload is enabled.
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
	AWSBedrock                     AWSBedrockConfig         `json:"aws_bedrock"`
	OpenCodeGo                     OpenCodeGoConfig         `json:"opencode_go"`
	OpenCodeZen                    OpenCodeZenConfig        `json:"opencode_zen"`
	AnthropicFirst                 AnthropicFirstConfig     `json:"anthropic_first"`
	Logging                        LoggingConfig            `json:"logging"`
	Debug                          DebugConfig              `json:"debug"`
}

// AnthropicFirstConfig controls direct Anthropic passthrough with OpenCode fallback.
type AnthropicFirstConfig struct {
	Enabled bool   `json:"enabled"`
	BaseURL string `json:"base_url"`
}

// DebugConfig holds debug-related configuration.
type DebugConfig struct {
	CaptureEnabled bool   `json:"capture_enabled"`
	CaptureDir     string `json:"capture_dir"`
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

// AWSBedrockConfig holds the upstream AWS Bedrock Mantle API settings.
type AWSBedrockConfig struct {
	BaseURL            string   `json:"base_url"`
	AnthropicBaseURL   string   `json:"anthropic_base_url,omitempty"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	ProjectID          string   `json:"project_id,omitempty"`
	WireFormat         string   `json:"wire_format,omitempty"` // "openai" (default), "anthropic"
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
}

// EffectiveAPIKeys returns the pool of API keys for AWS Bedrock. The APIKeys
// array (for key rotation) takes precedence; if empty, falls back to the single
// APIKey field. This precedence order allows a single key in config with an
// override to multiple keys via environment variable (ROUTATIC_PROXY_AWS_BEDROCK_API_KEYS).
func (c *AWSBedrockConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// OpenCodeGoConfig holds the upstream OpenCode Go API settings.
type OpenCodeGoConfig struct {
	BaseURL            string   `json:"base_url"`
	AnthropicBaseURL   string   `json:"anthropic_base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
}

// EffectiveAPIKeys returns the pool of API keys for OpenCode Go. The APIKeys
// array (for key rotation) takes precedence; if empty, falls back to the single
// APIKey field. This precedence order allows a single key in config with an
// override to multiple keys via environment variable (ROUTATIC_PROXY_OPENCODE_GO_API_KEYS).
func (c *OpenCodeGoConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// OpenCodeZenConfig holds the upstream OpenCode Zen API settings.
type OpenCodeZenConfig struct {
	BaseURL            string   `json:"base_url"`
	AnthropicBaseURL   string   `json:"anthropic_base_url"`
	ResponsesBaseURL   string   `json:"responses_base_url"`
	GeminiBaseURL      string   `json:"gemini_base_url"`
	APIKey             string   `json:"api_key,omitempty"`
	APIKeys            []string `json:"api_keys,omitempty"`
	TimeoutMs          int      `json:"timeout_ms"`
	StreamTimeoutMs    int      `json:"stream_timeout_ms"`
	StreamingTimeoutMs int      `json:"streaming_timeout_ms,omitempty"`
}

// EffectiveAPIKeys returns the pool of API keys for OpenCode Zen. The APIKeys
// array (for key rotation) takes precedence; if empty, falls back to the single
// APIKey field. This precedence order allows a single key in config with an
// override to multiple keys via environment variable (ROUTATIC_PROXY_OPENCODE_ZEN_API_KEYS).
func (c *OpenCodeZenConfig) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}

// LoggingConfig controls application logging behavior.
type LoggingConfig struct {
	Level        string        `json:"level"`
	Requests     bool          `json:"requests"`
	DebugCapture *DebugCapture `json:"debug_capture,omitempty"`
}

// DebugCapture controls request/response capture for debugging.
type DebugCapture struct {
	Enabled       bool     `json:"enabled"`
	Directory     string   `json:"directory"`
	MaxFiles      int      `json:"max_files"`
	MaxFileSize   int64    `json:"max_file_size"`
	CapturePhases []string `json:"capture_phases,omitempty"`
	RedactAPIKeys bool     `json:"redact_api_keys"`
}

// EffectiveDebugCapture returns the debug capture configuration with defaults applied.
// Returns zero value DebugCapture if nil.
func (lc *LoggingConfig) EffectiveDebugCapture() DebugCapture {
	if lc.DebugCapture == nil {
		return DebugCapture{}
	}
	return *lc.DebugCapture
}

// EffectiveAPIKeys returns the global pool of API keys for rotation. The APIKeys
// array takes precedence; if empty, falls back to the single APIKey field. This
// precedence order is used by providers that lack their own key configuration,
// falling back to global keys when neither provider-specific keys nor environment
// overrides are set.
func (c *Config) EffectiveAPIKeys() []string {
	if len(c.APIKeys) > 0 {
		return c.APIKeys
	}
	if c.APIKey != "" {
		return []string{c.APIKey}
	}
	return nil
}
