package router

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/routatic/proxy/internal/catalog"
	"github.com/routatic/proxy/internal/config"
)

func boolPtr(b bool) *bool { return &b }
func writeTestCatalog(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	data := []byte(`{
  "providers": {
    "opencode-go": {
      "name": "opencode-go",
      "base_url": "https://go.opencode.ai",
      "api_key": "",
      "enabled": true,
      "anthropic_tools_disabled": false
    },
    "openrouter": {
      "name": "openrouter",
      "base_url": "https://openrouter.ai/api/v1",
      "api_key": "",
      "enabled": true,
      "anthropic_tools_disabled": false
    }
  },
  "models": {
    "opencode-go/deepseek-v4-flash": {
      "id": "opencode-go/deepseek-v4-flash",
      "name": "DeepSeek V4 Flash",
      "limit": {"context": 1000000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text"], "output": ["text"]}
    },
    "opencode-go/kimi-k2.6": {
      "id": "opencode-go/kimi-k2.6",
      "name": "Kimi K2.6",
      "limit": {"context": 256000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text", "image"], "output": ["text"]}
    },
    "openrouter/kimi-k2.6": {
      "id": "openrouter/kimi-k2.6",
      "name": "Kimi K2.6",
      "limit": {"context": 256000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text", "image"], "output": ["text"]}
    },
    "opencode-go/glm-5": {
      "id": "opencode-go/glm-5",
      "name": "GLM 5",
      "limit": {"context": 200000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text"], "output": ["text"]}
    },
    "openrouter/glm-5": {
      "id": "openrouter/glm-5",
      "name": "GLM 5",
      "limit": {"context": 200000},
      "rates": {"input": 0.0, "output": 0.0},
      "tool_call": true,
      "modalities": {"input": ["text"], "output": ["text"]}
    }
  }
}`)

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatalf("failed to write test catalog: %v", err)
	}
	return path
}

func newTestAtomicConfig(cfg *config.Config) *config.AtomicConfig {
	return config.NewAtomicConfig(cfg, "/tmp/test-config.json")
}

func TestRoute_RespectRequestedModel_BypassesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider: "opencode-go",
				ModelID:  "kimi-k2.6",
			},
			"kimi-k2.6": {
				Provider:         "opencode-go",
				ModelID:          "kimi-k2.6",
				Temperature:      0.7,
				MaxTokens:        4096,
				ContextThreshold: 80000,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	// Complex message that would normally route to GLM-5.1
	messages := []MessageContent{
		{Role: "user", Content: "Architect a new microservice"},
	}

	result, err := router.Route(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected model kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Primary.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", result.Primary.Temperature)
	}
	if result.Primary.MaxTokens != 4096 {
		t.Errorf("expected max_tokens 4096, got %d", result.Primary.MaxTokens)
	}
	if result.Scenario != ScenarioDefault {
		t.Errorf("expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestRoute_RespectRequestedModel_False_UsesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(false),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "kimi-k2.6"},
			"complex": {ModelID: "glm-5.1"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Architect a new microservice"},
	}

	result, err := router.Route(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use scenario routing, not the requested model
	if result.Primary.ModelID != "glm-5.1" {
		t.Errorf("expected scenario-routed model glm-5.1, got %s", result.Primary.ModelID)
	}
}

func TestRoute_RespectRequestedModel_EmptyModel_FallsThrough(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.Route(messages, 100, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Empty model should fall through to scenario routing
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected default model kimi-k2.6, got %s", result.Primary.ModelID)
	}
}

func TestRoute_RespectRequestedModel_UnknownModel_UsesDefaults(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.Route(messages, 100, "some-unknown-model")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Primary.ModelID != "some-unknown-model" {
		t.Errorf("expected model some-unknown-model, got %s", result.Primary.ModelID)
	}
	// Unknown model should inherit default temperature/max_tokens
	if result.Primary.Temperature != 0.5 {
		t.Errorf("expected inherited temperature 0.5, got %f", result.Primary.Temperature)
	}
	if result.Primary.MaxTokens != 8192 {
		t.Errorf("expected inherited max_tokens 8192, got %d", result.Primary.MaxTokens)
	}
}

func TestRouteForStreaming_RespectRequestedModel_BypassesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "qwen3.6-plus"},
			"kimi-k2.6": {
				Provider: "opencode-go",
				ModelID:  "kimi-k2.6",
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.RouteForStreaming(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected model kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Scenario != ScenarioDefault {
		t.Errorf("expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestRouteForStreaming_RespectRequestedModel_False_UsesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(false),
		Models: map[string]config.ModelConfig{
			"default": {ModelID: "qwen3.6-plus"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "qwen3.5-plus"}},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	messages := []MessageContent{
		{Role: "user", Content: "Hello"},
	}

	result, err := router.RouteForStreaming(messages, 100, "kimi-k2.6")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should use streaming scenario routing, not the requested model
	if result.Primary.ModelID != "qwen3.6-plus" {
		t.Errorf("expected streaming model qwen3.6-plus, got %s", result.Primary.ModelID)
	}
}

func TestResolveRequestedModel_UsesFallbacks(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"kimi-k2.6": {ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
				{Provider: "opencode-go", ModelID: "glm-5.1"},
			},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok, err := router.resolveRequestedModel(cfg, "kimi-k2.6", false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected resolveRequestedModel to match")
	}
	if len(result.Fallbacks) != 2 {
		t.Errorf("expected 2 fallbacks, got %d", len(result.Fallbacks))
	}
	if result.Fallbacks[0].ModelID != "qwen3.5-plus" {
		t.Errorf("expected first fallback qwen3.5-plus, got %s", result.Fallbacks[0].ModelID)
	}
}

func TestRouteWithOverride_MatchesKey(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.3,
				MaxTokens:   2048,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"kimi-k2.6": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("kimi-k2.6")
	if !ok {
		t.Fatal("expected RouteWithOverride to match")
	}
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected primary kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Primary.Temperature != 0.3 {
		t.Errorf("expected temperature 0.3, got %f", result.Primary.Temperature)
	}
	if result.Scenario != ScenarioOverride {
		t.Errorf("expected ScenarioOverride, got %s", result.Scenario)
	}
	if len(result.Fallbacks) != 1 || result.Fallbacks[0].ModelID != "qwen3.5-plus" {
		t.Errorf("expected single fallback qwen3.5-plus, got %+v", result.Fallbacks)
	}
}

func TestRouteWithOverride_NoMatch(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("some-other-model")
	if ok {
		t.Errorf("expected no match, got result %+v", result)
	}
}

func TestRouteWithOverride_NilMap(t *testing.T) {
	cfg := &config.Config{} // ModelOverrides is nil

	router := NewModelRouter(newTestAtomicConfig(cfg))

	if _, ok := router.RouteWithOverride("anything"); ok {
		t.Error("expected no match for nil ModelOverrides map (must not panic)")
	}
}

func TestRouteWithOverride_MissingFallbacksKey_FallsBackToDefault(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
				{Provider: "opencode-go", ModelID: "mimo-v2.5-pro"},
			},
		},
		// No entry in Fallbacks for "kimi-k2.6" — should fall back to
		// fallbacks["default"], matching Route/RouteForStreaming behavior.
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("kimi-k2.6")
	if !ok {
		t.Fatal("expected RouteWithOverride to match")
	}
	if len(result.Fallbacks) != 2 {
		t.Fatalf("expected 2 default fallbacks, got %d: %+v", len(result.Fallbacks), result.Fallbacks)
	}
	if result.Fallbacks[0].ModelID != "qwen3.5-plus" || result.Fallbacks[1].ModelID != "mimo-v2.5-pro" {
		t.Errorf("expected default fallbacks [qwen3.5-plus, mimo-v2.5-pro], got %+v", result.Fallbacks)
	}
	chain := result.GetModelChain()
	if len(chain) != 3 {
		t.Errorf("expected 3-element chain (primary + 2 default fallbacks), got %d", len(chain))
	}
}

func TestRouteWithOverride_NoFallbacksAnywhere(t *testing.T) {
	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		// Both the override key and "default" are missing.
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))

	result, ok := router.RouteWithOverride("kimi-k2.6")
	if !ok {
		t.Fatal("expected RouteWithOverride to match")
	}
	if len(result.Fallbacks) != 0 {
		t.Errorf("expected empty fallbacks, got %+v", result.Fallbacks)
	}
	chain := result.GetModelChain()
	if len(chain) != 1 {
		t.Errorf("expected 1-element chain, got %d", len(chain))
	}
}

func TestUnknownProvider(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "qwen3.5-plus"}},
		},
	}
	atomic := newTestAtomicConfig(cfg)
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	t.Run("unknown provider in canonical reference returns ErrUnknownProvider", func(t *testing.T) {
		_, _, err := router.resolveRequestedModel(cfg, "deepseek/deepseek-v4-flash@nonexistent-provider", false)
		if err == nil {
			t.Fatal("expected error for unknown provider, got nil")
		}
		if !errors.Is(err, ErrUnknownProvider) {
			t.Fatalf("expected error to wrap ErrUnknownProvider, got %v", err)
		}
	})

	t.Run("unknown short id falls back silently to opencode-go", func(t *testing.T) {
		result, ok, err := router.resolveRequestedModel(cfg, "totally-unknown-short-id", false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatal("expected resolveRequestedModel to match")
		}
		if result.Primary.Provider != "opencode-go" {
			t.Errorf("expected provider opencode-go, got %q", result.Primary.Provider)
		}
		if result.Primary.ModelID != "totally-unknown-short-id" {
			t.Errorf("expected model_id totally-unknown-short-id, got %q", result.Primary.ModelID)
		}
	})
}

func TestResolveRequestedModel(t *testing.T) {
	catalogPath := writeTestCatalog(t)
	// Verify the fixture loads so the test failures are not misleading.
	if _, err := catalog.Load(catalogPath); err != nil {
		t.Fatalf("test catalog fixture is invalid: %v", err)
	}

	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
			"custom-model": {
				Provider:    "opencode-go",
				ModelID:     "custom-model",
				Temperature: 0.3,
				MaxTokens:   2048,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "qwen3.5-plus"}},
		},
	}
	atomic := newTestAtomicConfig(cfg)

	tests := []struct {
		name           string
		requestedModel string
		needsVision    bool
		catalogPath    string
		wantProvider   string
		wantModelID    string
		wantModelRef   string
		wantErr        bool
	}{
		{
			name:           "lab/model@provider resolves through catalog",
			requestedModel: "deepseek/deepseek-v4-flash@opencode-go",
			catalogPath:    catalogPath,
			wantProvider:   "opencode-go",
			wantModelID:    "deepseek-v4-flash",
			wantModelRef:   "deepseek/deepseek-v4-flash@opencode-go",
		},
		{
			name:           "short id resolves through catalog",
			requestedModel: "deepseek-v4-flash",
			catalogPath:    catalogPath,
			wantProvider:   "opencode-go",
			wantModelID:    "deepseek-v4-flash",
			wantModelRef:   "deepseek-v4-flash",
		},
		{
			name:           "config model takes precedence over catalog",
			requestedModel: "custom-model",
			catalogPath:    catalogPath,
			wantProvider:   "opencode-go",
			wantModelID:    "custom-model",
			wantModelRef:   "",
		},
		{
			name:           "unknown model without catalog uses legacy fallback",
			requestedModel: "some-unknown-model",
			catalogPath:    "",
			wantProvider:   "opencode-go",
			wantModelID:    "some-unknown-model",
			wantModelRef:   "",
		},
		{
			name:           "vision request for non-vision catalog model returns error",
			requestedModel: "glm-5",
			needsVision:    true,
			catalogPath:    catalogPath,
			wantErr:        true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewModelRouterWithCatalog(atomic, tt.catalogPath)
			result, ok, err := router.resolveRequestedModel(cfg, tt.requestedModel, tt.needsVision)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !ok {
				t.Fatalf("expected resolveRequestedModel to match")
			}
			if result.Primary.Provider != tt.wantProvider {
				t.Errorf("expected provider %q, got %q", tt.wantProvider, result.Primary.Provider)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Primary.ModelRef != tt.wantModelRef {
				t.Errorf("expected model_ref %q, got %q", tt.wantModelRef, result.Primary.ModelRef)
			}
		})
	}
}

func TestRoute_CanonicalAndShortRefs(t *testing.T) {
	catalogPath := writeTestCatalog(t)

	cfg := &config.Config{
		RespectRequestedModel: boolPtr(true),
		Models: map[string]config.ModelConfig{
			"default": {
				Provider:    "opencode-go",
				ModelID:     "kimi-k2.6",
				Temperature: 0.5,
				MaxTokens:   8192,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}
	atomic := newTestAtomicConfig(cfg)

	tests := []struct {
		name         string
		requested    string
		wantProvider string
		wantModelID  string
		wantModelRef string
	}{
		{
			name:         "canonical lab/model@provider",
			requested:    "deepseek/deepseek-v4-flash@opencode-go",
			wantProvider: "opencode-go",
			wantModelID:  "deepseek-v4-flash",
			wantModelRef: "deepseek/deepseek-v4-flash@opencode-go",
		},
		{
			name:         "short id resolves to first enabled provider",
			requested:    "deepseek-v4-flash",
			wantProvider: "opencode-go",
			wantModelID:  "deepseek-v4-flash",
			wantModelRef: "deepseek-v4-flash",
		},
		{
			name:         "short id with explicit provider",
			requested:    "kimi-k2.6@openrouter",
			wantProvider: "openrouter",
			wantModelID:  "kimi-k2.6",
			wantModelRef: "kimi-k2.6@openrouter",
		},
		{
			name:         "provider-qualified short id",
			requested:    "glm-5@openrouter",
			wantProvider: "openrouter",
			wantModelID:  "glm-5",
			wantModelRef: "glm-5@openrouter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewModelRouterWithCatalog(atomic, catalogPath)
			result, err := router.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, tt.requested)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Scenario != ScenarioDefault {
				t.Errorf("expected scenario %q, got %q", ScenarioDefault, result.Scenario)
			}
			if result.Primary.Provider != tt.wantProvider {
				t.Errorf("expected provider %q, got %q", tt.wantProvider, result.Primary.Provider)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Primary.ModelRef != tt.wantModelRef {
				t.Errorf("expected model_ref %q, got %q", tt.wantModelRef, result.Primary.ModelRef)
			}
		})
	}
}

func TestRoute_ModelOverridesPrecedence(t *testing.T) {
	catalogPath := writeTestCatalog(t)

	cfg := &config.Config{
		ModelOverrides: map[string]config.ModelConfig{
			"deepseek/deepseek-v4-flash@opencode-go": {
				Provider:    "opencode-zen",
				ModelID:     "claude-sonnet-4.5",
				Temperature: 0.2,
				MaxTokens:   4096,
			},
			"kimi-k2.6": {
				Provider:    "openrouter",
				ModelID:     "kimi-k2.6-or",
				Temperature: 0.1,
				MaxTokens:   1024,
			},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
		},
	}
	atomic := newTestAtomicConfig(cfg)
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	tests := []struct {
		name         string
		requested    string
		wantMatch    bool
		wantModelID  string
		wantProvider string
	}{
		{
			name:         "canonical override key matches",
			requested:    "deepseek/deepseek-v4-flash@opencode-go",
			wantMatch:    true,
			wantModelID:  "claude-sonnet-4.5",
			wantProvider: "opencode-zen",
		},
		{
			name:         "short override key matches",
			requested:    "kimi-k2.6",
			wantMatch:    true,
			wantModelID:  "kimi-k2.6-or",
			wantProvider: "openrouter",
		},
		{
			name:      "unrelated canonical ref does not match",
			requested: "kimi-k2.6@openrouter",
			wantMatch: false,
		},
		{
			name:      "unrelated short id does not match",
			requested: "glm-5",
			wantMatch: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := router.RouteWithOverride(tt.requested)
			if ok != tt.wantMatch {
				t.Fatalf("expected match=%v, got %v", tt.wantMatch, ok)
			}
			if !tt.wantMatch {
				return
			}
			if result.Scenario != ScenarioOverride {
				t.Errorf("expected scenario %q, got %q", ScenarioOverride, result.Scenario)
			}
			if result.Primary.ModelID != tt.wantModelID {
				t.Errorf("expected model_id %q, got %q", tt.wantModelID, result.Primary.ModelID)
			}
			if result.Primary.Provider != tt.wantProvider {
				t.Errorf("expected provider %q, got %q", tt.wantProvider, result.Primary.Provider)
			}
		})
	}
}

func TestCostBasedRouting_SelectsCheapest(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: true,
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
			"complex": {Provider: "opencode-go", ModelID: "legacy-complex"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "cheap-no-tools" {
		t.Errorf("default scenario: expected cheap-no-tools, got %s", result.Primary.ModelID)
	}

	complex, err := router.Route([]MessageContent{{Role: "user", Content: "Architect a new microservice"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if complex.Primary.ModelID != "large-context" {
		t.Errorf("complex scenario: expected large-context, got %s", complex.Primary.ModelID)
	}
}

func TestCostBasedRouting_DisabledUsesLegacy(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: false,
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "legacy-default"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.Route([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "legacy-default" {
		t.Errorf("expected legacy-default, got %s", result.Primary.ModelID)
	}
}

func TestCostBasedRouting_FallsBackWhenNoMatch(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: true,
		Models: map[string]config.ModelConfig{
			"background": {Provider: "opencode-go", ModelID: "legacy-background"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.Route([]MessageContent{{Role: "user", Content: "what is the time"}}, 100, "")
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}
	if result.Primary.ModelID != "legacy-background" {
		t.Errorf("expected fallback legacy-background, got %s", result.Primary.ModelID)
	}
}

func TestCostBasedRouting_RouteForStreaming(t *testing.T) {
	catalogPath := filepath.Join("testdata", "selector_catalog.json")
	cfg := &config.Config{
		APIKey:                 "global-key",
		EnableCostBasedRouting: true,
		Models: map[string]config.ModelConfig{
			"fast": {Provider: "opencode-go", ModelID: "legacy-fast"},
		},
	}
	atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	router := NewModelRouterWithCatalog(atomic, catalogPath)

	result, err := router.RouteForStreaming([]MessageContent{{Role: "user", Content: "Hello"}}, 100, "")
	if err != nil {
		t.Fatalf("RouteForStreaming failed: %v", err)
	}
	if result.Primary.ModelID != "cheap-no-tools" {
		t.Errorf("expected cheap-no-tools, got %s", result.Primary.ModelID)
	}
}

func TestRoute_LegacyConfigFixtures(t *testing.T) {
	t.Run("example config fixture", func(t *testing.T) {
		t.Setenv("ROUTATIC_PROXY_API_KEY", "test-key")

		cfgPath := "../../configs/config.example.json"
		cfg, err := config.LoadFromPath(cfgPath)
		if err != nil {
			t.Fatalf("failed to load example config: %v", err)
		}
		atomic := config.NewAtomicConfig(cfg, cfgPath)
		router := NewModelRouter(atomic)

		messages := []MessageContent{{Role: "user", Content: "Hello"}}

		result, err := router.Route(messages, 100, "")
		if err != nil {
			t.Fatalf("Route failed: %v", err)
		}
		if result.Primary.ModelID != "deepseek-v4-pro" {
			t.Errorf("expected primary deepseek-v4-pro, got %s", result.Primary.ModelID)
		}

		streamResult, err := router.RouteForStreaming(messages, 100, "")
		if err != nil {
			t.Fatalf("RouteForStreaming failed: %v", err)
		}
		if streamResult.Primary.ModelID != "deepseek-v4-flash" {
			t.Errorf("expected streaming primary deepseek-v4-flash, got %s", streamResult.Primary.ModelID)
		}
	})

	t.Run("inline legacy fixture", func(t *testing.T) {
		cfg := &config.Config{
			RespectRequestedModel: boolPtr(false),
			Models: map[string]config.ModelConfig{
				"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
				"fast":    {Provider: "opencode-go", ModelID: "qwen3.5-plus"},
			},
			Fallbacks: map[string][]config.ModelConfig{
				"default": {{Provider: "opencode-go", ModelID: "glm-5.1"}},
				"fast":    {{Provider: "opencode-go", ModelID: "deepseek-v4-flash"}},
			},
		}
		atomic := newTestAtomicConfig(cfg)
		router := NewModelRouter(atomic)

		messages := []MessageContent{{Role: "user", Content: "Hello"}}

		result, err := router.Route(messages, 100, "")
		if err != nil {
			t.Fatalf("Route failed: %v", err)
		}
		if result.Primary.ModelID != "kimi-k2.6" {
			t.Errorf("expected primary kimi-k2.6, got %s", result.Primary.ModelID)
		}

		streamResult, err := router.RouteForStreaming(messages, 100, "")
		if err != nil {
			t.Fatalf("RouteForStreaming failed: %v", err)
		}
		if streamResult.Primary.ModelID != "qwen3.5-plus" {
			t.Errorf("expected streaming primary qwen3.5-plus, got %s", streamResult.Primary.ModelID)
		}
	})
}

func TestListModels_MergesConfigAndOverrides(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default":   {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"kimi-k2.6": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-zen", ModelID: "minimax"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))
	models := router.ListModels(context.Background())

	byID := make(map[string]ModelInfo, len(models))
	for _, m := range models {
		byID[m.ID] = m
	}

	for _, want := range []string{"default", "kimi-k2.6", "claude-sonnet-4-5-20250929"} {
		if _, ok := byID[want]; !ok {
			t.Errorf("expected model %q in listing, got %+v", want, models)
		}
	}
	if got := byID["claude-sonnet-4-5-20250929"].Provider; got != "opencode-zen" {
		t.Errorf("expected override provider opencode-zen, got %q", got)
	}

	// Sorted ascending by ID.
	for i := 1; i < len(models); i++ {
		if models[i-1].ID > models[i].ID {
			t.Errorf("models not sorted: %q before %q", models[i-1].ID, models[i].ID)
		}
	}
}

func TestListModels_OverrideProviderWinsOnCollision(t *testing.T) {
	// A key present in both models and model_overrides must surface the
	// override's provider, matching routing precedence (model_overrides wins).
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4-5-20250929": {Provider: "opencode-zen", ModelID: "minimax"},
		},
	}

	router := NewModelRouter(newTestAtomicConfig(cfg))
	models := router.ListModels(context.Background())

	var got string
	for _, m := range models {
		if m.ID == "claude-sonnet-4-5-20250929" {
			got = m.Provider
		}
	}
	if got != "opencode-zen" {
		t.Errorf("expected override provider opencode-zen to win, got %q", got)
	}
}

func TestListModels_Empty(t *testing.T) {
	router := NewModelRouter(newTestAtomicConfig(&config.Config{}))
	if models := router.ListModels(context.Background()); len(models) != 0 {
		t.Errorf("expected no models, got %+v", models)
	}
}
