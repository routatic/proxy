package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/pkg/types"
)

func TestIsAnthropicModelOnlyRoutesNativeAnthropicModels(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{
			name:    "minimax m2.5 uses anthropic endpoint on Go provider",
			modelID: "minimax-m2.5",
			want:    true,
		},
		{
			name:    "minimax m2.7 uses anthropic endpoint on Go provider",
			modelID: "minimax-m2.7",
			want:    true,
		},
		{
			name:    "minimax m3 uses anthropic endpoint on Go provider",
			modelID: "minimax-m3",
			want:    true,
		},
		{
			name:    "deepseek pro uses openai endpoint",
			modelID: "deepseek-v4-pro",
			want:    false,
		},
		{
			name:    "deepseek flash uses openai endpoint",
			modelID: "deepseek-v4-flash",
			want:    false,
		},
		{
			name:    "kimi k2.6 uses openai endpoint",
			modelID: "kimi-k2.6",
			want:    false,
		},
		{
			name:    "kimi k2.7-code uses openai endpoint",
			modelID: "kimi-k2.7-code",
			want:    false,
		},
		{
			name:    "kimi k3 uses openai endpoint",
			modelID: "kimi-k3",
			want:    false,
		},
		{
			name:    "glm-5.1 uses openai endpoint",
			modelID: "glm-5.1",
			want:    false,
		},
		{
			name:    "glm-5.2 uses openai endpoint",
			modelID: "glm-5.2",
			want:    false,
		},
		{
			name:    "glm-5 uses openai endpoint",
			modelID: "glm-5",
			want:    false,
		},
		{
			name:    "kimi-k2.5 uses openai endpoint",
			modelID: "kimi-k2.5",
			want:    false,
		},
		{
			name:    "mimo-v2.5 uses openai endpoint",
			modelID: "mimo-v2.5",
			want:    false,
		},
		{
			name:    "mimo-v2.5-pro uses openai endpoint",
			modelID: "mimo-v2.5-pro",
			want:    false,
		},
		{
			name:    "qwen3.5-plus uses anthropic endpoint on Go provider",
			modelID: "qwen3.5-plus",
			want:    true,
		},
		{
			name:    "qwen3.6-plus uses anthropic endpoint on Go provider",
			modelID: "qwen3.6-plus",
			want:    true,
		},
		{
			name:    "qwen3.7-plus uses anthropic endpoint on Go provider",
			modelID: "qwen3.7-plus",
			want:    true,
		},
		{
			name:    "qwen3.7-max uses anthropic endpoint (no oa-compat support)",
			modelID: "qwen3.7-max",
			want:    true,
		},
		{
			name:    "claude models use openai endpoint on Go provider",
			modelID: "claude-sonnet-4-5",
			want:    false,
		},
		{
			name:    "claude-opus-4-7 uses openai endpoint on Go provider",
			modelID: "claude-opus-4-7",
			want:    false,
		},
		{
			name:    "claude-haiku-4-5 uses openai endpoint on Go provider",
			modelID: "claude-haiku-4-5",
			want:    false,
		},
		{
			name:    "claude-3-5-haiku uses openai endpoint on Go provider",
			modelID: "claude-3-5-haiku",
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsAnthropicModel(tt.modelID); got != tt.want {
				t.Fatalf("IsAnthropicModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestProvider(t *testing.T) {
	tests := []struct {
		name     string
		model    config.ModelConfig
		expected string
	}{
		{
			name:     "empty provider defaults to opencode-go",
			model:    config.ModelConfig{ModelID: "test-model"},
			expected: ProviderOpenCodeGo,
		},
		{
			name:     "explicit opencode-go provider",
			model:    config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"},
			expected: ProviderOpenCodeGo,
		},
		{
			name:     "explicit opencode-zen provider",
			model:    config.ModelConfig{Provider: ProviderOpenCodeZen, ModelID: "test-model"},
			expected: ProviderOpenCodeZen,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Provider(tt.model); got != tt.expected {
				t.Fatalf("Provider() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestIsZen(t *testing.T) {
	tests := []struct {
		name     string
		model    config.ModelConfig
		expected bool
	}{
		{
			name:     "opencode-go is not zen",
			model:    config.ModelConfig{Provider: ProviderOpenCodeGo},
			expected: false,
		},
		{
			name:     "opencode-zen is zen",
			model:    config.ModelConfig{Provider: ProviderOpenCodeZen},
			expected: true,
		},
		{
			name:     "empty provider is not zen",
			model:    config.ModelConfig{},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsZen(tt.model); got != tt.expected {
				t.Fatalf("IsZen() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestClassifyEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		modelID  string
		expected EndpointType
	}{
		{
			name:     "minimax m2.5 uses chat completions on Zen",
			modelID:  "minimax-m2.5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "minimax m2.7 uses chat completions on Zen",
			modelID:  "minimax-m2.7",
			expected: EndpointChatCompletions,
		},
		{
			name:     "minimax m3 uses chat completions on Zen",
			modelID:  "minimax-m3",
			expected: EndpointChatCompletions,
		},
		{
			name:     "qwen3.5-plus uses anthropic endpoint",
			modelID:  "qwen3.5-plus",
			expected: EndpointAnthropic,
		},
		{
			name:     "qwen3.6-plus uses anthropic endpoint",
			modelID:  "qwen3.6-plus",
			expected: EndpointAnthropic,
		},
		{
			name:     "qwen3.7-plus uses anthropic endpoint",
			modelID:  "qwen3.7-plus",
			expected: EndpointAnthropic,
		},
		{
			name:     "qwen3.7-max uses anthropic endpoint",
			modelID:  "qwen3.7-max",
			expected: EndpointAnthropic,
		},
		{
			name:     "gemini-3.5-flash uses gemini endpoint",
			modelID:  "gemini-3.5-flash",
			expected: EndpointGemini,
		},
		{
			name:     "gemini-3.1-pro uses gemini endpoint",
			modelID:  "gemini-3.1-pro",
			expected: EndpointGemini,
		},
		{
			name:     "gemini-3-flash uses gemini endpoint",
			modelID:  "gemini-3-flash",
			expected: EndpointGemini,
		},
		{
			name:     "gpt-5.5 uses responses endpoint",
			modelID:  "gpt-5.5",
			expected: EndpointResponses,
		},
		{
			name:     "gpt-5.4 uses responses endpoint",
			modelID:  "gpt-5.4",
			expected: EndpointResponses,
		},
		{
			name:     "gpt-5 uses responses endpoint",
			modelID:  "gpt-5",
			expected: EndpointResponses,
		},
		{
			name:     "kimi-k2.6 uses chat completions endpoint",
			modelID:  "kimi-k2.6",
			expected: EndpointChatCompletions,
		},
		{
			name:     "kimi-k2.7-code uses chat completions endpoint",
			modelID:  "kimi-k2.7-code",
			expected: EndpointChatCompletions,
		},
		{
			name:     "kimi-k2.5 uses chat completions endpoint",
			modelID:  "kimi-k2.5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "mimo-v2.5 uses chat completions endpoint",
			modelID:  "mimo-v2.5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "mimo-v2.5-pro uses chat completions endpoint",
			modelID:  "mimo-v2.5-pro",
			expected: EndpointChatCompletions,
		},
		{
			name:     "glm-5.1 uses chat completions endpoint",
			modelID:  "glm-5.1",
			expected: EndpointChatCompletions,
		},
		{
			name:     "glm-5.2 uses chat completions endpoint",
			modelID:  "glm-5.2",
			expected: EndpointChatCompletions,
		},
		{
			name:     "glm-5 uses chat completions endpoint",
			modelID:  "glm-5",
			expected: EndpointChatCompletions,
		},
		{
			name:     "deepseek-v4-flash uses chat completions endpoint",
			modelID:  "deepseek-v4-flash",
			expected: EndpointChatCompletions,
		},
		{
			name:     "grok-build-0.1 uses chat completions endpoint",
			modelID:  "grok-build-0.1",
			expected: EndpointChatCompletions,
		},
		{
			name:     "big-pickle uses chat completions endpoint",
			modelID:  "big-pickle",
			expected: EndpointChatCompletions,
		},
		{
			name:     "north-mini-code-free uses chat completions endpoint",
			modelID:  "north-mini-code-free",
			expected: EndpointChatCompletions,
		},
		{
			name:     "deepseek-v4-flash-free uses chat completions endpoint",
			modelID:  "deepseek-v4-flash-free",
			expected: EndpointChatCompletions,
		},
		{
			name:     "claude-sonnet-4-5 uses anthropic endpoint",
			modelID:  "claude-sonnet-4-5",
			expected: EndpointAnthropic,
		},
		{
			name:     "claude-opus-4-7 uses anthropic endpoint",
			modelID:  "claude-opus-4-7",
			expected: EndpointAnthropic,
		},
		{
			name:     "claude-haiku-4-5 uses anthropic endpoint",
			modelID:  "claude-haiku-4-5",
			expected: EndpointAnthropic,
		},
		{
			name:     "unknown model uses chat completions endpoint",
			modelID:  "unknown-model",
			expected: EndpointChatCompletions,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyEndpoint(tt.modelID); got != tt.expected {
				t.Fatalf("ClassifyEndpoint(%q) = %v, want %v", tt.modelID, got, tt.expected)
			}
		})
	}
}

func TestIsGeminiModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		// Gemini models
		{"gemini-3.5-flash", true},
		{"gemini-3.1-pro", true},
		{"gemini-3-flash", true},
		// Non-Gemini models
		{"kimi-k2.6", false},
		{"kimi-k2.7-code", false},
		{"glm-5.1", false},
		{"glm-5.2", false},
		{"glm-5", false},
		{"gpt-5.5", false},
		{"gpt-5", false},
		{"claude-sonnet-4-5", false},
		{"qwen3.7-plus", false},
		{"deepseek-v4-pro", false},
		{"mimo-v2.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := isGeminiModel(tt.modelID); got != tt.want {
				t.Fatalf("isGeminiModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestIsResponsesModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		// GPT 5.5 series
		{"gpt-5.5", true},
		{"gpt-5.5-pro", true},
		{"gpt-5.5-mini", true},
		{"gpt-5.5-nano", true},
		// GPT 5.4 series
		{"gpt-5.4", true},
		{"gpt-5.4-pro", true},
		{"gpt-5.4-mini", true},
		{"gpt-5.4-nano", true},
		// GPT 5.3 series
		{"gpt-5.3-codex", true},
		{"gpt-5.3-codex-spark", true},
		// GPT 5.2 series
		{"gpt-5.2", true},
		{"gpt-5.2-codex", true},
		// GPT 5.1 series
		{"gpt-5.1", true},
		{"gpt-5.1-codex", true},
		{"gpt-5.1-codex-max", true},
		{"gpt-5.1-codex-mini", true},
		// GPT 5 series
		{"gpt-5", true},
		{"gpt-5-codex", true},
		{"gpt-5-nano", true},
		// Non-GPT models
		{"kimi-k2.6", false},
		{"kimi-k2.7-code", false},
		{"glm-5.1", false},
		{"glm-5.2", false},
		{"glm-5", false},
		{"gemini-3.5-flash", false},
		{"gemini-3.1-pro", false},
		{"gemini-3-flash", false},
		{"claude-sonnet-4-5", false},
		{"qwen3.7-plus", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := isResponsesModel(tt.modelID); got != tt.want {
				t.Fatalf("isResponsesModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestIsZenAnthropicModel(t *testing.T) {
	tests := []struct {
		modelID string
		want    bool
	}{
		// Claude models on Zen use Anthropic endpoint
		{"claude-sonnet-4-5", true},
		{"claude-opus-4-7", true},
		{"claude-haiku-4-5", true},
		{"claude-3-5-haiku", true},
		{"claude-3-5-sonnet", true},
		{"claude-3-opus", true},
		// Qwen models on Zen use Anthropic endpoint
		{"qwen3.7-max", true},
		{"qwen3.7-plus", true},
		{"qwen3.6-plus", true},
		{"qwen3.5-plus", true},
		{"qwen3.5", true},
		// Non-Anthropic models
		{"kimi-k2.6", false},
		{"kimi-k2.7-code", false},
		{"glm-5.1", false},
		{"glm-5.2", false},
		{"glm-5", false},
		{"gemini-3.5-flash", false},
		{"gemini-3.1-pro", false},
		{"gpt-5.5", false},
		{"gpt-5", false},
		{"minimax-m2.5", false},
		{"minimax-m2.7", false},
		{"minimax-m3", false},
		{"deepseek-v4-pro", false},
		{"mimo-v2.5", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := isZenAnthropicModel(tt.modelID); got != tt.want {
				t.Fatalf("isZenAnthropicModel(%q) = %v, want %v", tt.modelID, got, tt.want)
			}
		})
	}
}

func TestNextAPIKey_RoundRobin(t *testing.T) {
	cfg := &config.Config{
		APIKeys: []string{"key-a", "key-b", "key-c"},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := &OpenCodeClient{
		atomic: atomicCfg,
	}

	// With 3 keys, iteration 0..5 should cycle key-a, key-b, key-c, key-a, key-b, key-c
	expected := []string{"key-a", "key-b", "key-c", "key-a", "key-b", "key-c"}
	for i, want := range expected {
		if got := c.nextAPIKey(cfg.EffectiveAPIKeys()); got != want {
			t.Errorf("iteration %d: nextAPIKey() = %q, want %q", i, got, want)
		}
	}
}

func TestNextAPIKey_SingleKey(t *testing.T) {
	cfg := &config.Config{APIKey: "single"}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := &OpenCodeClient{atomic: atomicCfg}

	for i := 0; i < 5; i++ {
		if got := c.nextAPIKey(cfg.EffectiveAPIKeys()); got != "single" {
			t.Errorf("iteration %d: nextAPIKey() = %q, want %q", i, got, "single")
		}
	}
}

func TestNextAPIKey_EmptyKeys(t *testing.T) {
	cfg := &config.Config{APIKey: ""}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := &OpenCodeClient{atomic: atomicCfg}

	if got := c.nextAPIKey(cfg.EffectiveAPIKeys()); got != "" {
		t.Errorf("nextAPIKey() = %q, want empty string", got)
	}
}

func TestNextAPIKey_ConcurrentSafety(t *testing.T) {
	cfg := &config.Config{
		APIKeys: []string{"k1", "k2", "k3"},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := &OpenCodeClient{atomic: atomicCfg}

	const goroutines = 3
	const callsPerGoroutine = 100
	results := make(chan string, goroutines*callsPerGoroutine)

	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < callsPerGoroutine; i++ {
				results <- c.nextAPIKey(cfg.EffectiveAPIKeys())
			}
		}()
	}

	seen := make(map[string]int)
	for i := 0; i < goroutines*callsPerGoroutine; i++ {
		key := <-results
		seen[key]++
	}

	// Each key should be seen exactly (goroutines*callsPerGoroutine)/3 times
	total := goroutines * callsPerGoroutine
	expectedPerKey := total / len(cfg.APIKeys)
	for _, key := range cfg.APIKeys {
		if seen[key] != expectedPerKey {
			t.Errorf("key %q seen %d times, want %d", key, seen[key], expectedPerKey)
		}
	}
}

func TestStreamIdleTimeout(t *testing.T) {
	tests := []struct {
		name     string
		goMs     int
		zenMs    int
		provider string
		wantDur  time.Duration
	}{
		{
			name:     "Go provider uses OpenCodeGo.StreamTimeoutMs",
			goMs:     120000, // 2 min
			provider: "opencode-go",
			wantDur:  120 * time.Second,
		},
		{
			name:     "Zen provider uses OpenCodeZen.StreamTimeoutMs",
			goMs:     100000,
			zenMs:    600000, // 10 min
			provider: "opencode-zen",
			wantDur:  10 * time.Minute,
		},
		{
			name:     "falls back to OpenCodeGo.TimeoutMs when StreamTimeoutMs is zero",
			goMs:     300000, // 5 min
			provider: "opencode-go",
			wantDur:  5 * time.Minute,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				OpenCodeGo:  config.OpenCodeGoConfig{TimeoutMs: tt.goMs, StreamTimeoutMs: tt.goMs},
				OpenCodeZen: config.OpenCodeZenConfig{TimeoutMs: tt.zenMs, StreamTimeoutMs: tt.zenMs},
			}
			// Fallback test: zero out StreamTimeoutMs for that provider.
			if tt.name == "falls back to OpenCodeGo.TimeoutMs when StreamTimeoutMs is zero" {
				cfg.OpenCodeGo.StreamTimeoutMs = 0
			}
			atomic := config.NewAtomicConfig(cfg, "/tmp/test-config.json")
			c := &OpenCodeClient{atomic: atomic}
			mc := config.ModelConfig{Provider: tt.provider, ModelID: "test-model"}
			got := c.StreamIdleTimeout(mc)
			if got != tt.wantDur {
				t.Errorf("StreamIdleTimeout() = %v, want %v", got, tt.wantDur)
			}
		})
	}
}

func TestRequestTimeout_UsesConfiguredTimeout(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs: 120000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.RequestTimeout(model)
	if timeout != 120*time.Second {
		t.Errorf("RequestTimeout = %v, want 120s", timeout)
	}
}

func TestRequestTimeout_FallsBackToDefault(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.RequestTimeout(model)
	if timeout != 5*time.Minute {
		t.Errorf("RequestTimeout = %v, want 5m", timeout)
	}
}

func TestRequestTimeout_ZenProvider(t *testing.T) {
	cfg := &config.Config{
		OpenCodeZen: config.OpenCodeZenConfig{
			TimeoutMs: 60000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeZen, ModelID: "claude-sonnet-4.5"}
	timeout := c.RequestTimeout(model)
	if timeout != 60*time.Second {
		t.Errorf("RequestTimeout = %v, want 60s", timeout)
	}
}

func TestStreamingTimeout_UsesStreamingTimeoutMs(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 600000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 600*time.Second {
		t.Errorf("StreamingTimeout = %v, want 600s", timeout)
	}
}

func TestStreamingTimeout_FallsBackToTimeoutMs(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 300*time.Second {
		t.Errorf("StreamingTimeout = %v, want 300s (fallback to timeout_ms)", timeout)
	}
}

func TestStreamingTimeout_FallsBackToDefault(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          0,
			StreamingTimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 5*time.Minute {
		t.Errorf("StreamingTimeout = %v, want 5m", timeout)
	}
}

func TestStreamingTimeout_ZenProvider(t *testing.T) {
	cfg := &config.Config{
		OpenCodeZen: config.OpenCodeZenConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 600000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeZen, ModelID: "claude-sonnet-4.5"}
	timeout := c.StreamingTimeout(model)
	if timeout != 600*time.Second {
		t.Errorf("StreamingTimeout = %v, want 600s", timeout)
	}
}

func TestStreamingTimeout_SmallConfiguredValue(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			TimeoutMs:          300000,
			StreamingTimeoutMs: 100,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "kimi-k2.6"}
	timeout := c.StreamingTimeout(model)
	if timeout != 100*time.Millisecond {
		t.Errorf("StreamingTimeout = %v, want 100ms", timeout)
	}
}

func TestGetProviderAPIKeys_ProviderSpecificKeys(t *testing.T) {
	cfg := &config.Config{
		OpenCodeGo: config.OpenCodeGoConfig{
			APIKeys: []string{"go-key-1", "go-key-2"},
		},
		OpenCodeZen: config.OpenCodeZenConfig{
			APIKey: "zen-specific-key",
		},
		AWSBedrock: config.AWSBedrockConfig{
			APIKeys: []string{"bedrock-key-1", "bedrock-key-2"},
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	tests := []struct {
		name     string
		provider string
		want     []string
	}{
		{
			name:     "OpenCode Go uses its own keys",
			provider: ProviderOpenCodeGo,
			want:     []string{"go-key-1", "go-key-2"},
		},
		{
			name:     "OpenCode Zen uses its own key",
			provider: ProviderOpenCodeZen,
			want:     []string{"zen-specific-key"},
		},
		{
			name:     "AWS Bedrock uses its own keys",
			provider: ProviderAWSBedrock,
			want:     []string{"bedrock-key-1", "bedrock-key-2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := config.ModelConfig{Provider: tt.provider, ModelID: "test-model"}
			got := c.getProviderAPIKeys(model)
			if len(got) != len(tt.want) {
				t.Errorf("getProviderAPIKeys() = %v, want %v", got, tt.want)
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("getProviderAPIKeys()[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestGetProviderAPIKeys_FallbackToGlobal(t *testing.T) {
	cfg := &config.Config{
		APIKeys:    []string{"global-key-1", "global-key-2"},
		OpenCodeGo: config.OpenCodeGoConfig{
			// No provider-specific keys
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"}
	got := c.getProviderAPIKeys(model)

	want := []string{"global-key-1", "global-key-2"}
	if len(got) != len(want) {
		t.Errorf("getProviderAPIKeys() = %v, want %v (fallback to global)", got, want)
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("getProviderAPIKeys()[%d] = %q, want %q (fallback to global)", i, got[i], want[i])
		}
	}
}

func TestGetProviderAPIKeys_ProviderKeysPrecedence(t *testing.T) {
	cfg := &config.Config{
		APIKeys: []string{"global-key"},
		OpenCodeGo: config.OpenCodeGoConfig{
			APIKeys: []string{"go-specific-key"},
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"}
	got := c.getProviderAPIKeys(model)

	// Should use provider-specific keys, not global
	want := []string{"go-specific-key"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("getProviderAPIKeys() = %v, want %v (provider keys should take precedence)", got, want)
	}
}

func TestOpenRouterKeys(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			APIKey: "openrouter-specific-key",
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}
	got := c.getProviderAPIKeys(model)

	want := []string{"openrouter-specific-key"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("getProviderAPIKeys() = %v, want %v", got, want)
	}
}

func TestOpenRouterEndpoint(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  "openrouter-key",
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}
	endpoint := c.getEndpoint("openrouter-model", model)

	if endpoint.BaseURL != cfg.OpenRouter.BaseURL {
		t.Errorf("getEndpoint BaseURL = %q, want %q", endpoint.BaseURL, cfg.OpenRouter.BaseURL)
	}
	if endpoint.APIKey != cfg.OpenRouter.APIKey {
		t.Errorf("getEndpoint APIKey = %q, want %q", endpoint.APIKey, cfg.OpenRouter.APIKey)
	}
}

func TestOpenRouterTimeout(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			TimeoutMs:          120000,
			StreamTimeoutMs:    180000,
			StreamingTimeoutMs: 240000,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}

	if got := c.RequestTimeout(model); got != 120*time.Second {
		t.Errorf("RequestTimeout = %v, want 120s", got)
	}
	if got := c.StreamIdleTimeout(model); got != 180*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 180s", got)
	}
	if got := c.StreamingTimeout(model); got != 240*time.Second {
		t.Errorf("StreamingTimeout = %v, want 240s", got)
	}
}

func TestOpenRouterChatCompletion_UsesBearerAuth(t *testing.T) {
	var gotURL string
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-1","object":"chat.completion","created":1,"model":"openrouter/model","choices":[],"usage":{}}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			BaseURL: ts.URL,
			APIKey:  "openrouter-key",
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter/model"}
	req := &types.ChatCompletionRequest{
		Model:    "openrouter/model",
		Messages: []types.ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	}
	_, err := c.ChatCompletionNonStreaming(context.Background(), "openrouter/model", req, model)
	if err != nil {
		t.Fatalf("ChatCompletionNonStreaming() error = %v", err)
	}

	if gotURL != "/" {
		t.Errorf("request URL = %q, want %q", gotURL, "/")
	}
	if gotAuth != "Bearer openrouter-key" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer openrouter-key")
	}
}

func TestOpenRouterChatCompletion_UsesOpenRouterBaseURL(t *testing.T) {
	var gotURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp-2","object":"chat.completion","created":2,"model":"openrouter/model","choices":[],"usage":{}}`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			BaseURL: ts.URL,
			APIKey:  "openrouter-key",
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter/model"}
	req := &types.ChatCompletionRequest{
		Model:    "openrouter/model",
		Messages: []types.ChatMessage{{Role: "user", Content: json.RawMessage(`"hello"`)}},
	}
	_, err := c.ChatCompletionNonStreaming(context.Background(), "openrouter/model", req, model)
	if err != nil {
		t.Fatalf("ChatCompletionNonStreaming() error = %v", err)
	}

	if gotURL != "/" {
		t.Errorf("request URL = %q, want %q", gotURL, "/")
	}
}

func TestOpenRouterTimeout_FallsBackToTimeoutMs(t *testing.T) {
	cfg := &config.Config{
		OpenRouter: config.OpenRouterConfig{
			TimeoutMs:          120000,
			StreamTimeoutMs:    0,
			StreamingTimeoutMs: 0,
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenRouter, ModelID: "openrouter-model"}

	if got := c.StreamIdleTimeout(model); got != 120*time.Second {
		t.Errorf("StreamIdleTimeout = %v, want 120s", got)
	}
	if got := c.StreamingTimeout(model); got != 120*time.Second {
		t.Errorf("StreamingTimeout = %v, want 120s", got)
	}
}

func TestGetProviderAPIKeys_EmptyReturnsGlobal(t *testing.T) {
	cfg := &config.Config{
		APIKey:     "global-single-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			// No keys configured
		},
	}
	atomicCfg := config.NewAtomicConfig(cfg, "")
	c := NewOpenCodeClient(atomicCfg, nil)

	model := config.ModelConfig{Provider: ProviderOpenCodeGo, ModelID: "test-model"}
	got := c.getProviderAPIKeys(model)

	want := []string{"global-single-key"}
	if len(got) != len(want) || got[0] != want[0] {
		t.Errorf("getProviderAPIKeys() = %v, want %v (should fallback to global)", got, want)
	}
}
