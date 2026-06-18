package handlers

import (
	"encoding/json"
	"log/slog"
	"testing"

	"oc-go-cc/internal/config"
	"oc-go-cc/internal/router"
)

func boolPtr(b bool) *bool { return &b }

func TestAppendUniqueModels_DedupsByModelID(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"}, // dup of base[0]
		{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
		{Provider: "opencode-go", ModelID: "glm-5"}, // dup of base[1]
	}

	got := appendUniqueModels(base, extra)
	wantIDs := []string{"kimi-k2.6", "glm-5", "mimo-v2-pro"}

	if len(got) != len(wantIDs) {
		t.Fatalf("got %d models, want %d (got=%+v)", len(got), len(wantIDs), got)
	}
	for i, m := range got {
		if m.ModelID != wantIDs[i] {
			t.Errorf("position %d: got %s, want %s", i, m.ModelID, wantIDs[i])
		}
	}
}

func TestAppendUniqueModels_PreservesBaseOrder(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
		{Provider: "opencode-go", ModelID: "c"},
	}
	// Extra starts with a model that would have come earlier in the chain
	// (b) and adds new models. The base order must be preserved.
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "b"}, // dup
		{Provider: "opencode-go", ModelID: "d"},
		{Provider: "opencode-go", ModelID: "e"},
	}

	got := appendUniqueModels(base, extra)
	wantIDs := []string{"a", "b", "c", "d", "e"}

	if len(got) != len(wantIDs) {
		t.Fatalf("got %d models, want %d (got=%+v)", len(got), len(wantIDs), got)
	}
	for i, m := range got {
		if m.ModelID != wantIDs[i] {
			t.Errorf("position %d: got %s, want %s", i, m.ModelID, wantIDs[i])
		}
	}
}

func TestAppendUniqueModels_EmptyExtra(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
	}
	got := appendUniqueModels(base, nil)
	if len(got) != 1 || got[0].ModelID != "a" {
		t.Errorf("expected unchanged base, got %+v", got)
	}
}

func TestAppendUniqueModels_AllDuplicates(t *testing.T) {
	base := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
	}
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
	}

	got := appendUniqueModels(base, extra)
	if len(got) != 2 {
		t.Errorf("expected 2 models, got %d (got=%+v)", len(got), got)
	}
}

func TestAppendUniqueModels_EmptyBase(t *testing.T) {
	extra := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "a"},
		{Provider: "opencode-go", ModelID: "b"},
	}
	got := appendUniqueModels(nil, extra)
	if len(got) != 2 {
		t.Errorf("expected 2 models, got %d (got=%+v)", len(got), got)
	}
}

// newTestMessagesHandler returns a MessagesHandler wired with a real ModelRouter
// and a non-nil logger. Other dependencies (client, fallbackHandler, metrics)
// are nil — these tests only exercise buildModelChain, which uses modelRouter.
func newTestMessagesHandler(t *testing.T, cfg *config.Config) *MessagesHandler {
	t.Helper()
	return &MessagesHandler{
		modelRouter: router.NewModelRouter(config.NewAtomicConfig(cfg, "/tmp/test-config.json")),
		logger:      slog.Default(),
	}
}

func chainIDs(chain []config.ModelConfig) []string {
	out := make([]string, len(chain))
	for i, m := range chain {
		out[i] = m.ModelID
	}
	return out
}

func TestBuildModelChain_NoOverride_UsesScenarioRoute(t *testing.T) {
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
				{Provider: "opencode-go", ModelID: "qwen3.6-plus"},
			},
		},
	}
	h := newTestMessagesHandler(t, cfg)

	chain, result, err := h.buildModelChain("", nil, 100, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"kimi-k2.6", "mimo-v2-pro", "qwen3.6-plus"}
	if got := chainIDs(chain); !equalStrings(got, want) {
		t.Errorf("chain = %v, want %v", got, want)
	}
	if result.Scenario != router.ScenarioDefault {
		t.Errorf("scenario = %s, want %s", result.Scenario, router.ScenarioDefault)
	}
}

func TestBuildModelChain_Override_AppendsScenarioChainDeduped(t *testing.T) {
	// The override's primary overlaps with the default scenario's primary.
	// The dedup logic must drop the duplicate.
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
				{Provider: "opencode-go", ModelID: "qwen3.6-plus"},
			},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"kimi-k2.6": {
				Provider:    "opencode-zen",
				ModelID:     "kimi-k2.6",
				Temperature: 0.3,
				MaxTokens:   2048,
			},
		},
	}
	h := newTestMessagesHandler(t, cfg)

	chain, result, err := h.buildModelChain("kimi-k2.6", nil, 100, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Order: [override.primary=kimi-k2.6, scenario.primary=kimi-k2.6 (DROPPED), scenario.fallbacks...]
	// Final chain: [kimi-k2.6, mimo-v2-pro, qwen3.6-plus]
	want := []string{"kimi-k2.6", "mimo-v2-pro", "qwen3.6-plus"}
	if got := chainIDs(chain); !equalStrings(got, want) {
		t.Errorf("chain = %v, want %v (dedup must drop scenario.primary that overlaps override.primary)", got, want)
	}

	// Primary must come from the override (preserving the override's settings).
	if result.Primary.Temperature != 0.3 {
		t.Errorf("primary.Temperature = %f, want 0.3 (override settings must be preserved)", result.Primary.Temperature)
	}
	if result.Scenario != router.ScenarioOverride {
		t.Errorf("scenario = %s, want %s", result.Scenario, router.ScenarioOverride)
	}
}

func TestBuildModelChain_Override_AppendsUniqueScenarioModels(t *testing.T) {
	// Override primary does NOT overlap with the scenario chain. With default
	// fallbacks, the chain is: [override primary, default fallback, scenario
	// primary, scenario fallback(s)] with dups removed.
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
			},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4.5": {
				Provider: "opencode-zen",
				ModelID:  "claude-sonnet-4.5",
			},
		},
	}
	h := newTestMessagesHandler(t, cfg)

	chain, result, err := h.buildModelChain("claude-sonnet-4.5", nil, 100, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Chain construction:
	//   1. override primary       = claude-sonnet-4.5
	//   2. default fallbacks      = [mimo-v2-pro]            (from fallbacks["default"])
	//   3. scenario safety-net:
	//        scenario primary      = kimi-k2.6                 (new)
	//        scenario fallbacks    = [mimo-v2-pro]            (dup, dropped)
	want := []string{"claude-sonnet-4.5", "mimo-v2-pro", "kimi-k2.6"}
	if got := chainIDs(chain); !equalStrings(got, want) {
		t.Errorf("chain = %v, want %v", got, want)
	}
	if result.Scenario != router.ScenarioOverride {
		t.Errorf("scenario = %s, want %s", result.Scenario, router.ScenarioOverride)
	}
}

func TestBuildModelChain_Override_NoMatchingFallbacksKey(t *testing.T) {
	// Override has no entry in fallbacks[]. RouteWithOverride should fall back
	// to fallbacks["default"], then the scenario chain is appended as a
	// deduplicated safety net.
	cfg := &config.Config{
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {
				{Provider: "opencode-go", ModelID: "mimo-v2-pro"},
			},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4.5": {Provider: "opencode-zen", ModelID: "claude-sonnet-4.5"},
		},
	}
	h := newTestMessagesHandler(t, cfg)

	chain, _, err := h.buildModelChain("claude-sonnet-4.5", nil, 100, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Expected: [override primary, default fallback (mimo-v2-pro), scenario primary (kimi-k2.6)]
	// Note: mimo-v2-pro is in BOTH the default fallback and NOT in the scenario
	// chain here, so dedup is exercised on the override primary not overlapping
	// the scenario primary.
	want := []string{"claude-sonnet-4.5", "mimo-v2-pro", "kimi-k2.6"}
	if got := chainIDs(chain); !equalStrings(got, want) {
		t.Errorf("chain = %v, want %v (override -> default fallback -> scenario primary)", got, want)
	}
}

func TestBuildModelChain_StreamingFlag_UsesStreamingRoute(t *testing.T) {
	// With streaming + EnableStreamingScenarioRouting=false, the safety-net
	// append should use the streaming route (RouteForStreaming), not Route.
	cfg := &config.Config{
		EnableStreamingScenarioRouting: false,
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"fast":    {Provider: "opencode-go", ModelID: "qwen3.6-plus"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "mimo-v2-pro"}},
			"fast":    {{ModelID: "qwen3.5-plus"}},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-sonnet-4.5": {Provider: "opencode-zen", ModelID: "claude-sonnet-4.5"},
		},
	}
	h := newTestMessagesHandler(t, cfg)

	// Non-streaming: scenario is default
	_, resultNonStream, _ := h.buildModelChain("claude-sonnet-4.5", nil, 100, false)
	if resultNonStream.Scenario != router.ScenarioOverride {
		t.Errorf("non-streaming scenario = %s, want %s", resultNonStream.Scenario, router.ScenarioOverride)
	}

	// Streaming: override still wins, but the safety-net uses fast route.
	// Chain: [claude-sonnet-4.5 (override), mimo-v2-pro (default fallback),
	//         qwen3.6-plus (fast scenario primary), qwen3.5-plus (fast scenario fallback)]
	chain, _, _ := h.buildModelChain("claude-sonnet-4.5", nil, 100, true)
	want := []string{"claude-sonnet-4.5", "mimo-v2-pro", "qwen3.6-plus", "qwen3.5-plus"}
	if got := chainIDs(chain); !equalStrings(got, want) {
		t.Errorf("streaming chain = %v, want %v (safety-net should use RouteForStreaming)", got, want)
	}
}

func TestBuildModelChain_UnknownModel_FallsThroughToScenarioRoute(t *testing.T) {
	// Requested model has no entry in model_overrides and not in models map,
	// and respect_requested_model is false → scenario routing.
	cfg := &config.Config{
		RespectRequestedModel: boolPtr(false),
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{ModelID: "mimo-v2-pro"}},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"some-other-model": {Provider: "opencode-zen", ModelID: "some-other-model"},
		},
	}
	h := newTestMessagesHandler(t, cfg)

	chain, result, err := h.buildModelChain("completely-unknown", nil, 100, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"kimi-k2.6", "mimo-v2-pro"}
	if got := chainIDs(chain); !equalStrings(got, want) {
		t.Errorf("chain = %v, want %v", got, want)
	}
	if result.Scenario == router.ScenarioOverride {
		t.Errorf("scenario should not be override, got %s", result.Scenario)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSanitizeAnthropicBody_RemovesToolTypeField(t *testing.T) {
	rawBody := json.RawMessage(`{
		"model": "minimax-m3",
		"tools": [
			{
				"type": "custom",
				"name": "my_tool",
				"description": "A test tool",
				"input_schema": {"type": "object"}
			},
			{
				"type": "custom",
				"name": "other_tool",
				"description": "Another tool",
				"input_schema": {"type": "object"}
			}
		]
	}`)

	result := sanitizeAnthropicBody(rawBody)

	var body map[string]any
	if err := json.Unmarshal(result, &body); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	tools, ok := body["tools"].([]any)
	if !ok {
		t.Fatal("expected tools array in result")
	}

	for i, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			t.Fatalf("tool %d is not a map", i)
		}
		if _, hasType := toolMap["type"]; hasType {
			t.Errorf("tool %d still has type field after sanitization", i)
		}
		if name, ok := toolMap["name"]; !ok || name != ([]string{"my_tool", "other_tool"})[i] {
			t.Errorf("tool %d name field was corrupted", i)
		}
	}
}

func TestSanitizeAnthropicBody_NoTools(t *testing.T) {
	rawBody := json.RawMessage(`{"model": "minimax-m3", "messages": []}`)
	result := sanitizeAnthropicBody(rawBody)

	// Should return the original body unchanged
	if string(result) != string(rawBody) {
		t.Error("body without tools should be returned unchanged")
	}
}

func TestSanitizeAnthropicBody_ToolsWithoutType(t *testing.T) {
	rawBody := json.RawMessage(`{
		"tools": [
			{
				"name": "my_tool",
				"description": "No type field",
				"input_schema": {"type": "object"}
			}
		]
	}`)
	result := sanitizeAnthropicBody(rawBody)

	// Should return the original body unchanged (no type field to remove)
	if string(result) != string(rawBody) {
		t.Error("body with tools without type should be returned unchanged")
	}
}

func TestSanitizeAnthropicBody_InvalidJSON(t *testing.T) {
	rawBody := json.RawMessage(`{invalid json}`)
	result := sanitizeAnthropicBody(rawBody)

	// Should return original body unchanged on invalid JSON
	if string(result) != string(rawBody) {
		t.Error("invalid JSON should be returned unchanged")
	}
}

func TestSanitizeAnthropicBody_EmptyBody(t *testing.T) {
	rawBody := json.RawMessage(`{}`)
	result := sanitizeAnthropicBody(rawBody)

	if string(result) != string(rawBody) {
		t.Error("empty body should be returned unchanged")
	}
}

func TestSanitizeAnthropicBody_KeepsOtherFields(t *testing.T) {
	rawBody := json.RawMessage(`{
		"model": "minimax-m3",
		"system": "You are a helpful assistant",
		"messages": [{"role": "user", "content": "hello"}],
		"max_tokens": 4096,
		"tools": [
			{
				"type": "custom",
				"name": "test_tool",
				"description": "desc",
				"input_schema": {"type": "object", "properties": {}}
			}
		]
	}`)
	result := sanitizeAnthropicBody(rawBody)

	var body map[string]any
	if err := json.Unmarshal(result, &body); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	// Check that non-tool fields are preserved
	if body["model"] != "minimax-m3" {
		t.Error("model field was corrupted")
	}
	if body["system"] != "You are a helpful assistant" {
		t.Error("system field was corrupted")
	}
	if body["max_tokens"] != float64(4096) {
		t.Error("max_tokens field was corrupted")
	}
}
