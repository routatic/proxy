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
			var cfg map[string]interface{}
			if err := json.Unmarshal(data, &cfg); err != nil {
				t.Errorf("config is not valid JSON: %v", err)
				return
			}

			// Check that models section exists
			models, ok := cfg["models"].(map[string]interface{})
			if !ok {
				t.Error("config missing 'models' section")
				return
			}

			// Verify default model uses the correct provider
			defaultModel, ok := models["default"].(map[string]interface{})
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
	var cfg map[string]interface{}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Errorf("config is not valid JSON: %v", err)
		return
	}

	// Default config should use opencode-go
	models, ok := cfg["models"].(map[string]interface{})
	if !ok {
		t.Error("config missing 'models' section")
		return
	}

	defaultModel, ok := models["default"].(map[string]interface{})
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

	var cfg map[string]interface{}
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
			var cfg map[string]interface{}
			if err := json.Unmarshal([]byte(config), &cfg); err != nil {
				t.Errorf("config for %s is not valid JSON: %v", provider, err)
				return
			}

			// Verify models section exists
			models, ok := cfg["models"].(map[string]interface{})
			if !ok {
				t.Errorf("config for %s missing 'models' section", provider)
				return
			}

			// Verify default model uses correct provider
			defaultModel, ok := models["default"].(map[string]interface{})
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
