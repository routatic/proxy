package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/token"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
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

	chain, result, err := h.buildModelChain("", nil, 100, false, 4096, false, false)
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

	chain, result, err := h.buildModelChain("kimi-k2.6", nil, 100, false, 4096, false, false)
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

	chain, result, err := h.buildModelChain("claude-sonnet-4.5", nil, 100, false, 4096, false, false)
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

	chain, _, err := h.buildModelChain("claude-sonnet-4.5", nil, 100, false, 4096, false, false)
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
	_, resultNonStream, _ := h.buildModelChain("claude-sonnet-4.5", nil, 100, false, 4096, false, false)
	if resultNonStream.Scenario != router.ScenarioOverride {
		t.Errorf("non-streaming scenario = %s, want %s", resultNonStream.Scenario, router.ScenarioOverride)
	}

	// Streaming: override still wins, but the safety-net uses fast route.
	// Chain: [claude-sonnet-4.5 (override), mimo-v2-pro (default fallback),
	//         qwen3.6-plus (fast scenario primary), qwen3.5-plus (fast scenario fallback)]
	chain, _, _ := h.buildModelChain("claude-sonnet-4.5", nil, 100, true, 4096, false, false)
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

	chain, result, err := h.buildModelChain("completely-unknown", nil, 100, false, 4096, false, false)
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

func TestReplaceModelInRawBody_JSONBased(t *testing.T) {
	raw := json.RawMessage(`{"model":"old-model","stream":true}`)
	res := replaceModelInRawBody(raw, "new-model")
	var m map[string]interface{}
	if err := json.Unmarshal(res, &m); err != nil {
		t.Fatal(err)
	}
	if got := m["model"]; got != "new-model" {
		t.Errorf("got %q, want new-model", got)
	}
	if got := m["stream"]; got != true {
		t.Errorf("got %v, want true", got)
	}
}

func TestReplaceModelInRawBody_HandlesWhitespace(t *testing.T) {
	raw := json.RawMessage(`{  "model"  :   "old-model"  ,   "stream": true}`)
	res := replaceModelInRawBody(raw, "new-model")
	var m map[string]interface{}
	if err := json.Unmarshal(res, &m); err != nil {
		t.Fatal(err)
	}
	if got := m["model"]; got != "new-model" {
		t.Errorf("got %q, want new-model", got)
	}
}

func TestReplaceModelInRawBody_ReturnsOriginalWhenModelMissing(t *testing.T) {
	raw := json.RawMessage(`{"stream":true}`)
	res := replaceModelInRawBody(raw, "new-model")
	if string(res) != string(raw) {
		t.Errorf("got %s, want original", string(res))
	}
}

func TestReplaceModelInRawBody_ReturnsOriginalOnInvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid json}`)
	res := replaceModelInRawBody(raw, "new-model")
	if string(res) != string(raw) {
		t.Errorf("got %s, want original", string(res))
	}
}

func TestReplaceModelInRawBody_HandlesNestedObjects(t *testing.T) {
	raw := json.RawMessage(`{"model":"old","nested":{"model":"don't touch me"}}`)
	res := replaceModelInRawBody(raw, "new")
	var m map[string]interface{}
	if err := json.Unmarshal(res, &m); err != nil {
		t.Fatal(err)
	}
	if got := m["model"]; got != "new" {
		t.Errorf("top-level model = %q, want new", got)
	}
	nested := m["nested"].(map[string]interface{})
	if got := nested["model"]; got != "don't touch me" {
		t.Errorf("nested model = %q, want 'don't touch me'", got)
	}
}

func TestHandleStreaming_GoAnthropicModel_SendsRawAnthropicBody(t *testing.T) {
	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Logf("upstream read body error: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer upstream.Close()

	handler := newStreamingTestHandler(t, upstream.URL)

	rawBody := json.RawMessage(`{
		"model": "claude-opus-4-8",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}],
		"tools": [{
			"name": "Bash",
			"description": "Run a command",
			"input_schema": {"type": "object", "properties": {"cmd": {"type": "string"}}}
		}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "minimax-m3"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{}, chain, rawBody)

	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	if got, ok := captured["model"]; !ok || got != "minimax-m3" {
		t.Fatalf("captured model = %v, want minimax-m3", got)
	}

	toolsRaw, ok := captured["tools"]
	if !ok {
		t.Fatal("captured body missing tools field")
	}
	tools, ok := toolsRaw.([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("captured body tools is empty or not an array")
	}
	tool0, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatal("tool[0] is not an object")
	}
	if _, ok := tool0["function"]; ok {
		t.Fatalf("captured tool has 'function' field (OpenAI format leak): %s", capturedBody)
	}
	if _, ok := tool0["input_schema"]; !ok {
		t.Fatalf("captured tool missing 'input_schema' (Anthropic format): %s", capturedBody)
	}
	if got, ok := tool0["name"]; !ok || got != "Bash" {
		t.Fatalf("captured tool name = %v, want Bash", got)
	}
}

func TestHandleStreaming_GoAnthropicModel_FallsThroughOnError(t *testing.T) {
	callCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {}\n\n")
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			AnthropicBaseURL: upstream.URL,
			BaseURL:          upstream.URL,
			TimeoutMs:        5000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)

	handler := &MessagesHandler{
		client:              ocClient,
		logger:              slog.Default(),
		metrics:             metrics.New(),
		streamHandler:       transformer.NewStreamHandler(),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
	}

	rawBody := json.RawMessage(`{
		"model": "claude-opus-4-8",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "minimax-m3"},
		{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{}, chain, rawBody)

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 2 {
		t.Fatalf("expected 2 upstream calls (1 fail + 1 success), got %d", finalCount)
	}
}

func newStreamingTestHandler(t *testing.T, upstreamURL string) *MessagesHandler {
	t.Helper()
	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			AnthropicBaseURL: upstreamURL,
			BaseURL:          upstreamURL,
			TimeoutMs:        5000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)

	return &MessagesHandler{
		client:              ocClient,
		logger:              slog.Default(),
		metrics:             metrics.New(),
		streamHandler:       transformer.NewStreamHandler(),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
	}
}

func TestHandleMessages_StreamingMinimaxM3_UsesAnthropicEndpoint(t *testing.T) {
	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Logf("upstream read body error: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
			"fast":    {Provider: "opencode-go", ModelID: "qwen3.6-plus"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "glm-5"}},
			"fast":    {{Provider: "opencode-go", ModelID: "qwen3.5-plus"}},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"minimax-m3": {
				Provider: "opencode-go",
				ModelID:  "minimax-m3",
			},
		},
		OpenCodeGo: config.OpenCodeGoConfig{
			AnthropicBaseURL: upstream.URL,
			BaseURL:          upstream.URL,
			TimeoutMs:        5000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")

	ocClient := client.NewOpenCodeClient(atomicCfg)
	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	handler := NewMessagesHandler(
		ocClient,
		nil, // providerRegistry
		modelRouter,
		nil, // fallbackHandler
		tokenCounter,
		metrics.New(),
	)
	handler.logger = slog.Default()

	requestBody := `{
		"model": "minimax-m3",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role": "user", "content": "Say hello"}],
		"tools": [{
			"name": "Bash",
			"description": "Run a command",
			"input_schema": {"type": "object", "properties": {"cmd": {"type": "string"}}}
		}]
	}`

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	handler.HandleMessages(recorder, req)

	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	if got, ok := captured["model"]; !ok || got != "minimax-m3" {
		t.Fatalf("captured model = %v, want minimax-m3", got)
	}

	toolsRaw, ok := captured["tools"]
	if !ok {
		t.Fatal("captured body missing tools field")
	}
	tools, ok := toolsRaw.([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("captured body tools is empty or not an array")
	}
	tool0, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatal("tool[0] is not an object")
	}
	if _, ok := tool0["function"]; ok {
		t.Fatalf("captured tool has 'function' field (OpenAI format leak): %s", capturedBody)
	}
	if _, ok := tool0["input_schema"]; !ok {
		t.Fatalf("captured tool missing 'input_schema' (Anthropic format): %s", capturedBody)
	}
}

func TestHandleNonStreaming_GoAnthropicModel_ReplacesModelInBody(t *testing.T) {
	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Logf("upstream read body error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "hello"}],
			"model": "minimax-m3",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "glm-5"}},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-haiku-4-5-20251001": {
				Provider: "opencode-go",
				ModelID:  "minimax-m3",
			},
		},
		OpenCodeGo: config.OpenCodeGoConfig{
			AnthropicBaseURL: upstream.URL,
			BaseURL:          upstream.URL,
			TimeoutMs:        5000,
		},
	}

	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)
	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	handler := NewMessagesHandler(
		ocClient,
		nil, // providerRegistry
		modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 30*time.Second),
		tokenCounter,
		metrics.New(),
	)
	handler.logger = slog.Default()

	requestBody := `{
		"model": "claude-haiku-4-5-20251001",
		"max_tokens": 256,
		"messages": [{"role": "user", "content": "Say hello"}],
		"tools": [{
			"name": "Bash",
			"description": "Run a command",
			"input_schema": {"type": "object", "properties": {"cmd": {"type": "string"}}}
		}]
	}`

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	handler.HandleMessages(recorder, req)

	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	if got, ok := captured["model"]; !ok || got != "minimax-m3" {
		t.Fatalf("captured model = %v, want minimax-m3", got)
	}

	toolsRaw, ok := captured["tools"]
	if !ok {
		t.Fatal("captured body missing tools field")
	}
	tools, ok := toolsRaw.([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("captured body tools is empty or not an array")
	}
	tool0, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatal("tool[0] is not an object")
	}
	if _, ok := tool0["function"]; ok {
		t.Fatalf("captured tool has 'function' field (OpenAI format leak): %s", capturedBody)
	}
	if _, ok := tool0["input_schema"]; !ok {
		t.Fatalf("captured tool missing 'input_schema' (Anthropic format): %s", capturedBody)
	}
}

func TestHandleNonStreaming_ZenAnthropicModel_ReplacesModelInBody(t *testing.T) {
	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Logf("upstream read body error: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "hello"}],
			"model": "claude-sonnet-4.5",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "glm-5"}},
		},
		ModelOverrides: map[string]config.ModelConfig{
			"claude-haiku-4-5-20251001": {
				Provider: "opencode-zen",
				ModelID:  "claude-sonnet-4.5",
			},
		},
		OpenCodeGo: config.OpenCodeGoConfig{
			AnthropicBaseURL: upstream.URL,
			BaseURL:          upstream.URL,
			TimeoutMs:        5000,
		},
		OpenCodeZen: config.OpenCodeZenConfig{
			AnthropicBaseURL: upstream.URL,
			TimeoutMs:        5000,
		},
	}

	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)
	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	handler := NewMessagesHandler(
		ocClient,
		nil, // providerRegistry
		modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 30*time.Second),
		tokenCounter,
		metrics.New(),
	)
	handler.logger = slog.Default()

	requestBody := `{
		"model": "claude-haiku-4-5-20251001",
		"max_tokens": 256,
		"messages": [{"role": "user", "content": "Say hello"}],
		"tools": [{
			"name": "Bash",
			"description": "Run a command",
			"input_schema": {"type": "object", "properties": {"cmd": {"type": "string"}}}
		}]
	}`

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	handler.HandleMessages(recorder, req)

	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	if got, ok := captured["model"]; !ok || got != "claude-sonnet-4.5" {
		t.Fatalf("captured model = %v, want claude-sonnet-4.5", got)
	}

	toolsRaw, ok := captured["tools"]
	if !ok {
		t.Fatal("captured body missing tools field")
	}
	tools, ok := toolsRaw.([]interface{})
	if !ok || len(tools) == 0 {
		t.Fatal("captured body tools is empty or not an array")
	}
	tool0, ok := tools[0].(map[string]interface{})
	if !ok {
		t.Fatal("tool[0] is not an object")
	}
	if _, ok := tool0["function"]; ok {
		t.Fatalf("captured tool has 'function' field (OpenAI format leak): %s", capturedBody)
	}
	if _, ok := tool0["input_schema"]; !ok {
		t.Fatalf("captured tool missing 'input_schema' (Anthropic format): %s", capturedBody)
	}
}

func TestHandleStreaming_ConfigurableTimeout(t *testing.T) {
	upstreamChan := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-upstreamChan:
		case <-time.After(5 * time.Second):
		}
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
	}))
	defer upstream.Close()
	defer close(upstreamChan)

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:            upstream.URL,
			TimeoutMs:          300000,
			StreamingTimeoutMs: 100,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)

	handler := &MessagesHandler{
		client:              ocClient,
		logger:              slog.Default(),
		metrics:             metrics.New(),
		streamHandler:       transformer.NewStreamHandler(),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
	}

	rawBody := json.RawMessage(`{
		"model": "kimi-k2.6",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{Stream: true}, chain, rawBody)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleStreaming did not return within 2s despite short streaming timeout")
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "all streaming models failed") && !strings.Contains(body, "all upstream models failed") {
		t.Errorf("unexpected output on streaming timeout: %s", body)
	}
}

func TestHandleStreaming_ClientContextCanceled_StopsFallback(t *testing.T) {
	callCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
	}))
	defer upstream.Close()

	handler := newStreamingTestHandler(t, upstream.URL)

	rawBody := json.RawMessage(`{
		"model": "kimi-k2.6",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{Stream: true}, chain, rawBody)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleStreaming did not return immediately on canceled client context")
	}

	if atomic.LoadInt32(&callCount) != 0 {
		t.Errorf("expected 0 upstream calls since client context was canceled, got %d", callCount)
	}
}

func TestHandleStreaming_ClientDisconnectsDuringStream_StopsFallback(t *testing.T) {
	blockCh := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-blockCh
	}))
	defer upstream.Close()
	defer close(blockCh)

	handler := newStreamingTestHandler(t, upstream.URL)

	rawBody := json.RawMessage(`{
		"model": "kimi-k2.6",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{Stream: true}, chain, rawBody)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handleStreaming did not return after client disconnected")
	}
}

func TestHandleStreaming_PerModelTimeoutFallback(t *testing.T) {
	callCount := int32(0)
	upstreamBlock := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			select {
			case <-upstreamBlock:
			case <-time.After(5 * time.Second):
			}
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {}\n\n")
	}))
	defer upstream.Close()
	defer close(upstreamBlock)

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:            upstream.URL,
			TimeoutMs:          300000,
			StreamingTimeoutMs: 100,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)

	handler := &MessagesHandler{
		client:              ocClient,
		logger:              slog.Default(),
		metrics:             metrics.New(),
		streamHandler:       transformer.NewStreamHandler(),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
	}

	rawBody := json.RawMessage(`{
		"model": "kimi-k2.6",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, handlerCancel := context.WithCancel(req.Context())
	defer handlerCancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{Stream: true}, chain, rawBody)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleStreaming did not complete within 5s")
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 2 {
		t.Errorf("expected 2 upstream calls (1 timeout + 1 success), got %d", finalCount)
	}
}

func TestHandleNonStreaming_ParentContextCanceled_No502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "hello"}],
			"model": "kimi-k2.6",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "glm-5"}},
		},
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:   upstream.URL,
			TimeoutMs: 5000,
		},
	}

	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)
	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	m := metrics.New()
	handler := NewMessagesHandler(
		ocClient,
		nil, // providerRegistry
		modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 30*time.Second),
		tokenCounter,
		m,
	)
	handler.logger = slog.Default()

	requestBody := `{
		"model": "claude-haiku-4-5-20251001",
		"max_tokens": 256,
		"messages": [{"role": "user", "content": "Say hello"}]
	}`

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	handler.HandleMessages(recorder, req)

	if recorder.Code == http.StatusBadGateway {
		t.Errorf("should not return 502 for canceled context, got status %d", recorder.Code)
	}

	snap := m.GetSnapshot()
	if snap.RequestsFailed > 0 {
		t.Errorf("failure count should be 0 for canceled context, got %d", snap.RequestsFailed)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "all models failed") {
		t.Errorf("should not contain 'all models failed' for client cancellation, got: %s", body)
	}
}

func TestHandleNonStreaming_ParentDeadlineExceeded_No502(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "msg_1",
			"type": "message",
			"role": "assistant",
			"content": [{"type": "text", "text": "hello"}],
			"model": "kimi-k2.6",
			"stop_reason": "end_turn",
			"usage": {"input_tokens": 10, "output_tokens": 5}
		}`))
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		Models: map[string]config.ModelConfig{
			"default": {Provider: "opencode-go", ModelID: "kimi-k2.6"},
		},
		Fallbacks: map[string][]config.ModelConfig{
			"default": {{Provider: "opencode-go", ModelID: "glm-5"}},
		},
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:   upstream.URL,
			TimeoutMs: 5000,
		},
	}

	atomicCfg := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
	ocClient := client.NewOpenCodeClient(atomicCfg)
	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	m := metrics.New()
	handler := NewMessagesHandler(
		ocClient,
		nil, // providerRegistry
		modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 30*time.Second),
		tokenCounter,
		m,
	)
	handler.logger = slog.Default()

	requestBody := `{
		"model": "claude-haiku-4-5-20251001",
		"max_tokens": 256,
		"messages": [{"role": "user", "content": "Say hello"}]
	}`

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", strings.NewReader(requestBody))
	req.Header.Set("Content-Type", "application/json")

	ctx, cancel := context.WithDeadline(req.Context(), time.Now().Add(-1*time.Second))
	defer cancel()
	req = req.WithContext(ctx)

	handler.HandleMessages(recorder, req)

	if recorder.Code == http.StatusBadGateway {
		t.Errorf("should not return 502 for deadline exceeded, got status %d", recorder.Code)
	}
	snap := m.GetSnapshot()
	if snap.RequestsFailed > 0 {
		t.Errorf("failure count should be 0 for deadline exceeded, got %d", snap.RequestsFailed)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "all models failed") {
		t.Errorf("should not contain 'all models failed' for deadline exceeded, got: %s", body)
	}
}

func TestResponseWriter_ConcurrentWrites(t *testing.T) {
	recorder := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: recorder}

	var wg sync.WaitGroup
	const goroutines = 10
	const writesPerGoroutine = 100

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < writesPerGoroutine; j++ {
				_, _ = fmt.Fprintf(rw, "goroutine-%d-write-%d\n", id, j)
			}
		}(i)
	}
	wg.Wait()

	output := recorder.Body.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")
	expectedLines := goroutines * writesPerGoroutine
	if len(lines) != expectedLines {
		t.Errorf("got %d lines, want %d (possible data loss from unsynchronized writes)", len(lines), expectedLines)
	}
}

func TestHandleStreaming_AnthropicRaw_NoKeepaliveInjection(t *testing.T) {
	blockCh := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {\"type\":\"message_start\"}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		select {
		case <-blockCh:
		case <-time.After(10 * time.Second):
		}
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\"}\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer upstream.Close()

	handler := newStreamingTestHandler(t, upstream.URL)

	rawBody := json.RawMessage(`{
		"model": "claude-opus-4-8",
		"stream": true,
		"max_tokens": 256,
		"messages": [{"role":"user","content":"hello"}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "minimax-m3"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, &core.NormalizedRequest{Stream: true}, chain, rawBody)
	}()

	time.Sleep(1000 * time.Millisecond)
	close(blockCh)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleStreaming did not return after unblocking upstream")
	}

	body := recorder.Body.String()

	if !strings.Contains(body, "message_start") {
		t.Error("output missing message_start event")
	}
	if !strings.Contains(body, "content_block_delta") {
		t.Error("output missing content_block_delta event")
	}

	if strings.Contains(body, ":keepalive") {
		t.Errorf("keepalive comment leaked into Anthropic raw stream output (concurrent write bug):\n%s", body)
	}
}
