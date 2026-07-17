package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestInitCmd_ProviderFlag(t *testing.T) {
	tests := []struct {
		name           string
		provider       string
		wantProvider   string
		wantErr        bool
		wantErrContain string
	}{
		{
			name:         "openrouter provider",
			provider:     "openrouter",
			wantProvider: "openrouter",
		},
		{
			name:         "aws-bedrock provider",
			provider:     "aws-bedrock",
			wantProvider: "aws-bedrock",
		},
		{
			name:         "opencode-zen provider",
			provider:     "opencode-zen",
			wantProvider: "opencode-zen",
		},
		{
			name:         "opencode-go provider",
			provider:     "opencode-go",
			wantProvider: "opencode-go",
		},
		{
			name:           "unknown provider returns error",
			provider:       "unknown-provider",
			wantErr:        true,
			wantErrContain: "unknown provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp directory
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.json")

			// Use ROUTATIC_PROXY_CONFIG to control config location
			t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

			// Create init command
			cmd := initCmd()
			cmd.SetArgs([]string{"--provider", tt.provider})

			// Run the command
			err := cmd.Execute()

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error containing %q, got nil", tt.wantErrContain)
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify config file was created
			data, err := os.ReadFile(configPath)
			if err != nil {
				t.Errorf("failed to read config file: %v", err)
				return
			}

			// Parse and verify it's valid JSON
			var cfg map[string]any
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Errorf("config is not valid JSON: %v", err)
				return
			}

			// Check that models section exists
			models, ok := cfg["models"].(map[string]any)
			if !ok {
				t.Error("config missing 'models' section")
				return
			}

			// Verify default model uses the correct provider
			defaultModel, ok := models["default"].(map[string]any)
			if !ok {
				t.Error("config missing 'models.default' section")
				return
			}

			provider, _ := defaultModel["provider"].(string)
			if provider != tt.wantProvider {
				t.Errorf("default model provider = %q, want %q", provider, tt.wantProvider)
			}
		})
	}
}

func TestInitCmd_NoProviderFlag(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Use ROUTATIC_PROXY_CONFIG to control config location
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	// Create init command without provider flag
	cmd := initCmd()

	// Run the command
	err := cmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Verify config file was created
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Errorf("failed to read config file: %v", err)
		return
	}

	// Parse and verify it's valid JSON
	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("config is not valid JSON: %v", err)
		return
	}

	// Default config should use opencode-go
	models, ok := cfg["models"].(map[string]any)
	if !ok {
		t.Error("config missing 'models' section")
		return
	}

	defaultModel, ok := models["default"].(map[string]any)
	if !ok {
		t.Error("config missing 'models.default' section")
		return
	}

	provider, _ := defaultModel["provider"].(string)
	if provider != "opencode-go" {
		t.Errorf("default model provider = %q, want 'opencode-go'", provider)
	}
}

func TestInitCmd_ConfigAlreadyExists(t *testing.T) {
	// Create temp directory with existing config
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Write existing config
	if err := os.WriteFile(configPath, []byte(`{"existing": true}`), 0600); err != nil {
		t.Fatalf("failed to write existing config: %v", err)
	}

	// Use ROUTATIC_PROXY_CONFIG to control config location
	t.Setenv("ROUTATIC_PROXY_CONFIG", configPath)

	// Create init command
	cmd := initCmd()

	// Run the command - should not error, just inform user
	err := cmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
		return
	}

	// Verify existing config was NOT overwritten
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Errorf("failed to read config file: %v", err)
		return
	}

	var cfg map[string]any
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("config is not valid JSON: %v", err)
		return
	}

	if _, ok := cfg["existing"]; !ok {
		t.Error("existing config was overwritten")
	}
}

func TestGetProviderConfig_UnknownProvider(t *testing.T) {
	_, err := getProviderConfig("invalid")
	if err == nil {
		t.Error("expected error for unknown provider")
	}
}

func TestGetProviderConfig_AllProviders(t *testing.T) {
	providers := []string{"opencode-go", "opencode-zen", "aws-bedrock", "openrouter"}

	for _, provider := range providers {
		t.Run(provider, func(t *testing.T) {
			config, err := getProviderConfig(provider)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			// Verify it's valid JSON
			var cfg map[string]any
			if err := json.Unmarshal([]byte(config), &cfg); err != nil {
				t.Errorf("config for %s is not valid JSON: %v", provider, err)
				return
			}

			// Verify models section exists
			models, ok := cfg["models"].(map[string]any)
			if !ok {
				t.Errorf("config for %s missing 'models' section", provider)
				return
			}

			// Verify default model uses correct provider
			defaultModel, ok := models["default"].(map[string]any)
			if !ok {
				t.Errorf("config for %s missing 'models.default' section", provider)
				return
			}

			actualProvider, _ := defaultModel["provider"].(string)
			if actualProvider != provider {
				t.Errorf("default model provider = %q, want %q", actualProvider, provider)
			}
		})
	}
}

// TestDefaultAndOpenCodeGoConfigsIdentical asserts the two default generators
// return byte-identical content. Both now read the single embedded
// templates/default_config.json, so this is a regression guard against anyone
// re-introducing a divergent hand-written copy.
func TestDefaultAndOpenCodeGoConfigsIdentical(t *testing.T) {
	if getDefaultConfig() != getOpenCodeGoConfig() {
		t.Error("getDefaultConfig() and getOpenCodeGoConfig() must return identical content (single source: templates/default_config.json)")
	}
}

// TestExampleConfigSharedModelsMatchDefault guards the remaining, deliberate
// duplication: configs/config.example.json is a documented superset of the
// embedded default (it adds cost_routing, aws_bedrock, extra models, etc.).
// For every model alias the two share, the definitions must be identical so
// the example never documents a stale shape (e.g. a missing vision flag).
func TestExampleConfigSharedModelsMatchDefault(t *testing.T) {
	def := mustParseConfig(t, getDefaultConfig())

	raw, err := os.ReadFile(filepath.Join("..", "..", "configs", "config.example.json"))
	if err != nil {
		t.Fatalf("read config.example.json: %v", err)
	}
	example := mustParseConfig(t, string(raw))

	defModels, _ := def["models"].(map[string]any)
	exModels, _ := example["models"].(map[string]any)

	for alias, defModel := range defModels {
		exModel, ok := exModels[alias]
		if !ok {
			t.Errorf("model %q exists in default config but is missing from config.example.json", alias)
			continue
		}
		defJSON, _ := json.Marshal(defModel)
		exJSON, _ := json.Marshal(exModel)
		if string(defJSON) != string(exJSON) {
			t.Errorf("model %q differs between default config and config.example.json:\n  default: %s\n  example: %s", alias, defJSON, exJSON)
		}
	}
}

// TestKimiK3InDefaultConfig ensures the kimi-k3 alias and its fallback chain
// are present in the embedded default (config.example.json is checked by
// TestExampleConfigSharedModelsMatchDefault).
func TestKimiK3InDefaultConfig(t *testing.T) {
	cfg := mustParseConfig(t, getDefaultConfig())
	models, _ := cfg["models"].(map[string]any)
	if _, ok := models["kimi-k3"]; !ok {
		t.Error("models.kimi-k3 missing from default config")
	}
	fallbacks, _ := cfg["fallbacks"].(map[string]any)
	if _, ok := fallbacks["kimi-k3"]; !ok {
		t.Error("fallbacks.kimi-k3 missing from default config")
	}
}

func mustParseConfig(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("invalid JSON config: %v", err)
	}
	return m
}
