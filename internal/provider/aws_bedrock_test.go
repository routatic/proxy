package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func TestAWSBedrockProvider_Name(t *testing.T) {
	p := NewAWSBedrockProvider(nil)
	if got := p.Name(); got != "aws-bedrock" {
		t.Errorf("Name() = %q, want %q", got, "aws-bedrock")
	}
}

func TestAWSBedrockProvider_WireFormat(t *testing.T) {
	cfg := &config.Config{}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)
	if got := p.WireFormat("any-model"); got != core.WireFormatOpenAIChat {
		t.Errorf("WireFormat() = %v, want WireFormatOpenAIChat", got)
	}
}

func TestAWSBedrockProvider_WireFormat_Anthropic(t *testing.T) {
	cfg := &config.Config{
		AWSBedrock: config.AWSBedrockConfig{
			WireFormat: "anthropic",
		},
	}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)
	if got := p.WireFormat("any-model"); got != core.WireFormatAnthropic {
		t.Errorf("WireFormat() = %v, want WireFormatAnthropic", got)
	}
}

func TestAWSBedrockProvider_RoundTripName(t *testing.T) {
	p := NewAWSBedrockProvider(nil)
	model := config.ModelConfig{ModelID: "moonshotai.kimi-k2.5"}
	if got := p.RoundTripName(model); got != "moonshotai.kimi-k2.5" {
		t.Errorf("RoundTripName() = %q, want %q", got, "moonshotai.kimi-k2.5")
	}
}

func TestAWSBedrockProvider_Capabilities(t *testing.T) {
	p := NewAWSBedrockProvider(nil)
	caps := p.Capabilities()
	if !caps.SupportsStreaming {
		t.Error("SupportsStreaming = false, want true")
	}
	if !caps.SupportsTools {
		t.Error("SupportsTools = false, want true")
	}
	if !caps.SupportsImageInput {
		t.Error("SupportsImageInput = false, want true")
	}
}

func TestAWSBedrockProvider_ModelCapabilities(t *testing.T) {
	p := NewAWSBedrockProvider(nil)
	caps, ok := p.ModelCapabilities("any-model")
	if !ok {
		t.Error("ModelCapabilities() returned false, want true")
	}
	if !caps.SupportsStreaming {
		t.Error("ModelCapabilities().SupportsStreaming = false, want true")
	}
}

func TestAWSBedrockProvider_StreamIdleTimeout_Default(t *testing.T) {
	cfg := &config.Config{}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)
	model := config.ModelConfig{}
	got := p.StreamIdleTimeout(model)
	if got != 5*60*1000*1000*1000 { // 5 minutes
		t.Errorf("StreamIdleTimeout() = %v, want 5m", got)
	}
}

func TestAWSBedrockProvider_StreamIdleTimeout_Configured(t *testing.T) {
	cfg := &config.Config{
		AWSBedrock: config.AWSBedrockConfig{
			StreamTimeoutMs: 30000,
		},
	}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)
	model := config.ModelConfig{}
	got := p.StreamIdleTimeout(model)
	if got != 30*1000*1000*1000 { // 30 seconds
		t.Errorf("StreamIdleTimeout() = %v, want 30s", got)
	}
}

func TestAWSBedrockProvider_Execute(t *testing.T) {
	// Mock upstream server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}
		if r.Header.Get("OpenAI-Project") != "proj_123" {
			t.Errorf("OpenAI-Project = %q, want %q", r.Header.Get("OpenAI-Project"), "proj_123")
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want %q", r.Header.Get("Content-Type"), "application/json")
		}

		// Return a valid OpenAI response
		resp := types.ChatCompletionResponse{
			ID:    "cmpl-test",
			Model: "moonshotai.kimi-k2.5",
			Choices: []types.Choice{
				{
					Index: 0,
					Message: types.ChatMessage{
						Role:    "assistant",
						Content: json.RawMessage(`"Hello from Bedrock"`),
					},
					FinishReason: "stop",
				},
			},
			Usage: types.UsageInfo{
				PromptTokens:     10,
				CompletionTokens: 5,
				TotalTokens:      15,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		AWSBedrock: config.AWSBedrockConfig{
			BaseURL:   server.URL,
			APIKey:    "test-key",
			ProjectID: "proj_123",
		},
	}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)

	req := &core.NormalizedRequest{
		Model:    "moonshotai.kimi-k2.5",
		Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}},
	}
	model := config.ModelConfig{ModelID: "moonshotai.kimi-k2.5"}

	result, err := p.Execute(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result == nil {
		t.Fatal("Execute() returned nil result")
	}
	if result.ModelID != "moonshotai.kimi-k2.5" {
		t.Errorf("ModelID = %q, want %q", result.ModelID, "moonshotai.kimi-k2.5")
	}
	if len(result.Body) == 0 {
		t.Error("Body is empty")
	}
}

func TestAWSBedrockProvider_Stream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}
		if r.Header.Get("OpenAI-Project") != "proj_123" {
			t.Errorf("OpenAI-Project = %q, want %q", r.Header.Get("OpenAI-Project"), "proj_123")
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Errorf("Accept = %q, want %q", r.Header.Get("Accept"), "text/event-stream")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"Hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := &config.Config{
		AWSBedrock: config.AWSBedrockConfig{
			BaseURL:   server.URL,
			APIKey:    "test-key",
			ProjectID: "proj_123",
		},
	}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)

	req := &core.NormalizedRequest{
		Model:    "moonshotai.kimi-k2.5",
		Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}},
		Stream:   true,
	}
	model := config.ModelConfig{ModelID: "moonshotai.kimi-k2.5"}

	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = body.Close() }()

	buf := make([]byte, 1024)
	n, _ := body.Read(buf)
	if n == 0 {
		t.Error("Stream() returned empty body")
	}
}

func TestAWSBedrockProvider_Execute_NoProjectID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("OpenAI-Project") != "" {
			t.Errorf("OpenAI-Project = %q, want empty", r.Header.Get("OpenAI-Project"))
		}
		resp := types.ChatCompletionResponse{
			ID:    "cmpl-test",
			Model: "test-model",
			Choices: []types.Choice{
				{Index: 0, Message: types.ChatMessage{Role: "assistant", Content: json.RawMessage(`"ok"`)}, FinishReason: "stop"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := &config.Config{
		AWSBedrock: config.AWSBedrockConfig{
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
	}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewAWSBedrockProvider(atomic)

	req := &core.NormalizedRequest{
		Model:    "test-model",
		Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}},
	}
	model := config.ModelConfig{ModelID: "test-model"}

	_, err := p.Execute(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestIsBedrock(t *testing.T) {
	tests := []struct {
		model config.ModelConfig
		want  bool
	}{
		{config.ModelConfig{Provider: "aws-bedrock"}, true},
		{config.ModelConfig{Provider: "opencode-go"}, false},
		{config.ModelConfig{Provider: "opencode-zen"}, false},
		{config.ModelConfig{}, false},
	}
	for _, tt := range tests {
		if got := client.IsBedrock(tt.model); got != tt.want {
			t.Errorf("IsBedrock(%v) = %v, want %v", tt.model, got, tt.want)
		}
	}
}

func TestAWSBedrockProvider_NeedsOpenaiPath(t *testing.T) {
	p := NewAWSBedrockProvider(nil)
	tests := []struct {
		modelID string
		want    bool
	}{
		{"xai.grok-4.3", true},
		{"xai.grok-4.5", true},
		{"xai.grok-2-latest", true},
		{"openai.gpt-5.5", true},
		{"openai.gpt-5.4", true},
		{"openai.gpt-4.1", true},
		{"anthropic.claude-3-5-sonnet", false},
		{"moonshotai.kimi-k2.5", false},
		{"deepseek-v4-pro", false},
		{"zai.glm-5", false},
	}
	for _, tt := range tests {
		if got := p.needsOpenaiPath(tt.modelID); got != tt.want {
			t.Errorf("needsOpenaiPath(%q) = %v, want %v", tt.modelID, got, tt.want)
		}
	}
}

func TestAWSBedrockProvider_BedrockEndpoint(t *testing.T) {
	p := NewAWSBedrockProvider(nil)
	tests := []struct {
		name     string
		baseURL  string
		modelID  string
		wantPath string
	}{
		{
			name:     "xai model without /openai in base",
			baseURL:  "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
			modelID:  "xai.grok-4.3",
			wantPath: "https://bedrock-mantle.us-east-1.api.aws/openai/v1/chat/completions",
		},
		{
			name:     "xai model already has /openai in base",
			baseURL:  "https://bedrock-mantle.us-east-1.api.aws/openai/v1/chat/completions",
			modelID:  "xai.grok-4.3",
			wantPath: "https://bedrock-mantle.us-east-1.api.aws/openai/v1/chat/completions",
		},
		{
			name:     "zai model uses standard path",
			baseURL:  "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
			modelID:  "zai.glm-5",
			wantPath: "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
		},
		{
			name:     "non-xai model keeps original path",
			baseURL:  "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
			modelID:  "anthropic.claude-3-5-sonnet",
			wantPath: "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
		},
		{
			name:     "kimi model keeps original path",
			baseURL:  "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
			modelID:  "moonshotai.kimi-k2.5",
			wantPath: "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
		},
		{
			name:     "openai.gpt model uses openai path",
			baseURL:  "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
			modelID:  "openai.gpt-5.5",
			wantPath: "https://bedrock-mantle.us-east-1.api.aws/openai/v1/chat/completions",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &config.Config{
				AWSBedrock: config.AWSBedrockConfig{
					BaseURL: tt.baseURL,
				},
			}
			got := p.bedrockEndpoint(cfg, tt.modelID)
			if got != tt.wantPath {
				t.Errorf("bedrockEndpoint(%q) = %q, want %q", tt.modelID, got, tt.wantPath)
			}
		})
	}
}
