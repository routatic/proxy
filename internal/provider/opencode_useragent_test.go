package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

func assertOpencodeUserAgentServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ua := r.Header.Get("User-Agent")
		if !strings.HasPrefix(ua, "opencode/") {
			t.Errorf("User-Agent = %q, want prefix %q", ua, "opencode/")
		}
		handler(w, r)
	}))
}

func chatCompletionServer(t *testing.T) *httptest.Server {
	t.Helper()
	return assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ChatCompletionResponse{
			ID:    "cmpl-test",
			Model: "test-model",
			Choices: []types.Choice{
				{Index: 0, Message: types.ChatMessage{Role: "assistant", Content: json.RawMessage(`"hi"`)}, FinishReason: "stop"},
			},
			Usage: types.UsageInfo{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
		})
	})
}

func sseServer(t *testing.T) *httptest.Server {
	t.Helper()
	return assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	})
}

func TestOpenCodeZenProvider_Execute_OpencodeUserAgent(t *testing.T) {
	server := chatCompletionServer(t)
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeZen: config.OpenCodeZenConfig{BaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeZenProvider(atomic)

	req := &core.NormalizedRequest{Model: "deepseek-v4-flash-free", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}}
	model := config.ModelConfig{ModelID: "deepseek-v4-flash-free"}
	if got := p.WireFormat(model); got != core.WireFormatOpenAIChat {
		t.Fatalf("WireFormat(%q) = %v, want OpenAIChat", model.ModelID, got)
	}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCodeZenProvider_Stream_OpencodeUserAgent(t *testing.T) {
	server := sseServer(t)
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeZen: config.OpenCodeZenConfig{BaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeZenProvider(atomic)

	req := &core.NormalizedRequest{Model: "deepseek-v4-flash-free", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}, Stream: true}
	model := config.ModelConfig{ModelID: "deepseek-v4-flash-free"}

	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = body.Close() }()

	buf := make([]byte, 1024)
	if n, _ := body.Read(buf); n == 0 {
		t.Error("Stream() returned empty body")
	}
}

func TestOpenCodeZenProvider_ExecuteAnthropic_OpencodeUserAgent(t *testing.T) {
	server := assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("Authorization = %q, want %q", r.Header.Get("Authorization"), "Bearer test-key")
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q, want %q", r.Header.Get("x-api-key"), "test-key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg-test","content":[{"type":"text","text":"hi"}]}`))
	})
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeZen: config.OpenCodeZenConfig{BaseURL: server.URL, AnthropicBaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeZenProvider(atomic)

	req := &core.NormalizedRequest{Model: "claude-sonnet-4.5", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}}
	model := config.ModelConfig{ModelID: "claude-sonnet-4.5"}
	if got := p.WireFormat(model); got != core.WireFormatAnthropic {
		t.Fatalf("WireFormat(%q) = %v, want Anthropic", model.ModelID, got)
	}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCodeZenProvider_StreamAnthropic_OpencodeUserAgent(t *testing.T) {
	server := assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\"}}\n\n"))
	})
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeZen: config.OpenCodeZenConfig{BaseURL: server.URL, AnthropicBaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeZenProvider(atomic)

	req := &core.NormalizedRequest{Model: "claude-sonnet-4.5", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}, Stream: true}
	model := config.ModelConfig{ModelID: "claude-sonnet-4.5"}

	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = body.Close() }()

	buf := make([]byte, 1024)
	if n, _ := body.Read(buf); n == 0 {
		t.Error("Stream() returned empty body")
	}
}

func TestOpenCodeZenProvider_ExecuteResponses_OpencodeUserAgent(t *testing.T) {
	server := assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ResponsesResponse{
			ID: "resp-test", Object: "response", Created: 1, Model: "gpt-5.4",
			Output: []types.ResponsesOutput{{Type: "message", Role: "assistant", Content: []types.ResponsesContent{{Type: "output_text", Text: "hi"}}}},
			Usage:  types.ResponsesUsage{InputTokens: 1, OutputTokens: 1},
		})
	})
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeZen: config.OpenCodeZenConfig{BaseURL: server.URL, ResponsesBaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeZenProvider(atomic)

	req := &core.NormalizedRequest{Model: "gpt-5.4", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}}
	model := config.ModelConfig{ModelID: "gpt-5.4"}
	if got := p.WireFormat(model); got != core.WireFormatOpenAIResponses {
		t.Fatalf("WireFormat(%q) = %v, want OpenAIResponses", model.ModelID, got)
	}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCodeGoProvider_Execute_OpencodeUserAgent(t *testing.T) {
	server := chatCompletionServer(t)
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeGoProvider(atomic)

	req := &core.NormalizedRequest{Model: "deepseek-v4-pro", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}}
	model := config.ModelConfig{ModelID: "deepseek-v4-pro"}
	if got := p.WireFormat(model); got != core.WireFormatOpenAIChat {
		t.Fatalf("WireFormat(%q) = %v, want OpenAIChat", model.ModelID, got)
	}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCodeGoProvider_Stream_OpencodeUserAgent(t *testing.T) {
	server := sseServer(t)
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeGoProvider(atomic)

	req := &core.NormalizedRequest{Model: "deepseek-v4-pro", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}, Stream: true}
	model := config.ModelConfig{ModelID: "deepseek-v4-pro"}

	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = body.Close() }()

	buf := make([]byte, 1024)
	if n, _ := body.Read(buf); n == 0 {
		t.Error("Stream() returned empty body")
	}
}

func TestOpenCodeGoProvider_ExecuteAnthropic_OpencodeUserAgent(t *testing.T) {
	server := assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("x-api-key = %q, want %q", r.Header.Get("x-api-key"), "test-key")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg-test","content":[{"type":"text","text":"hi"}]}`))
	})
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: server.URL, AnthropicBaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeGoProvider(atomic)

	req := &core.NormalizedRequest{Model: "qwen3.5-plus", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}}
	model := config.ModelConfig{ModelID: "qwen3.5-plus"}
	if got := p.WireFormat(model); got != core.WireFormatAnthropic {
		t.Fatalf("WireFormat(%q) = %v, want Anthropic", model.ModelID, got)
	}
	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
}

func TestOpenCodeGoProvider_StreamAnthropic_OpencodeUserAgent(t *testing.T) {
	server := assertOpencodeUserAgentServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: message_start\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"message_start\",\"message\":{\"id\":\"msg_1\"}}\n\n"))
	})
	defer server.Close()

	cfg := &config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: server.URL, AnthropicBaseURL: server.URL}}
	atomic := config.NewAtomicConfig(cfg, "")
	p := NewOpenCodeGoProvider(atomic)

	req := &core.NormalizedRequest{Model: "qwen3.5-plus", Messages: []core.NormalizedMessage{{Role: "user", Content: "Hi"}}, Stream: true}
	model := config.ModelConfig{ModelID: "qwen3.5-plus"}

	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	defer func() { _ = body.Close() }()

	buf := make([]byte, 1024)
	if n, _ := body.Read(buf); n == 0 {
		t.Error("Stream() returned empty body")
	}
}
