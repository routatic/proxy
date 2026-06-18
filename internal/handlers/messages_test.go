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

	"oc-go-cc/internal/client"
	"oc-go-cc/internal/config"
	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/router"
	"oc-go-cc/internal/token"
	"oc-go-cc/internal/transformer"
	"oc-go-cc/pkg/types"
)

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
		RespectRequestedModel: false,
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

// ---------------------------------------------------------------------------
// Phase 2 regression tests: replaceModelInRawBody (JSON-based replacement)
// ---------------------------------------------------------------------------

func TestReplaceModelInRawBody_JSONBased(t *testing.T) {
	raw := json.RawMessage(`{"model":"claude-opus-4-8","stream":true,"messages":[]}`)
	got := string(replaceModelInRawBody(raw, "minimax-m3"))

	if !strings.Contains(got, `"minimax-m3"`) {
		t.Fatalf("expected model replaced to minimax-m3, got: %s", got)
	}
	if !strings.Contains(got, `"stream":true`) {
		t.Fatalf("expected other fields preserved, got: %s", got)
	}
	if strings.Contains(got, `"claude-opus-4-8"`) {
		t.Fatalf("old model ID should be gone, got: %s", got)
	}
}

func TestReplaceModelInRawBody_HandlesWhitespace(t *testing.T) {
	raw := json.RawMessage(`{ "model" : "claude-opus-4-8" , "stream" : true }`)
	got := string(replaceModelInRawBody(raw, "minimax-m3"))

	if !strings.Contains(got, `"minimax-m3"`) {
		t.Fatalf("expected model replaced despite whitespace, got: %s", got)
	}
}

func TestReplaceModelInRawBody_ReturnsOriginalWhenModelMissing(t *testing.T) {
	raw := json.RawMessage(`{"stream":true,"messages":[]}`)
	got := replaceModelInRawBody(raw, "minimax-m3")

	// Should return original raw bytes since there's no "model" key
	var parsed map[string]interface{}
	if err := json.Unmarshal(got, &parsed); err != nil {
		t.Fatalf("result is invalid JSON: %v", err)
	}
	if _, ok := parsed["model"]; ok {
		t.Fatalf("model key should not be present in result when absent from input")
	}
}

func TestReplaceModelInRawBody_ReturnsOriginalOnInvalidJSON(t *testing.T) {
	raw := json.RawMessage(`{invalid}`)
	got := replaceModelInRawBody(raw, "minimax-m3")

	if string(got) != `{invalid}` {
		t.Fatalf("expected original body on invalid JSON, got: %s", got)
	}
}

func TestReplaceModelInRawBody_HandlesNestedObjects(t *testing.T) {
	raw := json.RawMessage(`{
		"model": "claude-opus-4-8",
		"messages": [{"role":"user","content":"hello"}],
		"tools": [{"name":"Bash","input_schema":{"type":"object","properties":{"cmd":{"type":"string"}}}}],
		"stream": true
	}`)
	got := string(replaceModelInRawBody(raw, "minimax-m3"))

	if !strings.Contains(got, `"minimax-m3"`) {
		t.Fatalf("expected model replaced to minimax-m3 in complex body, got: %s", got)
	}
	if !strings.Contains(got, `"Bash"`) {
		t.Fatalf("expected tool name Bash preserved, got: %s", got)
	}
	if !strings.Contains(got, `"input_schema"`) {
		t.Fatalf("expected input_schema preserved, got: %s", got)
	}
}

// ---------------------------------------------------------------------------
// Phase 2 regression tests: handleStreaming Go Anthropic-native branch
// ---------------------------------------------------------------------------

func TestHandleStreaming_GoAnthropicModel_SendsRawAnthropicBody(t *testing.T) {
	// Spin up a fake upstream that records the request body
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
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
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
		"tools": [{"name":"Bash","description":"Run a command","input_schema":{"type":"object","properties":{"cmd":{"type":"string"}}}}]
	}`)

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		t.Fatalf("unmarshal rawBody: %v", err)
	}

	// Call handleStreaming with minimax-m3 (Go Anthropic-native)
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "minimax-m3"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	// context is tied to the request lifetime; handleStreaming waits on it
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)

	// Verify the upstream received raw Anthropic format (not OpenAI-transformed)
	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	// Must have model = minimax-m3
	if got, ok := captured["model"]; !ok || got != "minimax-m3" {
		t.Fatalf("captured model = %v, want minimax-m3", got)
	}

	// Must have tools with input_schema (Anthropic format), NOT function (OpenAI format)
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

// TestHandleStreaming_GoAnthropicModel_FallsThroughOnError verifies that
// when the Go Anthropic-native model fails, the streaming handler falls
// through to the next model in the chain.
func TestHandleStreaming_GoAnthropicModel_FallsThroughOnError(t *testing.T) {
	// Single upstream: fails on first request, succeeds on second.
	// Both models in the chain use the same base URL.
	callCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			// First call (minimax-m3) fails
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// Second call (qwen3.5-plus) succeeds
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

	// Chain: minimax-m3 fails (first call → 500), qwen3.5-plus succeeds (second call)
	chain := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "minimax-m3"},
		{Provider: "opencode-go", ModelID: "qwen3.5-plus"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)

	// Both models tried: minimax got 500, qwen3.5-plus got 200
	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 2 {
		t.Fatalf("expected 2 upstream calls (1 fail + 1 success), got %d", finalCount)
	}
}

// newStreamingTestHandler creates a MessagesHandler for streaming tests,
// pointing both Go Anthropic and OpenAI endpoints at the given test server URL.
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

// ---------------------------------------------------------------------------
// End-to-end test: HandleMessages → routing → handleStreaming → upstream
// ---------------------------------------------------------------------------

// TestHandleMessages_StreamingMinimaxM3_UsesAnthropicEndpoint verifies the
// full public API path: HandleMessages receives a streaming request for
// minimax-m3, routing selects it (via ModelOverrides), and the upstream
// receives the raw Anthropic body (NOT OpenAI-transformed).
func TestHandleMessages_StreamingMinimaxM3_UsesAnthropicEndpoint(t *testing.T) {
	// 1. Set up fake upstream that records the request body.
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

	// 2. Build config that forces routing to minimax-m3.
	//    ModelOverrides takes highest precedence in buildModelChain.
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

	// 3. Build the full MessagesHandler with all real dependencies.
	ocClient := client.NewOpenCodeClient(atomicCfg)
	modelRouter := router.NewModelRouter(atomicCfg)
	tokenCounter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	handler := NewMessagesHandler(
		ocClient,
		modelRouter,
		nil, // fallbackHandler — not used in streaming path
		tokenCounter,
		metrics.New(),
	)
	handler.logger = slog.Default()

	// 4. Build the streaming request body requesting minimax-m3 with tools.
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

	// 5. Call HandleMessages — the full public entry point.
	handler.HandleMessages(recorder, req)

	// 6. Verify upstream received raw Anthropic body.
	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body — routing or streaming may have failed silently")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	// Model must be minimax-m3
	if got, ok := captured["model"]; !ok || got != "minimax-m3" {
		t.Fatalf("captured model = %v, want minimax-m3", got)
	}

	// Tools must be Anthropic format (input_schema), NOT OpenAI format (function)
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
		t.Fatalf("captured tool has 'function' field (OpenAI format leak — TransformRequest was called): %s", capturedBody)
	}
	if _, ok := tool0["input_schema"]; !ok {
		t.Fatalf("captured tool missing 'input_schema' (Anthropic format): %s", capturedBody)
	}
	if got, ok := tool0["name"]; !ok || got != "Bash" {
		t.Fatalf("captured tool name = %v, want Bash", got)
	}

	t.Logf("end-to-end test PASSED: upstream received raw Anthropic body with model=minimax-m3 and input_schema")
}

// ---------------------------------------------------------------------------
// Non-streaming regression tests: handleNonStreaming model replacement
// ---------------------------------------------------------------------------

// TestHandleNonStreaming_GoAnthropicModel_ReplacesModelInBody verifies that
// the non-streaming path replaces the model in the request body for Go
// Anthropic-native models (minimax-m3) before forwarding to upstream.
// Without this fix, upstream would receive "claude-haiku-4-5-20251001" and
// reject it with "Model is not supported".
func TestHandleNonStreaming_GoAnthropicModel_ReplacesModelInBody(t *testing.T) {
	var capturedBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		capturedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Logf("upstream read body error: %v", err)
		}
		// Non-streaming: return a valid JSON response
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
		modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 30),
		tokenCounter,
		metrics.New(),
	)
	handler.logger = slog.Default()

	// Use a different client model to verify the model is replaced to
	// minimax-m3 before sending upstream.
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

	// Verify upstream received the request body with model replaced
	if len(capturedBody) == 0 {
		t.Fatal("upstream received no body")
	}

	var captured map[string]interface{}
	if err := json.Unmarshal(capturedBody, &captured); err != nil {
		t.Fatalf("captured body is not valid JSON: %v\nbody: %s", err, capturedBody)
	}

	// Must have model = minimax-m3
	if got, ok := captured["model"]; !ok || got != "minimax-m3" {
		t.Fatalf("captured model = %v, want minimax-m3", got)
	}

	// Must have tools with input_schema (Anthropic format), NOT function (OpenAI)
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

	t.Logf("non-streaming Go Anthropic-native test PASSED: upstream received model=minimax-m3 with Anthropic tool format")
}

// TestHandleNonStreaming_ZenAnthropicModel_ReplacesModelInBody verifies that
// the non-streaming path replaces the model in the request body for Zen
// Anthropic-native models (claude-* via opencode-zen) before forwarding upstream.
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
		modelRouter,
		router.NewFallbackHandler(slog.Default(), 3, 30),
		tokenCounter,
		metrics.New(),
	)
	handler.logger = slog.Default()

	// Use a different client model to verify the model is replaced to
	// claude-sonnet-4.5 before sending upstream.
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

	// Must have model = claude-sonnet-4.5 (replaced from claude-haiku-4-5-20251001)
	if got, ok := captured["model"]; !ok || got != "claude-sonnet-4.5" {
		t.Fatalf("captured model = %v, want claude-sonnet-4.5", got)
	}

	// Must have tools with input_schema (Anthropic format), NOT function (OpenAI)
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

	t.Logf("non-streaming Zen Anthropic test PASSED: upstream received model=claude-sonnet-4.5 with Anthropic tool format")
}

func TestHandleStreaming_ConfigurableTimeout(t *testing.T) {
	callCount := int32(0)
	handlerCtx, handlerCancel := context.WithCancel(context.Background())
	defer handlerCancel()
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		<-handlerCtx.Done()
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:            upstream.URL,
			AnthropicBaseURL:   upstream.URL,
			TimeoutMs:          5000,
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
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()

	start := time.Now()
	handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)
	elapsed := time.Since(start)

	handlerCancel()

	if elapsed > 10*time.Second {
		t.Errorf("streaming attempt took %v, expected much less than 2 minutes", elapsed)
	}

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 1 {
		t.Errorf("expected 1 upstream call (single model in chain), got %d", finalCount)
	}
}

func TestHandleStreaming_ClientContextCanceled_StopsFallback(t *testing.T) {
	callCount := int32(0)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		<-r.Context().Done()
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:          upstream.URL,
			AnthropicBaseURL: upstream.URL,
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
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())

	cancel()

	handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)

	time.Sleep(50 * time.Millisecond)

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 0 {
		t.Errorf("expected 0 upstream calls (client canceled), got %d", finalCount)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "all upstream models failed") {
		t.Errorf("should not send 'all upstream models failed' event for client disconnect, got: %s", body)
	}
}

func TestHandleStreaming_ClientDisconnectsDuringStream_StopsFallback(t *testing.T) {
	callCount := int32(0)
	handlerCtx, handlerCancel := context.WithCancel(context.Background())
	defer handlerCancel()
	firstModelReady := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			w.WriteHeader(http.StatusOK)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			close(firstModelReady)
			<-handlerCtx.Done()
			return
		}
		t.Error("second model should not be attempted after client disconnect")
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:          upstream.URL,
			AnthropicBaseURL: upstream.URL,
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
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)
	}()

	select {
	case <-firstModelReady:
	case <-time.After(5 * time.Second):
		t.Fatal("first model did not start within 5s")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("handleStreaming did not return after client disconnect")
	}

	handlerCancel()

	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 1 {
		t.Errorf("expected 1 upstream call, got %d", finalCount)
	}

	body := recorder.Body.String()
	if strings.Contains(body, "all upstream models failed") {
		t.Errorf("should not send 'all upstream models failed' event for client disconnect, got: %s", body)
	}
}

func TestHandleStreaming_PerModelTimeoutFallback(t *testing.T) {
	callCount := int32(0)
	handlerCtx, handlerCancel := context.WithCancel(context.Background())
	defer handlerCancel()
	firstModelReady := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		count := atomic.AddInt32(&callCount, 1)
		if count == 1 {
			close(firstModelReady)
			<-handlerCtx.Done()
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprintf(w, "event: message_start\ndata: {}\n\n")
		_, _ = fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"delta\":{\"type\":\"text_delta\",\"text\":\"ok\"}}\n\n")
		_, _ = fmt.Fprintf(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\"},\"usage\":{\"output_tokens\":1}}\n\n")
		_, _ = fmt.Fprintf(w, "event: message_stop\ndata: {}\n\n")
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}))
	defer upstream.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:            upstream.URL,
			AnthropicBaseURL:   upstream.URL,
			TimeoutMs:          5000,
			StreamingTimeoutMs: 200,
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
		{Provider: "opencode-go", ModelID: "kimi-k2.6"},
		{Provider: "opencode-go", ModelID: "glm-5"},
	}

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages", nil)
	ctx, cancel := context.WithCancel(req.Context())

	done := make(chan struct{})
	go func() {
		defer close(done)
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)
		cancel()
	}()

	select {
	case <-firstModelReady:
	case <-time.After(5 * time.Second):
		t.Fatal("first model did not start within 5s")
	}

	time.Sleep(500 * time.Millisecond)

	handlerCancel()

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

// TestResponseWriter_ConcurrentWrites verifies the mutex serializes writes,
// preventing data races when heartbeat and stream copy write concurrently.
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
				rw.Write([]byte(fmt.Sprintf("goroutine-%d-write-%d\n", id, j)))
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

// TestHandleStreaming_AnthropicRaw_NoKeepaliveInjection verifies that the
// heartbeat is disabled during Anthropic raw passthrough. The upstream sends
// SSE data slowly (blocking for > heartbeat interval) and the proxy must
// not inject keepalive comments into the raw stream.
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
		handler.handleStreaming(recorder, req.WithContext(ctx), &anthropicReq, chain, rawBody)
	}()

	time.Sleep(3500 * time.Millisecond)
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
