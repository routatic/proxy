package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	defaultConfigPath       = "~/.config/routatic-proxy/config.json"
	legacyConfigPath        = "~/.config/oc-go-cc/config.json"
	defaultHost             = "127.0.0.1"
	defaultPort             = 3456
	defaultBaseURL          = "https://opencode.ai/zen/go/v1/chat/completions"
	defaultAnthropicBaseURL = "https://opencode.ai/zen/go/v1/messages"
	defaultTimeoutMs        = 300000
	defaultLogLevel         = "info"

	defaultZenBaseURL          = "https://opencode.ai/zen/v1/chat/completions"
	defaultZenAnthropicBaseURL = "https://opencode.ai/zen/v1/messages"
	defaultZenResponsesBaseURL = "https://opencode.ai/zen/v1/responses"
	defaultZenGeminiBaseURL    = "https://opencode.ai/zen/v1/models"

	// Default cloud endpoints for Routatic Cloud.
	// These can be overridden via ROUTATIC_CLOUD_BASE_URL environment variable.
	defaultRoutaticCloudBaseURL          = "https://api.routatic.cloud"
	defaultRoutaticAuthIntrospectionPath = "/v1/auth/introspect"
	defaultRoutaticConfigSnapshotPath    = "/v1/config/snapshot"
)

// envVarPattern matches ${ENV_VAR} placeholders in config values.
var envVarPattern = regexp.MustCompile(`\$\{([A-Za-z0-9_]+)\}`)

var legacyEnvNames = map[string]string{
	"ROUTATIC_PROXY_CONFIG":           "OC_GO_CC_CONFIG",
	"ROUTATIC_PROXY_API_KEY":          "OC_GO_CC_API_KEY",
	"ROUTATIC_PROXY_HOST":             "OC_GO_CC_HOST",
	"ROUTATIC_PROXY_PORT":             "OC_GO_CC_PORT",
	"ROUTATIC_PROXY_OPENCODE_URL":     "OC_GO_CC_OPENCODE_URL",
	"ROUTATIC_PROXY_OPENCODE_ZEN_URL": "OC_GO_CC_OPENCODE_ZEN_URL",
	"ROUTATIC_PROXY_LOG_LEVEL":        "OC_GO_CC_LOG_LEVEL",
}

// Load reads configuration from a JSON file and applies environment variable overrides.
// Config path resolution:
//  1. ROUTATIC_PROXY_CONFIG env var (explicit override)
//  2. OC_GO_CC_CONFIG env var (legacy explicit override)
//  3. ~/.config/routatic-proxy/config.json (default)
//  4. ~/.config/oc-go-cc/config.json (legacy fallback when the new path is absent)
func Load() (*Config, error) {
	return LoadFromPath(ResolveConfigPath())
}

// LoadFromPath reads configuration from the given JSON file path.
func LoadFromPath(path string) (*Config, error) {
	cfg, err := loadJSON(path)
	if err != nil {
		return nil, fmt.Errorf("loading config from %s: %w", path, err)
	}

	applyEnvOverrides(cfg)
	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// LoadFromEnv creates a configuration entirely from environment variables.
// This is used for serverless deployments where no config file exists.
// All settings are sourced from env vars with sensible defaults.
func LoadFromEnv() (*Config, error) {
	cfg := &Config{}
	applyEnvOverrides(cfg)
	applyDefaults(cfg)

	if err := validate(cfg); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}

	return cfg, nil
}

// ResolveConfigPath determines which config file to load.
func ResolveConfigPath() string {
	if path := envValue("ROUTATIC_PROXY_CONFIG"); path != "" {
		return path
	}
	path := expandHome(defaultConfigPath)
	if _, err := os.Stat(path); err == nil {
		return path
	}
	legacyPath := expandHome(legacyConfigPath)
	if _, err := os.Stat(legacyPath); err == nil {
		return legacyPath
	}
	return path
}

// expandHome replaces a leading ~ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// loadJSON reads and parses the configuration file.
func loadJSON(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Interpolate environment variables before parsing.
	data = []byte(interpolateEnvVars(string(data)))

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing JSON: %w", err)
	}

	return &cfg, nil
}

// interpolateEnvVars replaces ${ENV_VAR} patterns with their actual values.
func interpolateEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR}
		varName := match[2 : len(match)-1]
		if val := envValue(varName); val != "" {
			return val
		}
		// Leave unchanged if env var is not set
		return match
	})
}

// applyEnvOverrides applies environment variable overrides to the config.
func applyEnvOverrides(cfg *Config) {
	if v := envValue("ROUTATIC_PROXY_API_KEY"); v != "" {
		cfg.APIKey = v
		cfg.APIKeys = nil // env var overrides both api_key and api_keys
	}
	if v := envValue("ROUTATIC_PROXY_HOST"); v != "" {
		cfg.Host = v
	}
	if v := envValue("ROUTATIC_PROXY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Port = port
		}
	}
	if v := envValue("ROUTATIC_PROXY_OPENCODE_URL"); v != "" {
		cfg.OpenCodeGo.BaseURL = v
	}
	if v := envValue("ROUTATIC_PROXY_OPENCODE_ZEN_URL"); v != "" {
		cfg.OpenCodeZen.BaseURL = v
	}
	if v := envValue("ROUTATIC_PROXY_LOG_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}

	// Cloud provider settings for serverless deployments
	if v := envValue("ROUTATIC_PROXY_MODE"); v != "" {
		cfg.Mode = v
	}
	if v := envValue("ROUTATIC_PROXY_AUTH_PROVIDER"); v != "" {
		cfg.Auth.Provider = v
	}

	// Support ROUTATIC_CLOUD_BASE_URL for setting both introspection and snapshot URLs at once
	cloudBaseURL := envValue("ROUTATIC_CLOUD_BASE_URL")
	if cloudBaseURL == "" {
		cloudBaseURL = defaultRoutaticCloudBaseURL
	}

	if v := envValue("ROUTATIC_PROXY_AUTH_INTROSPECTION_URL"); v != "" {
		cfg.Auth.IntrospectionURL = v
	}
	if cfg.Auth.IntrospectionURL == "" && cfg.Mode == "managed" {
		cfg.Auth.IntrospectionURL = cloudBaseURL + defaultRoutaticAuthIntrospectionPath
	}
	if v := envValue("ROUTATIC_PROXY_AUTH_CACHE_TTL"); v != "" {
		if ttl, err := time.ParseDuration(v); err == nil {
			cfg.Auth.CacheTTL = ttl
		}
	}
	if v := envValue("ROUTATIC_PROXY_CONFIG_PROVIDER"); v != "" {
		cfg.ConfigProv.Provider = v
	}
	if v := envValue("ROUTATIC_PROXY_CONFIG_SNAPSHOT_URL"); v != "" {
		cfg.ConfigProv.SnapshotURL = v
	}
	if cfg.ConfigProv.SnapshotURL == "" && cfg.Mode == "managed" {
		cfg.ConfigProv.SnapshotURL = cloudBaseURL + defaultRoutaticConfigSnapshotPath
	}
	if v := envValue("ROUTATIC_PROXY_CONFIG_CACHE_TTL"); v != "" {
		if ttl, err := time.ParseDuration(v); err == nil {
			cfg.ConfigProv.CacheTTL = ttl
		}
	}
}

func envValue(name string) string {
	if val := os.Getenv(name); val != "" {
		return val
	}
	if legacyName, ok := legacyEnvNames[name]; ok {
		return os.Getenv(legacyName)
	}
	for canonicalName, legacyName := range legacyEnvNames {
		if name == legacyName {
			return os.Getenv(canonicalName)
		}
	}
	return ""
}

// applyDefaults fills in missing configuration values with sensible defaults.
func applyDefaults(cfg *Config) {
	if cfg.Host == "" {
		cfg.Host = defaultHost
	}
	if cfg.Port == 0 {
		cfg.Port = defaultPort
	}
	if cfg.OpenCodeGo.BaseURL == "" {
		cfg.OpenCodeGo.BaseURL = defaultBaseURL
	}
	if cfg.OpenCodeGo.AnthropicBaseURL == "" {
		cfg.OpenCodeGo.AnthropicBaseURL = defaultAnthropicBaseURL
	}
	if cfg.OpenCodeGo.TimeoutMs == 0 {
		cfg.OpenCodeGo.TimeoutMs = defaultTimeoutMs
	}
	if cfg.OpenCodeGo.StreamTimeoutMs == 0 {
		if cfg.OpenCodeGo.StreamingTimeoutMs > 0 {
			cfg.OpenCodeGo.StreamTimeoutMs = cfg.OpenCodeGo.StreamingTimeoutMs
		} else {
			cfg.OpenCodeGo.StreamTimeoutMs = cfg.OpenCodeGo.TimeoutMs
		}
	}
	if cfg.OpenCodeZen.BaseURL == "" {
		cfg.OpenCodeZen.BaseURL = defaultZenBaseURL
	}
	if cfg.OpenCodeZen.AnthropicBaseURL == "" {
		cfg.OpenCodeZen.AnthropicBaseURL = defaultZenAnthropicBaseURL
	}
	if cfg.OpenCodeZen.ResponsesBaseURL == "" {
		cfg.OpenCodeZen.ResponsesBaseURL = defaultZenResponsesBaseURL
	}
	if cfg.OpenCodeZen.GeminiBaseURL == "" {
		cfg.OpenCodeZen.GeminiBaseURL = defaultZenGeminiBaseURL
	}
	if cfg.OpenCodeZen.TimeoutMs == 0 {
		cfg.OpenCodeZen.TimeoutMs = defaultTimeoutMs
	}
	if cfg.OpenCodeZen.StreamTimeoutMs == 0 {
		if cfg.OpenCodeZen.StreamingTimeoutMs > 0 {
			cfg.OpenCodeZen.StreamTimeoutMs = cfg.OpenCodeZen.StreamingTimeoutMs
		} else {
			cfg.OpenCodeZen.StreamTimeoutMs = cfg.OpenCodeZen.TimeoutMs
		}
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = defaultLogLevel
	}
	// For serverless/cloud deployments: if no mode is set but cloud provider env vars are present,
	// default to managed mode with cloud providers.
	if cfg.Mode == "" {
		if os.Getenv("ROUTATIC_PROXY_SERVICE_TOKEN") != "" {
			cfg.Mode = "managed"
			if cfg.Auth.Provider == "" {
				cfg.Auth.Provider = "cloud"
			}
			if cfg.ConfigProv.Provider == "" {
				cfg.ConfigProv.Provider = "cloud_snapshot"
			}
		}
	}
	if cfg.Fallbacks == nil {
		cfg.Fallbacks = make(map[string][]ModelConfig)
	}
	if cfg.ModelOverrides == nil {
		cfg.ModelOverrides = make(map[string]ModelConfig)
	}
}

// isCloudManagedMode returns true if the config is in managed mode with cloud config provider.
// In this mode, API keys and model configuration come from the cloud snapshot,
// so they are not required in the local bootstrap config.
// NOTE: This only checks the config provider, not the auth provider.
// The auth provider only validates tokens - API keys come from the config provider's snapshot.
func isCloudManagedMode(cfg *Config) bool {
	if cfg.Mode != "managed" {
		return false
	}
	// Check if using cloud config provider.
	// API keys and model configuration come from the config provider's cloud snapshot.
	if cfg.ConfigProv.Provider == "cloud_snapshot" || cfg.ConfigProv.Provider == "cloud" {
		return true
	}
	return false
}

// validate checks that all required configuration fields are present.
func validate(cfg *Config) error {
	// In managed mode with cloud config provider, API keys come from the cloud snapshot
	// and are not required in the bootstrap config.
	if !isCloudManagedMode(cfg) {
		if cfg.APIKey == "" && len(cfg.APIKeys) == 0 {
			return fmt.Errorf("api_key or api_keys is required (set via config file or ROUTATIC_PROXY_API_KEY env var; OC_GO_CC_API_KEY is still supported)")
		}
	}

	if err := validateAPIKeys(cfg.APIKeys); err != nil {
		return err
	}

	if err := validateSingleAPIKey(cfg.APIKey); err != nil {
		return err
	}

	// In cloud managed mode, model configuration comes from the cloud snapshot
	// so local validation can be skipped.
	if !isCloudManagedMode(cfg) {
		if err := validateModelOverrides(cfg.ModelOverrides); err != nil {
			return err
		}

		if err := validateAnthropicToolsDisabled(cfg); err != nil {
			return err
		}

		if err := validateVisionModels(cfg); err != nil {
			return err
		}
	}

	return nil
}

// validateVisionModels checks that when a vision scenario is configured,
// the primary model supports vision. Vision scenarios are optional —
// only validate them when they appear in the models map.
func validateVisionModels(cfg *Config) error {
	for _, scenario := range []string{"vision", "vision_complex", "vision_long_context"} {
		if model, ok := cfg.Models[scenario]; ok && !model.Vision {
			resolved := ResolveModelConfig(model)
			if !resolved.Vision {
				return fmt.Errorf("models[%q] does not support vision but is configured for vision scenario", scenario)
			}
		}
	}
	return nil
}

// validateAnthropicToolsDisabled checks that models with anthropic_tools_disabled
// set are configured correctly. This field only applies to models that route to
// the Anthropic endpoint; enabling it on an OpenAI Chat Completions model has no
// effect and likely indicates a misconfiguration.
func validateAnthropicToolsDisabled(cfg *Config) error {
	for key, mc := range cfg.Models {
		if mc.AnthropicToolsDisabled {
			// Models in cfg.Models are selectable by scenario routing. The flag
			// is only meaningful on models that go through the Anthropic endpoint.
			// Log a warning since the config system can't resolve the endpoint
			// without the client package.
			fmt.Fprintf(os.Stderr, "WARNING: config: models[%q] has anthropic_tools_disabled=true — this is only effective on models routing to the Anthropic endpoint\n", key)
		}
	}
	for key, mc := range cfg.ModelOverrides {
		if mc.AnthropicToolsDisabled {
			fmt.Fprintf(os.Stderr, "WARNING: config: model_overrides[%q] has anthropic_tools_disabled=true — this is only effective on models routing to the Anthropic endpoint\n", key)
		}
	}
	return nil
}

// validateAPIKeys ensures no api_keys entries contain unresolved ${VAR} placeholders.
// Unresolved placeholders indicate the user did not set the corresponding env vars,
// and the literal placeholder string would be sent as a bearer token.
func validateSingleAPIKey(key string) error {
	if key == "" {
		return nil
	}
	if envVarPattern.MatchString(key) {
		return fmt.Errorf("api_key contains unresolved env var %q — set the corresponding environment variable or use api_keys", key)
	}
	return nil
}

func validateAPIKeys(keys []string) error {
	for i, key := range keys {
		if key == "" {
			return fmt.Errorf("api_keys[%d] is empty — each key must be a non-empty string", i)
		}
		if envVarPattern.MatchString(key) {
			return fmt.Errorf("api_keys[%d] contains unresolved env var %q — set the corresponding environment variable or remove this entry", i, key)
		}
	}
	return nil
}

// validateModelOverrides ensures each override entry has a non-empty model_id
// and a recognized provider. Empty model_id would produce broken upstream URLs
// (surfacing far from the config error); an unknown provider would silently
// fall through to defaults at request time.
func validateModelOverrides(overrides map[string]ModelConfig) error {
	for key, mc := range overrides {
		if mc.ModelID == "" {
			return fmt.Errorf("model_overrides[%q] is missing required field model_id", key)
		}
		if mc.Provider != "" && mc.Provider != "opencode-go" && mc.Provider != "opencode-zen" {
			return fmt.Errorf("model_overrides[%q] has invalid provider %q (must be \"opencode-go\" or \"opencode-zen\")", key, mc.Provider)
		}
	}
	return nil
}
