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

func TestAnthropicFirstDefaultsAndValidation(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, []byte(`{"api_key":"test-key","anthropic_first":{"enabled":true}}`), 0600); err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadFromPath(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.AnthropicFirst.Enabled || cfg.AnthropicFirst.BaseURL != "https://api.anthropic.com" {
		t.Fatalf("AnthropicFirst=%+v", cfg.AnthropicFirst)
	}

	if err := os.WriteFile(cfgPath, []byte(`{"api_key":"test-key","anthropic_first":{"enabled":true,"base_url":"not-a-url"}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFromPath(cfgPath); err == nil {
		t.Fatal("expected invalid anthropic_first.base_url to fail validation")
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

func TestEnvOverrides_RoutaticProxyTakesPrecedenceOverLegacy(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgPath, []byte(`{"api_key": "file-key"}`), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("OC_GO_CC_CONFIG", filepath.Join(dir, "legacy.json"))
	_ = os.Setenv("ROUTATIC_PROXY_API_KEY", "new-key")
	_ = os.Setenv("OC_GO_CC_API_KEY", "legacy-key")
	_ = os.Setenv("ROUTATIC_PROXY_HOST", "new-host")
	_ = os.Setenv("OC_GO_CC_HOST", "legacy-host")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("OC_GO_CC_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_API_KEY")
		_ = os.Unsetenv("OC_GO_CC_API_KEY")
		_ = os.Unsetenv("ROUTATIC_PROXY_HOST")
		_ = os.Unsetenv("OC_GO_CC_HOST")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "new-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "new-key")
	}
	if cfg.Host != "new-host" {
		t.Errorf("Host = %q, want %q", cfg.Host, "new-host")
	}
}

func TestInterpolateEnvVars_NewPlaceholderAcceptsLegacyEnv(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	if err := os.WriteFile(cfgPath, []byte(`{"api_key": "${ROUTATIC_PROXY_API_KEY}"}`), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Unsetenv("ROUTATIC_PROXY_API_KEY")
	_ = os.Setenv("OC_GO_CC_API_KEY", "legacy-key")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_API_KEY")
		_ = os.Unsetenv("OC_GO_CC_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.APIKey != "legacy-key" {
		t.Errorf("APIKey = %q, want %q", cfg.APIKey, "legacy-key")
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

func TestEnvOverrides_ProviderSpecificAPIKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "global-key",
		"opencode_go": {
			"base_url": "https://go.example.com/v1"
		},
		"opencode_zen": {
			"base_url": "https://zen.example.com/v1"
		},
		"aws_bedrock": {
			"base_url": "https://bedrock.example.com/v1"
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEY", "go-env-key")
	_ = os.Setenv("ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY", "zen-env-key")
	_ = os.Setenv("ROUTATIC_PROXY_AWS_BEDROCK_API_KEY", "bedrock-env-key")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEY")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY")
		_ = os.Unsetenv("ROUTATIC_PROXY_AWS_BEDROCK_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify provider-specific keys are set from env vars
	if cfg.OpenCodeGo.APIKey != "go-env-key" {
		t.Errorf("OpenCodeGo.APIKey = %q, want %q", cfg.OpenCodeGo.APIKey, "go-env-key")
	}
	if cfg.OpenCodeZen.APIKey != "zen-env-key" {
		t.Errorf("OpenCodeZen.APIKey = %q, want %q", cfg.OpenCodeZen.APIKey, "zen-env-key")
	}
	if cfg.AWSBedrock.APIKey != "bedrock-env-key" {
		t.Errorf("AWSBedrock.APIKey = %q, want %q", cfg.AWSBedrock.APIKey, "bedrock-env-key")
	}

	// Verify APIKeys are nilified when env var is set
	if cfg.OpenCodeGo.APIKeys != nil {
		t.Errorf("OpenCodeGo.APIKeys = %v, want nil", cfg.OpenCodeGo.APIKeys)
	}
}

func TestEnvOverrides_ProviderSpecificKeysPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	// Config has both global and provider-specific keys
	cfgJSON := `{
		"api_key": "global-key",
		"opencode_go": {
			"api_key": "go-file-key",
			"base_url": "https://go.example.com/v1"
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	// Provider env var should override file value
	_ = os.Setenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEY", "go-env-key")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Provider env var should take precedence over file
	if cfg.OpenCodeGo.APIKey != "go-env-key" {
		t.Errorf("OpenCodeGo.APIKey = %q, want %q", cfg.OpenCodeGo.APIKey, "go-env-key")
	}
}

func TestParseCommaSeparatedKeys(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []string
	}{
		{
			name:  "single key",
			input: "key-1",
			want:  []string{"key-1"},
		},
		{
			name:  "multiple keys",
			input: "key-1,key-2,key-3",
			want:  []string{"key-1", "key-2", "key-3"},
		},
		{
			name:  "keys with spaces",
			input: "key-1 , key-2 , key-3",
			want:  []string{"key-1", "key-2", "key-3"},
		},
		{
			name:  "empty string",
			input: "",
			want:  nil,
		},
		{
			name:  "only commas",
			input: ",,,",
			want:  nil,
		},
		{
			name:  "mixed empty entries",
			input: "key-1,,key-2,",
			want:  []string{"key-1", "key-2"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCommaSeparatedKeys(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCommaSeparatedKeys(%q) = %v, want %v", tt.input, got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("parseCommaSeparatedKeys(%q)[%d] = %q, want %q", tt.input, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestEnvOverrides_CommaSeparatedProviderKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "global-key",
		"opencode_go": {
			"base_url": "https://go.example.com/v1"
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	// Set comma-separated keys
	_ = os.Setenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEYS", "go-key-1,go-key-2,go-key-3")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENCODE_GO_API_KEYS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify APIKeys array is set from env var
	want := []string{"go-key-1", "go-key-2", "go-key-3"}
	if len(cfg.OpenCodeGo.APIKeys) != len(want) {
		t.Errorf("OpenCodeGo.APIKeys = %v, want %v", cfg.OpenCodeGo.APIKeys, want)
		return
	}
	for i := range cfg.OpenCodeGo.APIKeys {
		if cfg.OpenCodeGo.APIKeys[i] != want[i] {
			t.Errorf("OpenCodeGo.APIKeys[%d] = %q, want %q", i, cfg.OpenCodeGo.APIKeys[i], want[i])
		}
	}

	// Verify APIKey is cleared when API_KEYS env var is set
	if cfg.OpenCodeGo.APIKey != "" {
		t.Errorf("OpenCodeGo.APIKey = %q, want empty string", cfg.OpenCodeGo.APIKey)
	}
}

func TestEnvOverrides_GlobalCommaSeparatedKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{"api_key": "file-key"}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("ROUTATIC_PROXY_API_KEYS", "env-key-1,env-key-2")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_API_KEYS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify global APIKeys is set from env var
	want := []string{"env-key-1", "env-key-2"}
	if len(cfg.APIKeys) != len(want) {
		t.Errorf("APIKeys = %v, want %v", cfg.APIKeys, want)
		return
	}
	for i := range cfg.APIKeys {
		if cfg.APIKeys[i] != want[i] {
			t.Errorf("APIKeys[%d] = %q, want %q", i, cfg.APIKeys[i], want[i])
		}
	}

	// Verify APIKey is cleared when API_KEYS env var is set
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty string", cfg.APIKey)
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

func TestOpenCodeGoConfig_EffectiveAPIKeys(t *testing.T) {
	tests := []struct {
		name   string
		config OpenCodeGoConfig
		want   []string
	}{
		{
			name:   "APIKeys takes precedence",
			config: OpenCodeGoConfig{APIKeys: []string{"key-1", "key-2"}, APIKey: "single-key"},
			want:   []string{"key-1", "key-2"},
		},
		{
			name:   "Falls back to APIKey",
			config: OpenCodeGoConfig{APIKey: "single-key"},
			want:   []string{"single-key"},
		},
		{
			name:   "Empty returns nil",
			config: OpenCodeGoConfig{},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.EffectiveAPIKeys()
			if len(got) != len(tt.want) {
				t.Errorf("EffectiveAPIKeys() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("EffectiveAPIKeys()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestOpenCodeZenConfig_EffectiveAPIKeys(t *testing.T) {
	tests := []struct {
		name   string
		config OpenCodeZenConfig
		want   []string
	}{
		{
			name:   "APIKeys takes precedence",
			config: OpenCodeZenConfig{APIKeys: []string{"zen-1", "zen-2"}, APIKey: "zen-single"},
			want:   []string{"zen-1", "zen-2"},
		},
		{
			name:   "Falls back to APIKey",
			config: OpenCodeZenConfig{APIKey: "zen-single"},
			want:   []string{"zen-single"},
		},
		{
			name:   "Empty returns nil",
			config: OpenCodeZenConfig{},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.EffectiveAPIKeys()
			if len(got) != len(tt.want) {
				t.Errorf("EffectiveAPIKeys() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("EffectiveAPIKeys()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestAWSBedrockConfig_EffectiveAPIKeys(t *testing.T) {
	tests := []struct {
		name   string
		config AWSBedrockConfig
		want   []string
	}{
		{
			name:   "APIKeys takes precedence",
			config: AWSBedrockConfig{APIKeys: []string{"bedrock-1", "bedrock-2"}, APIKey: "bedrock-single"},
			want:   []string{"bedrock-1", "bedrock-2"},
		},
		{
			name:   "Falls back to APIKey",
			config: AWSBedrockConfig{APIKey: "bedrock-single"},
			want:   []string{"bedrock-single"},
		},
		{
			name:   "Empty returns nil",
			config: AWSBedrockConfig{},
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.EffectiveAPIKeys()
			if len(got) != len(tt.want) {
				t.Errorf("EffectiveAPIKeys() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("EffectiveAPIKeys()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
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

func TestValidate_ProviderSpecificAPIKeys(t *testing.T) {
	tests := []struct {
		name    string
		cfgJSON string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid provider keys",
			cfgJSON: `{"api_key": "global", "opencode_go": {"api_key": "go-key"}}`,
			wantErr: false,
		},
		{
			name:    "unresolved placeholder in provider api_key",
			cfgJSON: `{"api_key": "global", "opencode_go": {"api_key": "${UNSET_VAR}"}}`,
			wantErr: true,
			errMsg:  "opencode_go.api_key",
		},
		{
			name:    "unresolved placeholder in provider api_keys",
			cfgJSON: `{"api_key": "global", "opencode_go": {"api_keys": ["key1", "${UNSET_VAR}"]}}`,
			wantErr: true,
			errMsg:  "opencode_go.api_keys",
		},
		{
			name:    "empty entry in provider api_keys",
			cfgJSON: `{"api_key": "global", "opencode_go": {"api_keys": ["key1", ""]}}`,
			wantErr: true,
			errMsg:  "opencode_go.api_keys",
		},
		{
			name:    "valid provider api_keys",
			cfgJSON: `{"api_key": "global", "opencode_go": {"api_keys": ["key1", "key2"]}}`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			cfgPath := filepath.Join(dir, "config.json")

			if err := os.WriteFile(cfgPath, []byte(tt.cfgJSON), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
			defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()

			_, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Errorf("Load() expected error containing %q, got nil", tt.errMsg)
				} else if tt.errMsg != "" && !contains(err.Error(), tt.errMsg) {
					t.Errorf("Load() error = %v, want error containing %q", err, tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("Load() unexpected error = %v", err)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEnvOverrides_OpenRouterSpecificKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "global-key",
		"openrouter": {
			"base_url": "https://openrouter.example.com/v1"
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("ROUTATIC_PROXY_OPENROUTER_API_KEY", "openrouter-env-key")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENROUTER_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenRouter.APIKey != "openrouter-env-key" {
		t.Errorf("OpenRouter.APIKey = %q, want %q", cfg.OpenRouter.APIKey, "openrouter-env-key")
	}
	if cfg.OpenRouter.APIKeys != nil {
		t.Errorf("OpenRouter.APIKeys = %v, want nil", cfg.OpenRouter.APIKeys)
	}
}

func TestEnvOverrides_OpenRouterSpecificKeysPrecedence(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "global-key",
		"openrouter": {
			"api_key": "openrouter-file-key",
			"base_url": "https://openrouter.example.com/v1"
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("ROUTATIC_PROXY_OPENROUTER_API_KEY", "openrouter-env-key")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENROUTER_API_KEY")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenRouter.APIKey != "openrouter-env-key" {
		t.Errorf("OpenRouter.APIKey = %q, want %q", cfg.OpenRouter.APIKey, "openrouter-env-key")
	}
}

func TestEnvOverrides_OpenRouterCommaSeparatedKeys(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "global-key",
		"openrouter": {
			"api_key": "openrouter-file-key",
			"base_url": "https://openrouter.example.com/v1"
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("ROUTATIC_PROXY_CONFIG", cfgPath)
	_ = os.Setenv("ROUTATIC_PROXY_OPENROUTER_API_KEYS", "openrouter-key-1,openrouter-key-2,openrouter-key-3")
	defer func() {
		_ = os.Unsetenv("ROUTATIC_PROXY_CONFIG")
		_ = os.Unsetenv("ROUTATIC_PROXY_OPENROUTER_API_KEYS")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := []string{"openrouter-key-1", "openrouter-key-2", "openrouter-key-3"}
	if len(cfg.OpenRouter.APIKeys) != len(want) {
		t.Errorf("OpenRouter.APIKeys = %v, want %v", cfg.OpenRouter.APIKeys, want)
		return
	}
	for i := range cfg.OpenRouter.APIKeys {
		if cfg.OpenRouter.APIKeys[i] != want[i] {
			t.Errorf("OpenRouter.APIKeys[%d] = %q, want %q", i, cfg.OpenRouter.APIKeys[i], want[i])
		}
	}

	if cfg.OpenRouter.APIKey != "" {
		t.Errorf("OpenRouter.APIKey = %q, want empty string", cfg.OpenRouter.APIKey)
	}
}

func TestDefaults_StreamingTimeoutFallback(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")

	cfgJSON := `{
		"api_key": "test-key",
		"opencode_go": {
			"timeout_ms": 300000,
			"streaming_timeout_ms": 600000
		},
		"opencode_zen": {
			"timeout_ms": 300000,
			"streaming_timeout_ms": 700000
		}
	}`
	if err := os.WriteFile(cfgPath, []byte(cfgJSON), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	_ = os.Setenv("OC_GO_CC_CONFIG", cfgPath)
	defer func() { _ = os.Unsetenv("OC_GO_CC_CONFIG") }()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.OpenCodeGo.StreamingTimeoutMs != 600000 {
		t.Errorf("OpenCodeGo.StreamingTimeoutMs = %d, want 600000", cfg.OpenCodeGo.StreamingTimeoutMs)
	}
	if cfg.OpenCodeGo.StreamTimeoutMs != 600000 {
		t.Errorf("OpenCodeGo.StreamTimeoutMs = %d, want 600000 (should fallback to StreamingTimeoutMs)", cfg.OpenCodeGo.StreamTimeoutMs)
	}

	if cfg.OpenCodeZen.StreamingTimeoutMs != 700000 {
		t.Errorf("OpenCodeZen.StreamingTimeoutMs = %d, want 700000", cfg.OpenCodeZen.StreamingTimeoutMs)
	}
	if cfg.OpenCodeZen.StreamTimeoutMs != 700000 {
		t.Errorf("OpenCodeZen.StreamTimeoutMs = %d, want 700000 (should fallback to StreamingTimeoutMs)", cfg.OpenCodeZen.StreamTimeoutMs)
	}
}
