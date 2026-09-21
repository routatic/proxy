package provider

import (
	"context"
	"encoding/json"
	"io"
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

// TestOpenCodeGoProvider_ExecuteResponses_ToolRoundTrip sends a 3-message tool
// round-trip (user text -> assistant tool_use -> user tool_result) and asserts
// the request body the server receives contains a function_call item followed
// by a function_call_output item for the same call id, and no item that is
// neither typed nor role+content (the shape that 400s upstream).
func TestOpenCodeGoProvider_ExecuteResponses_ToolRoundTrip(t *testing.T) {
	var gotBody []byte
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(types.ResponsesResponse{
			ID: "resp-tool", Object: "response", Created: 1, Model: "muse-spark-1.2-contributor",
			Output: []types.ResponsesOutput{{
				Type: "message", Role: "assistant",
				Content: []types.ResponsesContent{{Type: "output_text", Text: "done"}},
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
		Model: model.ModelID,
		Messages: []core.NormalizedMessage{
			{Role: "user", Blocks: []core.NormalizedContentBlock{{Type: "text", Text: "What's the weather?"}}},
			{Role: "assistant", Blocks: []core.NormalizedContentBlock{
				{Type: "text", Text: "Let me check."},
				{Type: "tool_use", ID: "t1", Name: "get_weather", Input: json.RawMessage(`{"location":"SF"}`)},
			}},
			{Role: "user", Blocks: []core.NormalizedContentBlock{
				{Type: "tool_result", ToolUseID: "t1", Content: json.RawMessage(`"72F"`)},
			}},
		},
	}

	if _, err := p.Execute(context.Background(), req, model); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	var rr types.ResponsesRequest
	if err := json.Unmarshal(gotBody, &rr); err != nil {
		t.Fatalf("request body is not a valid ResponsesRequest: %v\nbody: %s", err, gotBody)
	}

	callIdx, outputIdx := -1, -1
	for i, item := range rr.Input {
		if item.Type == "" && (item.Role == "" || len(item.Content) == 0) {
			t.Errorf("input[%d] is neither typed nor role+content: %+v", i, item)
		}
		switch item.Type {
		case "function_call":
			callIdx = i
			if item.CallID != "t1" || item.Name != "get_weather" {
				t.Errorf("function_call item = %+v, want call_id t1 / name get_weather", item)
			}
		case "function_call_output":
			outputIdx = i
			if item.CallID != "t1" {
				t.Errorf("function_call_output item = %+v, want call_id t1", item)
			}
		}
	}

	if callIdx == -1 {
		t.Error("no function_call item in request body")
	}
	if outputIdx == -1 {
		t.Error("no function_call_output item in request body")
	}
	if callIdx != -1 && outputIdx != -1 && callIdx > outputIdx {
		t.Errorf("function_call at %d must precede function_call_output at %d", callIdx, outputIdx)
	}
}
