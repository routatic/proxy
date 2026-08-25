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

func TestOpenCodeGoProvider_WireFormat_Override(t *testing.T) {
	p := NewOpenCodeGoProvider(config.NewAtomicConfig(&config.Config{}, ""))

	tests := []struct {
		name       string
		modelID    string
		wireFormat string
		want       core.WireFormat
	}{
		// Override takes precedence over the built-in classification.
		{"responses override", "muse-spark-1.2-contributor", "responses", core.WireFormatOpenAIResponses},
		{"responses override on chat model", "deepseek-v4-pro", "responses", core.WireFormatOpenAIResponses},
		{"anthropic override on chat model", "deepseek-v4-pro", "anthropic", core.WireFormatAnthropic},
		{"messages alias", "deepseek-v4-pro", "messages", core.WireFormatAnthropic},
		{"openai override on anthropic-native model", "minimax-m3", "openai", core.WireFormatOpenAIChat},
		{"chat alias", "minimax-m3", "chat", core.WireFormatOpenAIChat},
		{"chat_completions alias", "minimax-m3", "chat_completions", core.WireFormatOpenAIChat},

		// No override — fall back to classification.
		{"empty falls back to chat", "deepseek-v4-pro", "", core.WireFormatOpenAIChat},
		{"empty falls back to anthropic", "minimax-m3", "", core.WireFormatAnthropic},
		{"auto falls back to chat", "deepseek-v4-pro", "auto", core.WireFormatOpenAIChat},
		{"auto falls back to anthropic", "qwen3.7-max", "auto", core.WireFormatAnthropic},
		{"unrecognised falls back", "minimax-m3", "not-a-format", core.WireFormatAnthropic},

		// The Go provider has no Gemini path, so "gemini" must not be honoured —
		// otherwise Execute/Stream would silently send a chat body to BaseURL.
		{"gemini is ignored", "deepseek-v4-pro", "gemini", core.WireFormatOpenAIChat},
		{"gemini is ignored on anthropic-native", "minimax-m3", "gemini", core.WireFormatAnthropic},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := config.ModelConfig{ModelID: tt.modelID, WireFormat: tt.wireFormat}
			if got := p.WireFormat(model); got != tt.want {
				t.Errorf("WireFormat(%q, wire_format=%q) = %v, want %v",
					tt.modelID, tt.wireFormat, got, tt.want)
			}
		})
	}
}

// TestOpenCodeGoProvider_WireFormatOverride_MatchesEndpoint pins the contract
// that broke when Execute/Stream resolved the override but the streaming
// handler did not: the wire format reported by WireFormat (which the handler
// uses to pick an SSE parser) must match the endpoint the request is actually
// sent to. A mismatch means Responses SSE gets parsed as Chat Completions.
func TestOpenCodeGoProvider_WireFormatOverride_MatchesEndpoint(t *testing.T) {
	var chatHit, responsesHit bool

	chatServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		chatHit = true
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer chatServer.Close()

	responsesServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responsesHit = true
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer responsesServer.Close()

	cfg := &config.Config{
		APIKey: "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{
			BaseURL:          chatServer.URL,
			ResponsesBaseURL: responsesServer.URL,
		},
	}
	p := NewOpenCodeGoProvider(config.NewAtomicConfig(cfg, ""))

	// deepseek-v4-pro classifies as Chat Completions, so only the override can
	// route it to the Responses endpoint.
	model := config.ModelConfig{ModelID: "deepseek-v4-pro", WireFormat: "responses"}
	req := &core.NormalizedRequest{
		Model:    model.ModelID,
		Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
	}

	if got := p.WireFormat(model); got != core.WireFormatOpenAIResponses {
		t.Fatalf("WireFormat() = %v, want OpenAIResponses", got)
	}

	body, err := p.Stream(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Stream() error = %v", err)
	}
	_ = body.Close()

	if !responsesHit {
		t.Error("Stream() did not reach the Responses endpoint")
	}
	if chatHit {
		t.Error("Stream() reached the Chat Completions endpoint despite wire_format=responses")
	}
}

func TestOpenCodeGoProvider_Responses_MissingBaseURL(t *testing.T) {
	cfg := &config.Config{APIKey: "test-key", OpenCodeGo: config.OpenCodeGoConfig{BaseURL: "http://127.0.0.1:1"}}
	p := NewOpenCodeGoProvider(config.NewAtomicConfig(cfg, ""))

	model := config.ModelConfig{ModelID: "deepseek-v4-pro", WireFormat: "responses"}
	req := &core.NormalizedRequest{
		Model:    model.ModelID,
		Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
	}

	// An unset responses_base_url must name the missing config key rather than
	// surfacing an opaque `unsupported protocol scheme ""` from net/http.
	for _, tc := range []struct {
		name string
		call func() error
	}{
		{"Execute", func() error { _, err := p.Execute(context.Background(), req, model); return err }},
		{"Stream", func() error { _, err := p.Stream(context.Background(), req, model); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.call()
			if err == nil {
				t.Fatal("expected an error when responses_base_url is unset")
			}
			if !strings.Contains(err.Error(), "responses_base_url") {
				t.Errorf("error = %q, want it to mention responses_base_url", err)
			}
		})
	}
}

func TestOpenCodeGoProvider_ExecuteResponses_Override(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ResponsesResponse{
			ID: "resp-test", Object: "response", Created: 1, Model: "muse-spark-1.2-contributor",
			Output: []types.ResponsesOutput{{
				Type: "message", Role: "assistant",
				Content: []types.ResponsesContent{{Type: "output_text", Text: "hi"}},
			}},
			Usage: types.ResponsesUsage{InputTokens: 1, OutputTokens: 1},
		})
	}))
	defer server.Close()

	cfg := &config.Config{
		APIKey:     "test-key",
		OpenCodeGo: config.OpenCodeGoConfig{BaseURL: "http://127.0.0.1:1", ResponsesBaseURL: server.URL},
	}
	p := NewOpenCodeGoProvider(config.NewAtomicConfig(cfg, ""))

	model := config.ModelConfig{ModelID: "muse-spark-1.2-contributor", WireFormat: "responses"}
	req := &core.NormalizedRequest{
		Model:    model.ModelID,
		Messages: []core.NormalizedMessage{{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "Hi"}}}},
	}

	result, err := p.Execute(context.Background(), req, model)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !strings.Contains(string(result.Body), "hi") {
		t.Errorf("Execute() body = %s, want it to contain the assistant text", result.Body)
	}
}
