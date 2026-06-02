package router

import (
	"testing"

	"oc-go-cc/internal/config"
)

func newTestAtomicConfig(cfg *config.Config) *config.AtomicConfig {
	return config.NewAtomicConfig(cfg, "/tmp/test-config.json")
}

func TestRoute_RespectRequestedModel_BypassesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: true,
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
		RespectRequestedModel: false,
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
		RespectRequestedModel: true,
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
		RespectRequestedModel: true,
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
		RespectRequestedModel: true,
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

	result := router.RouteForStreaming(messages, 100, "kimi-k2.6")
	if result.Primary.ModelID != "kimi-k2.6" {
		t.Errorf("expected model kimi-k2.6, got %s", result.Primary.ModelID)
	}
	if result.Scenario != ScenarioDefault {
		t.Errorf("expected ScenarioDefault, got %s", result.Scenario)
	}
}

func TestRouteForStreaming_RespectRequestedModel_False_UsesScenarioRouting(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: false,
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

	result := router.RouteForStreaming(messages, 100, "kimi-k2.6")
	// Should use streaming scenario routing, not the requested model
	if result.Primary.ModelID != "qwen3.6-plus" {
		t.Errorf("expected streaming model qwen3.6-plus, got %s", result.Primary.ModelID)
	}
}

func TestResolveRequestedModel_UsesFallbacks(t *testing.T) {
	cfg := &config.Config{
		RespectRequestedModel: true,
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

	result, ok := router.resolveRequestedModel(cfg, "kimi-k2.6")
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
				{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
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
	if result.Fallbacks[0].ModelID != "qwen3.5-plus" || result.Fallbacks[1].ModelID != "mimo-v2-pro" {
		t.Errorf("expected default fallbacks [qwen3.5-plus, mimo-v2-pro], got %+v", result.Fallbacks)
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
