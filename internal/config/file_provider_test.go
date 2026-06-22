package config

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/auth"
)

func TestNewFileConfigProvider(t *testing.T) {
	tests := []struct {
		name        string
		config      string
		wantErr     bool
		errContains string
	}{
		{
			name: "valid yaml config",
			config: `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
    enforcement:
      require_auth: false
      enforce_model_allowlist: false
      enforce_budgets: false
      enforce_rate_limits: false
    logging:
      level: info
      log_requests: true
      log_responses: true
      log_latency: true
      log_rate_limits: false
      pii_masking: true
`,
			wantErr: false,
		},
		{
			name: "valid json config",
			config: `{
  "version": "1.0",
  "workspaces": {
    "default": {
      "supermodels": {
        "default": {
          "name": "default",
          "default": {
            "provider": "opencode-go",
            "model_id": "kimi-k2.6"
          }
        }
      },
      "providers": {
        "opencode-go": {
          "name": "opencode-go",
          "type": "opencode-go",
          "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
          "timeout_ms": 300000
        }
      },
      "enforcement": {
        "require_auth": false
      },
      "logging": {
        "level": "info",
        "log_requests": true
      }
    }
  }
}`,
			wantErr: false,
		},
		{
			name: "missing version",
			config: `workspaces:
  default:
    supermodels: {}
    providers: {}
`,
			wantErr:     true,
			errContains: "version is required",
		},
		{
			name: "no workspaces",
			config: `version: "1.0"
workspaces: {}
`,
			wantErr:     true,
			errContains: "at least one workspace is required",
		},
		{
			name: "missing supermodels",
			config: `version: "1.0"
workspaces:
  default:
    providers: {}
`,
			wantErr:     true,
			errContains: "at least one supermodel",
		},
		{
			name: "missing providers",
			config: `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
`,
			wantErr:     true,
			errContains: "at least one provider",
		},
		{
			name: "supermodel missing provider",
			config: `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`,
			wantErr:     true,
			errContains: "default provider is required",
		},
		{
			name: "provider missing base_url",
			config: `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        timeout_ms: 300000
`,
			wantErr:     true,
			errContains: "base_url is required",
		},
		{
			name: "provider invalid timeout",
			config: `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 0
`,
			wantErr:     true,
			errContains: "timeout_ms must be positive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temp file
			tmpDir := t.TempDir()
			configPath := filepath.Join(tmpDir, "config.yaml")
			if err := os.WriteFile(configPath, []byte(tt.config), 0644); err != nil {
				t.Fatalf("failed to write test config: %v", err)
			}

			provider, err := NewFileConfigProvider(configPath)

			if tt.wantErr {
				if err == nil {
					t.Errorf("NewFileConfigProvider() expected error but got nil")
				} else if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Errorf("NewFileConfigProvider() error = %v, want error containing %q", err, tt.errContains)
				}
				return
			}

			if err != nil {
				t.Errorf("NewFileConfigProvider() unexpected error: %v", err)
				return
			}

			if provider == nil {
				t.Error("NewFileConfigProvider() returned nil provider")
				return
			}

			// Verify config is loaded
			if provider.runtime == nil {
				t.Error("provider.runtime is nil")
			}

			// Cleanup
			_ = provider.StopWatching()
		})
	}
}

func TestFileConfigProvider_GetEffectiveConfig(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
      complex:
        name: complex
        default:
          provider: opencode-go
          model_id: glm-5.1
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
    enforcement:
      require_auth: false
    logging:
      level: info
`
	// Create temp file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	ctx := context.Background()
	authCtx := &auth.AuthContext{
		WorkspaceID: "default",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "default",
			Version:     "1.0",
		},
	}

	rtc, err := provider.GetEffectiveConfig(ctx, authCtx)
	if err != nil {
		t.Errorf("GetEffectiveConfig() error = %v", err)
		return
	}

	if rtc == nil {
		t.Error("GetEffectiveConfig() returned nil")
		return
	}

	if rtc.Version != "1.0" {
		t.Errorf("Version = %q, want %q", rtc.Version, "1.0")
	}

	if rtc.WorkspaceID != "default" {
		t.Errorf("WorkspaceID = %q, want %q", rtc.WorkspaceID, "default")
	}

	// Check supermodels
	if len(rtc.Supermodels) != 2 {
		t.Errorf("len(Supermodels) = %d, want 2", len(rtc.Supermodels))
	}

	// Check default supermodel
	if sm, ok := rtc.Supermodels["default"]; !ok {
		t.Error("Supermodels['default'] not found")
	} else {
		if sm.Default.ModelID != "kimi-k2.6" {
			t.Errorf("default supermodel ModelID = %q, want %q", sm.Default.ModelID, "kimi-k2.6")
		}
	}

	// Check providers
	if len(rtc.Providers) != 1 {
		t.Errorf("len(Providers) = %d, want 1", len(rtc.Providers))
	}

	if prov, ok := rtc.Providers["opencode-go"]; !ok {
		t.Error("Providers['opencode-go'] not found")
	} else {
		if prov.BaseURL != "https://opencode.ai/zen/go/v1/chat/completions" {
			t.Errorf("provider BaseURL = %q, want %q", prov.BaseURL, "https://opencode.ai/zen/go/v1/chat/completions")
		}
	}

	// Check enforcement
	if rtc.Enforcement.RequireAuth != false {
		t.Errorf("Enforcement.RequireAuth = %v, want false", rtc.Enforcement.RequireAuth)
	}
}

func TestFileConfigProvider_GetEffectiveConfig_NilAuthContext(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	ctx := context.Background()
	_, err = provider.GetEffectiveConfig(ctx, nil)
	if err == nil {
		t.Error("GetEffectiveConfig(nil authContext) expected error")
	} else if !strings.Contains(err.Error(), "auth context is nil") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFileConfigProvider_GetConfigByRef(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	ctx := context.Background()
	ref := auth.ConfigRef{
		WorkspaceID:  "default",
		Version:      "1.0",
		LastModified: time.Now().Unix(),
	}

	rtc, err := provider.GetConfigByRef(ctx, ref)
	if err != nil {
		t.Errorf("GetConfigByRef() error = %v", err)
		return
	}

	if rtc == nil {
		t.Error("GetConfigByRef() returned nil")
		return
	}

	if rtc.Version != "1.0" {
		t.Errorf("Version = %q, want %q", rtc.Version, "1.0")
	}
}

func TestFileConfigProvider_Invalidate(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	ctx := context.Background()

	// Get current modTime
	initialModTime := provider.modTime

	// Invalidate should trigger a reload
	err = provider.Invalidate(ctx, "default", "1.0")
	if err != nil {
		t.Errorf("Invalidate() error = %v", err)
	}

	// After reload, modTime should be updated (or at least not zero)
	if provider.modTime.IsZero() {
		t.Error("modTime is zero after invalidate/reload")
	}

	// Verify config is still available
	if provider.runtime == nil {
		t.Error("runtime is nil after invalidate")
	}

	t.Logf("Initial modTime: %v, After invalidate: %v", initialModTime, provider.modTime)
}

func TestFileConfigProvider_HealthCheck(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	ctx := context.Background()

	err = provider.HealthCheck(ctx)
	if err != nil {
		t.Errorf("HealthCheck() error = %v", err)
	}
}

func TestFileConfigProvider_HealthCheck_MissingFile(t *testing.T) {
	// Create a provider with a valid file
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	// Delete the file
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("failed to remove config file: %v", err)
	}

	ctx := context.Background()
	err = provider.HealthCheck(ctx)
	if err == nil {
		t.Error("HealthCheck() expected error when file is missing")
	}
}

func TestFileConfigProvider_Reload(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	// Get initial version
	if provider.runtime.Version != "1.0" {
		t.Fatalf("initial version = %q, want %q", provider.runtime.Version, "1.0")
	}

	// Update config
	newConfig := `version: "2.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	if err := os.WriteFile(configPath, []byte(newConfig), 0644); err != nil {
		t.Fatalf("failed to update test config: %v", err)
	}

	// Reload
	if err := provider.Reload(); err != nil {
		t.Errorf("Reload() error = %v", err)
		return
	}

	// Verify new version
	if provider.runtime.Version != "2.0" {
		t.Errorf("after reload version = %q, want %q", provider.runtime.Version, "2.0")
	}
}

func TestFileConfigProvider_Reload_InvalidConfig(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	// Save initial config
	initialConfig := *provider.runtime

	// Write invalid config
	invalidConfig := `version: "2.0"
workspaces: {}`
	if err := os.WriteFile(configPath, []byte(invalidConfig), 0644); err != nil {
		t.Fatalf("failed to write invalid config: %v", err)
	}

	// Reload should fail
	err = provider.Reload()
	if err == nil {
		t.Error("Reload() expected error for invalid config")
	}

	// Verify old config is preserved
	if provider.runtime.Version != initialConfig.Version {
		t.Error("runtime config was changed despite reload error")
	}
}

func TestFileConfigProvider_EmptyPath(t *testing.T) {
	_, err := NewFileConfigProvider("")
	if err == nil {
		t.Error("NewFileConfigProvider(\"\") expected error")
	}
	if !strings.Contains(err.Error(), "cannot be empty") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFileConfigProvider_NonExistentFile(t *testing.T) {
	_, err := NewFileConfigProvider("/nonexistent/path/config.yaml")
	if err == nil {
		t.Error("NewFileConfigProvider(nonexistent) expected error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestFileConfigProvider_StartStopWatching(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProviderWithWatch(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProviderWithWatch() failed: %v", err)
	}

	// Verify watcher is started
	if provider.watcher == nil {
		t.Error("watcher is nil after NewFileConfigProviderWithWatch")
	}

	// Stop watching
	if err := provider.StopWatching(); err != nil {
		t.Errorf("StopWatching() error = %v", err)
	}

	// Verify watcher is stopped
	if provider.watcher != nil {
		t.Error("watcher is not nil after StopWatching")
	}
}

func TestFileConfigProvider_ThreadSafety(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	ctx := context.Background()
	authCtx := &auth.AuthContext{
		WorkspaceID: "default",
		ConfigRef: auth.ConfigRef{
			WorkspaceID: "default",
			Version:     "1.0",
		},
	}

	// Run concurrent reads
	done := make(chan bool, 3)

	// Reader 1
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = provider.GetEffectiveConfig(ctx, authCtx)
		}
		done <- true
	}()

	// Reader 2
	go func() {
		for i := 0; i < 100; i++ {
			_, _ = provider.GetConfigByRef(ctx, authCtx.ConfigRef)
		}
		done <- true
	}()

	// Reader 3 - HealthCheck
	go func() {
		for i := 0; i < 100; i++ {
			_ = provider.HealthCheck(ctx)
		}
		done <- true
	}()

	// Wait for all readers
	for i := 0; i < 3; i++ {
		<-done
	}

	// If we got here without race conditions, the test passed
	t.Log("Thread safety test passed")
}

func TestFileConfigProvider_WithScenarios(t *testing.T) {
	config := `version: "1.0"
workspaces:
  default:
    supermodels:
      default:
        name: default
        default:
          provider: opencode-go
          model_id: kimi-k2.6
        scenarios:
          long_context:
            provider: opencode-zen
            model_id: minimax-k2.6
            context_window: 1000000
          complex:
            provider: opencode-go
            model_id: glm-5.1
      vision:
        name: vision
        default:
          provider: opencode-go
          model_id: kimi-k2.5
          vision: true
    providers:
      opencode-go:
        name: opencode-go
        type: opencode-go
        base_url: https://opencode.ai/zen/go/v1/chat/completions
        timeout_ms: 300000
      opencode-zen:
        name: opencode-zen
        type: opencode-zen
        base_url: https://opencode.ai/zen/v1/chat/completions
        anthropic_base_url: https://opencode.ai/zen/v1/messages
        timeout_ms: 300000
`
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(config), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	provider, err := NewFileConfigProvider(configPath)
	if err != nil {
		t.Fatalf("NewFileConfigProvider() failed: %v", err)
	}
	defer func() { _ = provider.StopWatching() }()

	// Check default supermodel has scenarios
	if sm, ok := provider.runtime.Supermodels["default"]; !ok {
		t.Error("Supermodels['default'] not found")
	} else {
		if len(sm.Scenarios) != 2 {
			t.Errorf("len(default.Scenarios) = %d, want 2", len(sm.Scenarios))
		}

		// Check long_context scenario
		if lc, ok := sm.Scenarios["long_context"]; !ok {
			t.Error("Scenarios['long_context'] not found")
		} else {
			if lc.ModelID != "minimax-k2.6" {
				t.Errorf("long_context ModelID = %q, want %q", lc.ModelID, "minimax-k2.6")
			}
			if lc.ContextWindow != 1000000 {
				t.Errorf("long_context ContextWindow = %d, want %d", lc.ContextWindow, 1000000)
			}
		}
	}

	// Check capability index
	if len(provider.runtime.CapabilityIndex) == 0 {
		t.Error("CapabilityIndex is empty")
	}

	t.Logf("CapabilityIndex: %+v", provider.runtime.CapabilityIndex)
}
