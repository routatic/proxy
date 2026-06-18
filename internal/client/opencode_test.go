package client

import (
	"testing"
	"time"

	"oc-go-cc/internal/config"
)

func TestIsAnthropicModelOnlyRoutesNativeAnthropicModels(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{
			name:    "minimax m2.5 uses openai endpoint on Go provider",
			modelID: "minimax-m2.5",
			want:    false,
		},
		{
			name:    "minimax m2.7 uses openai endpoint on Go provider",
			modelID: "minimax-m2.7",
			want:    false,
		},
		{
			name:    "minimax m3 uses openai endpoint on Go provider",
			modelID: "minimax-m3",
			want:    false,
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
			name:    "glm-5.1 uses openai endpoint",
			modelID: "glm-5.1",
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
			name:    "qwen3.5-plus uses openai endpoint on Go provider",
			modelID: "qwen3.5-plus",
			want:    false,
		},
		{
			name:    "qwen3.6-plus uses openai endpoint on Go provider",
			modelID: "qwen3.6-plus",
			want:    false,
		},
		{
			name:    "qwen3.7-plus uses openai endpoint on Go provider",
			modelID: "qwen3.7-plus",
			want:    false,
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
		{"gemini-3.5-flash", true},
		{"gemini-3.1-pro", true},
		{"gemini-3-flash", true},
		{"kimi-k2.6", false},
		{"glm-5.1", false},
		{"gpt-5.5", false},
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
		{"gpt-5.5", true},
		{"gpt-5.5-pro", true},
		{"gpt-5.4", true},
		{"gpt-5.4-pro", true},
		{"gpt-5.4-mini", true},
		{"gpt-5.4-nano", true},
		{"gpt-5.3-codex", true},
		{"gpt-5.3-codex-spark", true},
		{"gpt-5.2", true},
		{"gpt-5.2-codex", true},
		{"gpt-5.1", true},
		{"gpt-5.1-codex", true},
		{"gpt-5.1-codex-max", true},
		{"gpt-5.1-codex-mini", true},
		{"gpt-5", true},
		{"gpt-5-codex", true},
		{"gpt-5-nano", true},
		{"kimi-k2.6", false},
		{"glm-5.1", false},
		{"gemini-3.5-flash", false},
	}

	for _, tt := range tests {
		t.Run(tt.modelID, func(t *testing.T) {
			if got := isResponsesModel(tt.modelID); got != tt.want {
				t.Fatalf("isResponsesModel(%q) = %v, want %v", tt.modelID, got, tt.want)
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
