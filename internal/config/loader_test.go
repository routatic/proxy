package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadJSON(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "test-key-123",
		"host": "0.0.0.0",
		"port": 8080,
		"opencode_go": {
			"base_url": "https://custom.url/v1",
			"timeout_ms": 60000
		},
		"logging": {
			"level": "debug",
			"requests": true
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()

	// Prevent env var API key from overriding test config
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "test-key-123" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "test-key-123")
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("Host = %q, want %q", cfg.Host, "0.0.0.0")
	}
	if cfg.Port != 8080 {
		t.Errorf("Port = %d, want %d", cfg.Port, 8080)
	}
	if cfg.OpenCodeGo.BaseURL != "https://custom.url/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.OpenCodeGo.BaseURL, "https://custom.url/v1")
	}
	if cfg.OpenCodeGo.TimeoutMs != 60000 {
		t.Errorf("TimeoutMs = %d, want %d", cfg.OpenCodeGo.TimeoutMs, 60000)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.Logging.Level, "debug")
	}
	if !cfg.Logging.Requests {
		t.Error("Logging.Requests = false, want true")
	}
}

func TestLoadJSON_WithModelOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "test-key",
		"model_overrides": {
			"claude-sonnet-4.5": {
				"provider": "opencode-zen",
				"model_id": "claude-sonnet-4.5",
				"temperature": 0.5,
				"max_tokens": 4096
			}
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	entry, ok := cfg.ModelOverrides["claude-sonnet-4.5"]
	if !ok {
		t.Fatal("expected model_overrides[\"claude-sonnet-4.5\"] to be present after Load()")
	}
	if entry.Provider != "opencode-zen" {
		t.Errorf("Provider = %q, want %q", entry.Provider, "opencode-zen")
	}
	if entry.ModelID != "claude-sonnet-4.5" {
		t.Errorf("ModelID = %q, want %q", entry.ModelID, "claude-sonnet-4.5")
	}
	if entry.Temperature != 0.5 {
		t.Errorf("Temperature = %f, want 0.5", entry.Temperature)
	}
	if entry.MaxTokens != 4096 {
		t.Errorf("MaxTokens = %d, want 4096", entry.MaxTokens)
	}
}

func TestLoadJSON_ModelOverrides_InvalidEntryRejected(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "test-key",
		"model_overrides": {
			"bad-entry": {
				"provider": "opencode-go"
			}
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	if _, err := Load(); err == nil {
		t.Fatal("expected Load() to fail validation for empty model_id, got nil")
	}
}

func TestLoadMissingAPIKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"host": "127.0.0.1"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()

	// Prevent env var API key from making this test pass incorrectly
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing API key, got nil")
	}
}

func TestEnvOverrides(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"api_key": "file-key"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	_ = os.Setenv("OC_GO_CC_API_KEY", "env-key")
	_ = os.Setenv("OC_GO_CC_HOST", "env-host")
	_ = os.Setenv("OC_GO_CC_PORT", "9999")
	_ = os.Setenv("OC_GO_CC_OPENCODE_URL", "https://env-url/v1")
	_ = os.Setenv("OC_GO_CC_LOG_LEVEL", "warn")
	defer func() {
		_ = os.Unsetenv("OC_GO_CC_CONFIG")
		_ = os.Unsetenv("OC_GO_CC_API_KEY")
		_ = os.Unsetenv("OC_GO_CC_HOST")
		_ = os.Unsetenv("OC_GO_CC_PORT")
		_ = os.Unsetenv("OC_GO_CC_OPENCODE_URL")
		_ = os.Unsetenv("OC_GO_CC_LOG_LEVEL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "env-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "env-key")
	}
	// Env var must be the effective key (not appended to api_keys).
	if keys := cfg.EffectiveAPIKeys(); len(keys) != 1 || keys[0] != "env-key" {
		t.Errorf("EffectiveAPIKeys() = %v, want [env-key]", keys)
	}
	if cfg.Host != "env-host" {
		t.Errorf("Host = %q, want %q", cfg.Host, "env-host")
	}
	if cfg.Port != 9999 {
		t.Errorf("Port = %d, want %d", cfg.Port, 9999)
	}
	if cfg.OpenCodeGo.BaseURL != "https://env-url/v1" {
		t.Errorf("BaseURL = %q, want %q", cfg.OpenCodeGo.BaseURL, "https://env-url/v1")
	}
	if cfg.Logging.Level != "warn" {
		t.Errorf("LogLevel = %q, want %q", cfg.Logging.Level, "warn")
	}
}

func TestEnvOverrides_OC_GO_CC_API_KEY_OverridesAPIKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfJSON := `{
		"api_keys": ["file-key-1", "file-key-2"]
	}`
	if err := os.WriteFile(cfgPath, []byte(cfJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	_ = os.Setenv("OC_GO_CC_API_KEY", "env-key")
	defer func() {
		_ = os.Unsetenv("OC_GO_CC_CONFIG")
		_ = os.Unsetenv("OC_GO_CC_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Env var must fully replace the key pool, not append to it.
	if keys := cfg.EffectiveAPIKeys(); len(keys) != 1 || keys[0] != "env-key" {
		t.Errorf("EffectiveAPIKeys() = %v, want [env-key]", keys)
	}
}

func TestDefaults(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Minimal config — only API key, everything else should default.
	cfgJSON := `{"api_key": "test-key"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Host != defaultHost {
		t.Errorf("Host = %q, want %q", cfg.Host, defaultHost)
	}
	if cfg.Port != defaultPort {
		t.Errorf("Port = %d, want %d", cfg.Port, defaultPort)
	}
	if cfg.OpenCodeGo.BaseURL != defaultBaseURL {
		t.Errorf("OpenCodeGo.BaseURL = %q, want %q", cfg.OpenCodeGo.BaseURL, defaultBaseURL)
	}
	if cfg.OpenCodeGo.AnthropicBaseURL != defaultAnthropicBaseURL {
		t.Errorf("OpenCodeGo.AnthropicBaseURL = %q, want %q", cfg.OpenCodeGo.AnthropicBaseURL, defaultAnthropicBaseURL)
	}
	if cfg.OpenCodeGo.TimeoutMs != defaultTimeoutMs {
		t.Errorf("OpenCodeGo.TimeoutMs = %d, want %d", cfg.OpenCodeGo.TimeoutMs, defaultTimeoutMs)
	}
	if cfg.OpenCodeGo.StreamTimeoutMs != defaultTimeoutMs {
		t.Errorf("OpenCodeGo.StreamTimeoutMs = %d, want %d (should default to TimeoutMs when unset)",
			cfg.OpenCodeGo.StreamTimeoutMs, defaultTimeoutMs)
	}
	if cfg.OpenCodeZen.BaseURL != defaultZenBaseURL {
		t.Errorf("OpenCodeZen.BaseURL = %q, want %q", cfg.OpenCodeZen.BaseURL, defaultZenBaseURL)
	}
	if cfg.OpenCodeZen.AnthropicBaseURL != defaultZenAnthropicBaseURL {
		t.Errorf("OpenCodeZen.AnthropicBaseURL = %q, want %q", cfg.OpenCodeZen.AnthropicBaseURL, defaultZenAnthropicBaseURL)
	}
	if cfg.OpenCodeZen.ResponsesBaseURL != defaultZenResponsesBaseURL {
		t.Errorf("OpenCodeZen.ResponsesBaseURL = %q, want %q", cfg.OpenCodeZen.ResponsesBaseURL, defaultZenResponsesBaseURL)
	}
	if cfg.OpenCodeZen.GeminiBaseURL != defaultZenGeminiBaseURL {
		t.Errorf("OpenCodeZen.GeminiBaseURL = %q, want %q", cfg.OpenCodeZen.GeminiBaseURL, defaultZenGeminiBaseURL)
	}
	if cfg.OpenCodeZen.TimeoutMs != defaultTimeoutMs {
		t.Errorf("OpenCodeZen.TimeoutMs = %d, want %d", cfg.OpenCodeZen.TimeoutMs, defaultTimeoutMs)
	}
	if cfg.OpenCodeZen.StreamTimeoutMs != defaultTimeoutMs {
		t.Errorf("OpenCodeZen.StreamTimeoutMs = %d, want %d (should default to TimeoutMs when unset)",
			cfg.OpenCodeZen.StreamTimeoutMs, defaultTimeoutMs)
	}
	if cfg.Logging.Level != defaultLogLevel {
		t.Errorf("LogLevel = %q, want %q", cfg.Logging.Level, defaultLogLevel)
	}
}

func TestZenEnvOverride(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"api_key": "test-key"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	_ = os.Setenv("OC_GO_CC_OPENCODE_ZEN_URL", "https://custom-zen.url/v1/chat/completions")
	defer func() {
		_ = os.Unsetenv("OC_GO_CC_CONFIG")
		_ = os.Unsetenv("OC_GO_CC_OPENCODE_ZEN_URL")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenCodeZen.BaseURL != "https://custom-zen.url/v1/chat/completions" {
		t.Errorf("OpenCodeZen.BaseURL = %q, want %q", cfg.OpenCodeZen.BaseURL, "https://custom-zen.url/v1/chat/completions")
	}
}

func TestInterpolateEnvVars(t *testing.T) {
	_ = os.Setenv("TEST_SECRET", "my-secret-value")
	defer func() { _ = os.Unsetenv("TEST_SECRET") }()

	input := `{"api_key": "${TEST_SECRET}", "host": "${UNSET_VAR:-fallback}"}`
	result := interpolateEnvVars(input)

	want := `{"api_key": "my-secret-value", "host": "${UNSET_VAR:-fallback}"}`
	if result != want {
		t.Errorf("interpolateEnvVars() = %q, want %q", result, want)
	}
}

func TestApplyDefaults_InitializesNilMaps(t *testing.T) {
	cfg := &Config{APIKey: "test"}
	applyDefaults(cfg)
	if cfg.Fallbacks == nil {
		t.Error("applyDefaults should initialize Fallbacks to non-nil map")
	}
	if cfg.ModelOverrides == nil {
		t.Error("applyDefaults should initialize ModelOverrides to non-nil map")
	}
	// Both maps should be writable (read-then-write) without panicking.
	cfg.Fallbacks["default"] = nil
	cfg.ModelOverrides["kimi-k2.6"] = ModelConfig{}
}

func TestValidateModelOverrides_EmptyModelID(t *testing.T) {
	cfg := &Config{
		APIKey: "test",
		ModelOverrides: map[string]ModelConfig{
			"bad-entry": {Provider: "opencode-go", ModelID: ""},
		},
	}
	if err := validate(cfg); err == nil {
		t.Fatal("expected validation error for empty model_id, got nil")
	}
}

func TestValidateModelOverrides_InvalidProvider(t *testing.T) {
	cfg := &Config{
		APIKey: "test",
		ModelOverrides: map[string]ModelConfig{
			"bad-provider": {Provider: "unknown-provider", ModelID: "some-model"},
		},
	}
	if err := validate(cfg); err == nil {
		t.Fatal("expected validation error for unknown provider, got nil")
	}
}

func TestValidateModelOverrides_EmptyProviderOK(t *testing.T) {
	// Empty provider should be allowed (defaults to opencode-go at request time).
	cfg := &Config{
		APIKey: "test",
		ModelOverrides: map[string]ModelConfig{
			"good-entry": {ModelID: "kimi-k2.6"},
		},
	}
	if err := validate(cfg); err != nil {
		t.Errorf("expected no validation error for empty provider, got %v", err)
	}
}

func TestValidateModelOverrides_AllValidProviders(t *testing.T) {
	cfg := &Config{
		APIKey: "test",
		ModelOverrides: map[string]ModelConfig{
			"a": {Provider: "opencode-go", ModelID: "m1"},
			"b": {Provider: "opencode-zen", ModelID: "m2"},
			"c": {ModelID: "m3"},
		},
	}
	if err := validate(cfg); err != nil {
		t.Errorf("expected no validation error, got %v", err)
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()

	tests := []struct {
		input string
		want  string
	}{
		{"~/some/path", filepath.Join(home, "some/path")},
		{"/absolute/path", "/absolute/path"},
		{"relative/path", "relative/path"},
	}

	for _, tt := range tests {
		got := expandHome(tt.input)
		if got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEffectiveAPIKeys_APICKeysTakesPrecedence(t *testing.T) {
	cfg := &Config{
		APIKey:  "single-key",
		APIKeys: []string{"key-a", "key-b"},
	}
	keys := cfg.EffectiveAPIKeys()
	if len(keys) != 2 {
		t.Fatalf("len(keys) = %d, want 2", len(keys))
	}
	if keys[0] != "key-a" || keys[1] != "key-b" {
		t.Errorf("keys = %v, want [key-a key-b]", keys)
	}
}

func TestEffectiveAPIKeys_FallsBackToAPIKey(t *testing.T) {
	cfg := &Config{APIKey: "single-key"}
	keys := cfg.EffectiveAPIKeys()
	if len(keys) != 1 {
		t.Fatalf("len(keys) = %d, want 1", len(keys))
	}
	if keys[0] != "single-key" {
		t.Errorf("keys[0] = %q, want %q", keys[0], "single-key")
	}
}

func TestEffectiveAPIKeys_EmptyReturnsNil(t *testing.T) {
	cfg := &Config{}
	keys := cfg.EffectiveAPIKeys()
	if keys != nil {
		t.Errorf("EffectiveAPIKeys() = %v, want nil", keys)
	}
}

func TestLoad_AcceptsAPIKeysWithoutAPIKey(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_keys": ["key-1", "key-2"],
		"host": "127.0.0.1"
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()
	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	keys := cfg.EffectiveAPIKeys()
	if len(keys) != 2 {
		t.Fatalf("len(EffectiveAPIKeys()) = %d, want 2", len(keys))
	}
}

func TestLoadMissingAPIKey_NoKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"host": "127.0.0.1"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()

	oldAPIKey := os.Getenv("OC_GO_CC_API_KEY")
	_ = os.Unsetenv("OC_GO_CC_API_KEY")
	defer func() { _ = os.Setenv("OC_GO_CC_API_KEY", oldAPIKey) }()

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for missing API key, got nil")
	}
}

func TestValidateAPIKeys_RejectsUnresolvedPlaceholder(t *testing.T) {
	cfg := &Config{
		APIKeys: []string{"valid-key", "${UNRESOLVED_VAR}"},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for unresolved placeholder, got nil")
	}
}

func TestValidateAPIKeys_AcceptsResolvedKeys(t *testing.T) {
	cfg := &Config{
		APIKeys: []string{"key-1", "key-2"},
	}
	if err := validate(cfg); err != nil {
		t.Errorf("expected no validation error, got %v", err)
	}
}

func TestLoad_RejectsUnresolvedAPIKeysPlaceholders(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_keys": ["real-key", "${OC_GO_CC_UNSET_TEST_PLACEHOLDER}"]
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()
	_ = os.Unsetenv("OC_GO_CC_UNSET_TEST_PLACEHOLDER")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() expected error for unresolved placeholder in api_keys, got nil")
	}
}

func TestValidateAPIKeys_RejectsEmptyEntry(t *testing.T) {
	cfg := &Config{
		APIKeys: []string{"valid-key", ""},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty api_keys entry, got nil")
	}
}

func TestValidateAPIKeys_RejectsAllEmpty(t *testing.T) {
	cfg := &Config{
		APIKeys: []string{""},
	}
	err := validate(cfg)
	if err == nil {
		t.Fatal("expected validation error for empty api_keys entry, got nil")
	}
}
