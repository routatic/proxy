package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

const testSessionID = "session-123e4567-e89b-12d3-a456-426614174000"

// sessionAssertServer returns a server that asserts the x-opencode-session
// header on every upstream request, then delegates to handler.
func sessionAssertServer(t *testing.T, want string, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(core.OpenCodeSessionHeader); got != want {
			t.Errorf("%s = %q, want %q", core.OpenCodeSessionHeader, got, want)
		}
		handler(w, r)
	}))
}

// sessionAbsentServer returns a server that asserts the header is NOT present.
func sessionAbsentServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(core.OpenCodeSessionHeader); got != "" {
			t.Errorf("%s = %q, want absent", core.OpenCodeSessionHeader, got)
		}
		handler(w, r)
	}))
}

func chatCompletionJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types.ChatCompletionResponse{
		ID:    "cmpl-test",
		Model: "test-model",
		Choices: []types.Choice{
			{Index: 0, Message: types.ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)}, FinishReason: "stop"},
		},
		Usage: types.UsageInfo{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	})
}

func anthropicJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"id":"msg-test","content":[{"type":"text","text":"hi"}]}`))
}

func responsesJSON(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(types.ResponsesResponse{
		ID: "resp-test", Object: "response", Created: 1, Model: "muse-spark-1.2-contributor",
		Output: []types.ResponsesOutput{{
			Type: "message", Role: "assistant",
			Content: []types.ResponsesContent{{Type: "output_text", Text: "hi"}},
		}},
		Usage: types.ResponsesUsage{InputTokens: 1, OutputTokens: 1},
	})
}

func chatCompletionSSE(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

func anthropicSSE(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("event: message_start\n"))
	_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\"}}\n\n"))
}

func responsesSSE(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = w.Write([]byte("data: [DONE]\n\n"))
}

// TestOpenCodeGoProvider_SetsOpenCodeSessionHeader pins that every OpenCode Go
// request builder (chat, anthropic, and responses; execute and stream) forwards
// the session ID from the context verbatim as x-opencode-session.
func TestOpenCodeGoProvider_SetsOpenCodeSessionHeader(t *testing.T) {
	testCases := []struct {
		name    string
		model   config.ModelConfig
		stream  bool
		cfg     func(serverURL string) config.Config
		respond func(w http.ResponseWriter, r *http.Request)
	}{
		{
			name:  "chat completions execute",
			model: config.ModelConfig{ModelID: "deepseek-v4-pro"},
			cfg: func(u string) config.Config {
				return config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: u}}
			},
			respond: chatCompletionJSON,
		},
		{
			name:   "chat completions stream",
			model:  config.ModelConfig{ModelID: "deepseek-v4-pro"},
			stream: true,
			cfg: func(u string) config.Config {
				return config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: u}}
			},
			respond: chatCompletionSSE,
		},
		{
			name:  "anthropic execute",
			model: config.ModelConfig{ModelID: "qwen3.5-plus"},
			cfg: func(u string) config.Config {
				return config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: u, AnthropicBaseURL: u}}
			},
			respond: anthropicJSON,
		},
		{
			name:   "anthropic stream",
			model:  config.ModelConfig{ModelID: "qwen3.5-plus"},
			stream: true,
			cfg: func(u string) config.Config {
				return config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: u, AnthropicBaseURL: u}}
			},
			respond: anthropicSSE,
		},
		{
			name:  "responses execute",
			model: config.ModelConfig{ModelID: "muse-spark-1.2-contributor", WireFormat: "responses"},
			cfg: func(u string) config.Config {
				return config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: "http://127.0.0.1:1", ResponsesBaseURL: u}}
			},
			respond: responsesJSON,
		},
		{
			name:   "responses stream",
			model:  config.ModelConfig{ModelID: "muse-spark-1.2-contributor", WireFormat: "responses"},
			stream: true,
			cfg: func(u string) config.Config {
				return config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: "http://127.0.0.1:1", ResponsesBaseURL: u}}
			},
			respond: responsesSSE,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			server := sessionAssertServer(t, testSessionID, tc.respond)
			defer server.Close()

			cfg := tc.cfg(server.URL)
			p := NewOpenCodeGoProvider(config.NewAtomicConfig(&cfg, ""))

			req := &core.NormalizedRequest{
				Model:    tc.model.ModelID,
				Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
				Stream:   tc.stream,
			}
			ctx := core.WithSessionID(context.Background(), testSessionID)

			if tc.stream {
				body, err := p.Stream(ctx, req, tc.model)
				if err != nil {
					t.Fatalf("Stream() error = %v", err)
				}
				defer func() { _ = body.Close() }()
				buf := make([]byte, 1024)
				if n, _ := body.Read(buf); n == 0 {
					t.Error("Stream() returned empty body")
				}
			} else {
				if _, err := p.Execute(ctx, req, tc.model); err != nil {
					t.Fatalf("Execute() error = %v", err)
				}
			}
		})
	}
}

// TestOpenCodeGoProvider_NoSessionID_OmitsHeader pins that a context without a
// session ID produces no header (the handler fills the fallback UUID before it
// reaches the provider, so this only happens on direct provider use).
func TestOpenCodeGoProvider_NoSessionID_OmitsHeader(t *testing.T) {
	server := sessionAbsentServer(t, chatCompletionJSON)
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: server.URL}}
	p := NewOpenCodeGoProvider(config.NewAtomicConfig(cfg, ""))

	req := &core.NormalizedRequest{
		Model:    "deepseek-v4-pro",
		Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
	}
	model := config.ModelConfig{ModelID: "deepseek-v4-pro"}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

// TestOpenCodeZenProvider_DoesNotReceiveSessionHeader pins the scope boundary:
// x-opencode-session is for OpenCode Go only, so a session ID in the context
// must not leak onto OpenCode Zen requests.
func TestOpenCodeZenProvider_DoesNotReceiveSessionHeader(t *testing.T) {
	server := sessionAbsentServer(t, chatCompletionJSON)
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeZen: config.OpenCodeZenConfig{BaseURL: server.URL}}
	p := NewOpenCodeZenProvider(config.NewAtomicConfig(cfg, ""))

	req := &core.NormalizedRequest{
		Model:    "deepseek-v4-flash-free",
		Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
	}
	model := config.ModelConfig{ModelID: "deepseek-v4-flash-free"}
	if _, err := p.Execute(core.WithSessionID(context.Background(), testSessionID), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

// TestAWSBedrockProvider_DoesNotReceiveSessionHeader pins that the session ID
// does not leak onto Bedrock requests.
func TestAWSBedrockProvider_DoesNotReceiveSessionHeader(t *testing.T) {
	server := sessionAbsentServer(t, chatCompletionJSON)
	defer server.Close()

	cfg := &config.Config{
		AWSBedrock: config.AWSBedrockConfig{
			BaseURL: server.URL,
			APIKey:  "test-key",
		},
	}
	p := NewAWSBedrockProvider(config.NewAtomicConfig(cfg, ""))

	req := &core.NormalizedRequest{
		Model:    "moonshotai.kimi-k2.5",
		Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
	}
	model := config.ModelConfig{ModelID: "moonshotai.kimi-k2.5"}
	if _, err := p.Execute(core.WithSessionID(context.Background(), testSessionID), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}
