package client

import (
	"testing"

	"oc-go-cc/internal/config"
)

func TestIsAnthropicModelOnlyRoutesNativeAnthropicModels(t *testing.T) {
	tests := []struct {
		name    string
		modelID string
		want    bool
	}{
		{
			name:    "minimax m2.5 uses anthropic endpoint",
			modelID: "minimax-m2.5",
			want:    true,
		},
		{
			name:    "minimax m2.7 uses anthropic endpoint",
			modelID: "minimax-m2.7",
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
			name:    "glm-5.1 uses openai endpoint",
			modelID: "glm-5.1",
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
			name:     "minimax m2.5 uses anthropic endpoint",
			modelID:  "minimax-m2.5",
			expected: EndpointAnthropic,
		},
		{
			name:     "minimax m2.7 uses anthropic endpoint",
			modelID:  "minimax-m2.7",
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
