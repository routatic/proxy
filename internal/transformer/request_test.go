package transformer

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

// TestTransformRequestRoundTripReasoning verifies that a DeepSeek response with
// reasoning_content survives the full round-trip (OpenAI response → Anthropic
// response → Anthropic request → OpenAI request) so that on the next turn
// DeepSeed receives the reasoning_content it expects.
func TestTransformRequestRoundTripReasoning(t *testing.T) {
	// Step 1: Simulate a DeepSeek response with reasoning_content.
	deepSeekReasoning := "Let me think step by step"
	openaiResp := &types.ChatCompletionResponse{
		ID:     "resp_123",
		Object: "chat.completion",
		Model:  "deepseek-v4-flash",
		Choices: []types.Choice{{
			Index: 0,
			Message: types.ChatMessage{
				Role:             "assistant",
				Content:          contentText("The answer is 42"),
				ReasoningContent: &deepSeekReasoning,
			},
			FinishReason: "stop",
		}},
		Usage: types.UsageInfo{
			PromptTokens:     10,
			CompletionTokens: 20,
		},
	}

	// Step 2: Transform to Anthropic format (what Claude Code receives).
	rt := NewResponseTransformer()
	anthropicResp, err := rt.TransformResponse(openaiResp, "deepseek-v4-flash")
	if err != nil {
		t.Fatalf("TransformResponse error: %v", err)
	}

	// Verify Anthropic response has a thinking block.
	if len(anthropicResp.Content) != 2 {
		t.Fatalf("expected 2 content blocks, got %d", len(anthropicResp.Content))
	}
	if anthropicResp.Content[0].Type != "thinking" {
		t.Fatalf("expected first block to be thinking, got %s", anthropicResp.Content[0].Type)
	}
	if anthropicResp.Content[0].Thinking != deepSeekReasoning {
		t.Fatalf("thinking text = %q, want %q", anthropicResp.Content[0].Thinking, deepSeekReasoning)
	}

	// Step 3: Simulate Claude Code sending the conversation back on the next turn.
	// It includes the previous assistant message with the thinking block.
	anthropicReq := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"What is the answer?"`)},
			{
				Role:    "assistant",
				Content: mustJSONBytes(t, anthropicResp.Content),
			},
			{Role: "user", Content: json.RawMessage(`"Explain why?"`)},
		},
	}

	// Step 4: Transform back to OpenAI request.
	qt := NewRequestTransformer()
	openaiReq, err := qt.TransformRequest(anthropicReq, config.ModelConfig{ModelID: "deepseek-v4-flash"})
	if err != nil {
		t.Fatalf("TransformRequest error: %v", err)
	}

	// Find the assistant message.
	var assistantMsg *types.ChatMessage
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role == "assistant" {
			assistantMsg = &openaiReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("assistant message not found in transformed request")
	}

	// Step 5: Verify reasoning_content is preserved.
	if assistantMsg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil after round-trip")
	}
	if got, want := *assistantMsg.ReasoningContent, deepSeekReasoning; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}

	// Also verify the JSON serialization includes the field.
	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal error: %v", err)
	}
	if !bytes.Contains(body, []byte(`"reasoning_content"`)) {
		t.Fatalf("serialized request missing reasoning_content field: %s", body)
	}
}

func TestTransformRequestPreservesThinkingAsReasoningContent(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := true

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Need to look up the weather first","signature":"sig_123"},
					{"type":"tool_use","id":"toolu_123","name":"get_weather","input":{"city":"Kigali"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	msg := openaiReq.Messages[0]
	if got, want := msg.Role, "assistant"; got != want {
		t.Fatalf("Role = %q, want %q", got, want)
	}
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil")
	}
	if got, want := *msg.ReasoningContent, "Need to look up the weather first"; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	if got, want := len(msg.ToolCalls), 1; got != want {
		t.Fatalf("len(ToolCalls) = %d, want %d", got, want)
	}
	if got, want := msg.ToolCalls[0].ID, "toolu_123"; got != want {
		t.Fatalf("ToolCalls[0].ID = %q, want %q", got, want)
	}
	if got, want := msg.ToolCalls[0].Function.Name, "get_weather"; got != want {
		t.Fatalf("ToolCalls[0].Function.Name = %q, want %q", got, want)
	}
	if got, want := msg.ToolCalls[0].Function.Arguments, `{"city":"Kigali"}`; got != want {
		t.Fatalf("ToolCalls[0].Function.Arguments = %q, want %q", got, want)
	}
}

func TestTransformRequestIncludesStreamUsageOptions(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := true

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.StreamOptions == nil {
		t.Fatal("StreamOptions = nil, want include_usage enabled")
	}
	if !openaiReq.StreamOptions.IncludeUsage {
		t.Fatal("StreamOptions.IncludeUsage = false, want true")
	}
}

func TestTransformRequestOmitsStreamUsageOptionsWhenStreamingDisabled(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := false

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.StreamOptions != nil {
		t.Fatalf("StreamOptions = %v, want nil when streaming is disabled", openaiReq.StreamOptions)
	}
}

func TestTransformRequestIncludesEmptyReasoningContentForToolCalls(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_456","name":"search_docs","input":{"query":"figma api"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	msg := openaiReq.Messages[0]
	if msg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil placeholder")
	}
	if got, want := *msg.ReasoningContent, " "; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
}

func TestTransformRequestSerializesAssistantToolCallContent(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_456","name":"search_docs","input":{"query":"figma api"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	body, err := json.Marshal(openaiReq)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload struct {
		Messages []map[string]json.RawMessage `json:"messages"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if _, ok := payload.Messages[0]["content"]; !ok {
		t.Fatalf("serialized assistant tool-call message omitted content: %s", body)
	}
	if got, want := string(payload.Messages[0]["content"]), `""`; got != want {
		t.Fatalf("serialized content = %s, want %s", got, want)
	}
}

func TestTransformRequestAppliesReasoningEffortAndThinking(t *testing.T) {
	transformer := NewRequestTransformer()

	// When the conversation history already contains thinking blocks,
	// reasoning_effort and thinking should be applied.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"The answer is 42"}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort == nil {
		t.Fatal("ReasoningEffort = nil, want max")
	}
	if got, want := *openaiReq.ReasoningEffort, "max"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"enabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestDeepSeekHistoryGuardOverridesExplicitThinking(t *testing.T) {
	transformer := NewRequestTransformer()

	// DeepSeek rejects thinking mode when historical assistant messages lack
	// reasoning_content, so the safety guard must win over explicit config.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
			{Role: "assistant", Content: json.RawMessage(`"The answer is 42"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil (DeepSeek history guard)", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestFirstTurnEnablesThinkingWithReasoningEffort(t *testing.T) {
	transformer := NewRequestTransformer()

	// First turn (no assistant messages in history), only reasoning_effort
	// set in config → thinking should be enabled so DeepSeek can produce
	// reasoning content from the very first response.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort == nil {
		t.Fatal("ReasoningEffort = nil, want max on first turn")
	}
	if got, want := *openaiReq.ReasoningEffort, "max"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"enabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestRequestDisabledThinkingSkipsReasoningEffort(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Thinking:  json.RawMessage(`{"type":"disabled"}`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil when request disables thinking", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestThinkingDecisionMatrix(t *testing.T) {
	transformer := NewRequestTransformer()

	userOnly := []types.Message{
		{Role: "user", Content: json.RawMessage(`"solve this carefully"`)},
	}
	plainAssistantHistory := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		{Role: "user", Content: json.RawMessage(`"explain"`)},
	}
	thinkingHistory := []types.Message{
		{Role: "user", Content: json.RawMessage(`"hello"`)},
		{
			Role: "assistant",
			Content: json.RawMessage(`[
				{"type":"thinking","thinking":"Let me think..."},
				{"type":"text","text":"The answer is 42"}
			]`),
		},
		{Role: "user", Content: json.RawMessage(`"explain"`)},
	}

	tests := []struct {
		name       string
		messages   []types.Message
		thinking   json.RawMessage
		model      config.ModelConfig
		wantThink  string
		wantEffort *string
	}{
		{
			name:      "deepseek request thinking first turn maps budget to effort",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro"},
			wantThink: `{"type":"enabled","budget_tokens":4096}`,
			wantEffort: func() *string {
				s := "medium"
				return &s
			}(),
		},
		{
			name:      "deepseek plain assistant history guard beats request thinking",
			messages:  plainAssistantHistory,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":4096}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro"},
			wantThink: `{"type":"disabled"}`,
		},
		{
			name:      "deepseek request disabled beats thinking history and effort",
			messages:  thinkingHistory,
			thinking:  json.RawMessage(`{"type":"disabled"}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro", ReasoningEffort: "max"},
			wantThink: `{"type":"disabled"}`,
		},
		{
			name:      "openai reasoning model maps request budget without thinking field",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":2048}`),
			model:     config.ModelConfig{ModelID: "o3-mini"},
			wantThink: "",
			wantEffort: func() *string {
				s := "low"
				return &s
			}(),
		},
		{
			name:      "openai reasoning model uses explicit effort without thinking field",
			messages:  userOnly,
			model:     config.ModelConfig{ModelID: "o3-mini", ReasoningEffort: "max"},
			wantThink: "",
			wantEffort: func() *string {
				s := "max"
				return &s
			}(),
		},
		{
			name:      "standard model ignores request thinking",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":2048}`),
			model:     config.ModelConfig{ModelID: "qwen3.6-plus"},
			wantThink: "",
		},
		{
			name:      "request disabled overrides explicit model thinking",
			messages:  userOnly,
			thinking:  json.RawMessage(`{"type":"disabled"}`),
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro", Thinking: json.RawMessage(`{"type":"enabled"}`), ReasoningEffort: "max"},
			wantThink: `{"type":"disabled"}`,
		},
		{
			name:      "model disabled thinking skips explicit effort",
			messages:  userOnly,
			model:     config.ModelConfig{ModelID: "deepseek-v4-pro", Thinking: json.RawMessage(`{"type":"disabled"}`), ReasoningEffort: "max"},
			wantThink: `{"type":"disabled"}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &types.MessageRequest{
				Model:     "claude-test",
				MaxTokens: 256,
				Thinking:  tt.thinking,
				Messages:  tt.messages,
			}

			openaiReq, err := transformer.TransformRequest(req, tt.model)
			if err != nil {
				t.Fatalf("TransformRequest() error = %v", err)
			}

			if got := string(openaiReq.Thinking); got != tt.wantThink {
				t.Fatalf("Thinking = %s, want %s", got, tt.wantThink)
			}
			if tt.wantEffort == nil {
				if openaiReq.ReasoningEffort != nil {
					t.Fatalf("ReasoningEffort = %v, want nil", *openaiReq.ReasoningEffort)
				}
				return
			}
			if openaiReq.ReasoningEffort == nil {
				t.Fatalf("ReasoningEffort = nil, want %s", *tt.wantEffort)
			}
			if got, want := *openaiReq.ReasoningEffort, *tt.wantEffort; got != want {
				t.Fatalf("ReasoningEffort = %s, want %s", got, want)
			}
		})
	}
}

func TestTransformRequestFirstTurnReasoningEffortDefaultsToHigh(t *testing.T) {
	transformer := NewRequestTransformer()

	// First turn with thinking in history (from previous response round-trip).
	// No explicit ReasoningEffort → defaults to "high".
	// This test also covers the no-explicit-thinking-config path.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"The answer is 42"}
				]`),
			},
			{Role: "user", Content: json.RawMessage(`"explain"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort == nil {
		t.Fatal("ReasoningEffort = nil, want default high")
	}
	if got, want := *openaiReq.ReasoningEffort, "high"; got != want {
		t.Fatalf("ReasoningEffort = %q, want %q", got, want)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"enabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestSafetyGuardWithReasoningEffortOnly(t *testing.T) {
	transformer := NewRequestTransformer()

	// Only ReasoningEffort set (no explicit Thinking). History has an
	// assistant message without thinking blocks + it's a DeepSeek model
	// → safety guard fires, thinking is disabled to prevent the 400
	// "reasoning_content must be passed back" error.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			{Role: "user", Content: json.RawMessage(`"explain"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil (safety guard)", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s (safety guard)", got, want)
	}
}

func TestTransformRequestNoThinkingConfigNoHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	// No Thinking, no ReasoningEffort, no thinking history → nothing set.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil", *openaiReq.ReasoningEffort)
	}
	if openaiReq.Thinking != nil {
		t.Fatalf("Thinking = %s, want nil", string(openaiReq.Thinking))
	}
}

func TestTransformRequestPreservesSystemCacheControl(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"You are helpful","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if got, want := systemMsg.ContentText(), "You are helpful"; got != want {
		t.Fatalf("Messages[0].Content = %q, want %q", got, want)
	}
	if systemMsg.CacheControl == nil {
		t.Fatal("Messages[0].CacheControl = nil, want non-nil")
	}
	if got, want := systemMsg.CacheControl.Type, "ephemeral"; got != want {
		t.Fatalf("Messages[0].CacheControl.Type = %q, want %q", got, want)
	}
}

func TestTransformRequestSkipsCacheControlForKimiSystem(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if got, want := systemMsg.ContentText(), "system prompt"; got != want {
		t.Fatalf("Messages[0].Content = %q, want %q", got, want)
	}
	if systemMsg.CacheControl != nil {
		t.Fatalf("Kimi system message CacheControl = %v, want nil (guard should prevent assignment)", systemMsg.CacheControl)
	}
}

func TestTransformRequestStripsCacheControlForNonKimiNonDeepSeek(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"system prompt","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	// Use a non-Kimi, non-DeepSeek model (e.g. GLM) — cache_control should be
	// set by transformMessages then stripped by stripCacheControl().
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "glm-5"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if systemMsg.CacheControl != nil {
		t.Fatalf("Non-Kimi/non-DeepSeek system message CacheControl = %v, want nil", systemMsg.CacheControl)
	}
}

func TestTransformRequestStripsCacheControlForNonDeepSeek(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System: json.RawMessage(`[
			{"type":"text","text":"You are helpful","cache_control":{"type":"ephemeral"}}
		]`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	systemMsg := openaiReq.Messages[0]
	if systemMsg.CacheControl != nil {
		t.Fatalf("Messages[0].CacheControl = %v, want nil for non-DeepSeek model", systemMsg.CacheControl)
	}
}

func TestTransformRequestOmitsCacheControlWhenAbsent(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		System:    json.RawMessage(`"You are helpful"`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	systemMsg := openaiReq.Messages[0]
	if got, want := systemMsg.Role, "system"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if systemMsg.CacheControl != nil {
		t.Fatalf("Messages[0].CacheControl = %v, want nil", systemMsg.CacheControl)
	}
}

func TestTransformRequestPlacesToolResultsBeforeUserText(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_123","name":"create_file","input":{"name":"draft.fig"}}
				]`),
			},
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"tool_result","tool_use_id":"toolu_123","content":"created"},
					{"type":"text","text":"now continue"}
				]`),
			},
		},
	}

	// DeepSeek models preserve cache_control
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 3; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	if got, want := openaiReq.Messages[0].Role, "assistant"; got != want {
		t.Fatalf("Messages[0].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[1].Role, "tool"; got != want {
		t.Fatalf("Messages[1].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[1].ToolCallID, "toolu_123"; got != want {
		t.Fatalf("Messages[1].ToolCallID = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[2].Role, "user"; got != want {
		t.Fatalf("Messages[2].Role = %q, want %q", got, want)
	}
	if got, want := openaiReq.Messages[2].ContentText(), "now continue"; got != want {
		t.Fatalf("Messages[2].Content = %q, want %q", got, want)
	}
}

func TestTransformRequestSkipsReasoningEffortWhenThinkingDisabled(t *testing.T) {
	transformer := NewRequestTransformer()

	// When thinking is explicitly disabled in model config, reasoning_effort
	// must NOT be set — DeepSeek returns 400 if both are present.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"think carefully"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"The answer is 42"}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-pro",
		ReasoningEffort: "max",
		Thinking:        json.RawMessage(`{"type":"disabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("ReasoningEffort = %v, want nil (stripped because thinking is disabled)", *openaiReq.ReasoningEffort)
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
	}
}

func TestTransformRequestOmitsPlaceholderForDeepSeek(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_456","name":"search_docs","input":{"query":"figma api"}}
				]`),
			},
		},
	}

	// DeepSeek should NOT get a placeholder when there's no thinking history
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	msg := openaiReq.Messages[1] // assistant message
	if msg.ReasoningContent != nil {
		t.Fatalf("ReasoningContent = %q, want nil (DeepSeek without thinking history should not get placeholder)", *msg.ReasoningContent)
	}
}

func TestTransformRequestDeepSeekPlaceholderWithThinkingHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	// When thinking history exists, DeepSeek assistant messages with tool_calls
	// but no thinking block MUST get a placeholder reasoning_content, because
	// DeepSeek requires ALL assistant messages to have reasoning_content in
	// thinking mode.
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"think about this"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"I considered it"}
				]`),
			},
			{Role: "user", Content: json.RawMessage(`"now use a tool"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"tool_use","id":"toolu_789","name":"search","input":{"q":"test"}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-flash",
		ReasoningEffort: "high",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Find the second assistant message (tool_call only, no thinking)
	var toolCallAssistant *types.ChatMessage
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role == "assistant" && len(openaiReq.Messages[i].ToolCalls) > 0 {
			toolCallAssistant = &openaiReq.Messages[i]
			break
		}
	}
	if toolCallAssistant == nil {
		t.Fatal("no assistant message with tool_calls found")
	}
	if toolCallAssistant.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil placeholder for DeepSeek with thinking history")
	}
	if *toolCallAssistant.ReasoningContent != " " {
		t.Fatalf("ReasoningContent = %q, want placeholder space", *toolCallAssistant.ReasoningContent)
	}
}

// Regression test for an upstream 400 observed in production:
//
//	"The reasoning_content in the thinking mode must be passed back to the API."
//
// Claude Code can produce assistant turns that are text-only (no tool_use,
// no thinking block) inside a conversation where an earlier turn DID carry
// thinking. Examples: a follow-up that the model answers in plain text, or
// a post-/compact summary message. DeepSeek in thinking mode requires
// reasoning_content on EVERY assistant message, not just tool_use ones, so
// the proxy must add the placeholder for text-only turns too.
func TestTransformRequestDeepSeekPlaceholderForTextOnlyAssistant(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"think about this"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{"type":"thinking","thinking":"Let me think..."},
					{"type":"text","text":"My initial answer"}
				]`),
			},
			{Role: "user", Content: json.RawMessage(`"a follow-up question"`)},
			{
				// Text-only continuation. No thinking block (Claude Code
				// commonly drops thinking on simple follow-ups), no tool_use.
				Role:    "assistant",
				Content: json.RawMessage(`[{"type":"text","text":"A short reply"}]`),
			},
			{Role: "user", Content: json.RawMessage(`"and another"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID:         "deepseek-v4-flash",
		ReasoningEffort: "high",
		Thinking:        json.RawMessage(`{"type":"enabled"}`),
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Find the second assistant message (text-only follow-up).
	var textOnlyAssistant *types.ChatMessage
	seen := 0
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role != "assistant" {
			continue
		}
		seen++
		if seen == 2 {
			textOnlyAssistant = &openaiReq.Messages[i]
			break
		}
	}
	if textOnlyAssistant == nil {
		t.Fatal("expected two assistant messages in transformed request, found fewer")
	}
	if len(textOnlyAssistant.ToolCalls) != 0 {
		t.Fatalf("text-only assistant message unexpectedly had tool_calls: %+v", textOnlyAssistant.ToolCalls)
	}

	// The bug this test guards against: ReasoningContent was nil on this
	// message, causing DeepSeek to 400 the entire request. After the fix,
	// it's the single-space placeholder.
	if textOnlyAssistant.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil placeholder for DeepSeek text-only assistant in thinking-mode conversation")
	}
	if got, want := *textOnlyAssistant.ReasoningContent, " "; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
}

// Regression test for the production failure that motivated this PR.
//
// User configured oc-go-cc with a bare DeepSeek model config — no
// `thinking` field, no `reasoning_effort`. They ran Claude Code with
// `effortLevel: xhigh` set globally. Workflow:
//
//	turn 1: user asks question  →  proxy forwards to deepseek-v4-flash
//	turn 1 response: succeeds, upstream ran in DeepSeek's *default*
//	                 thinking mode (DeepSeek-v4 always defaults to
//	                 thinking mode unless explicitly disabled)
//	turn 2: user follows up  →  proxy receives request, sees no thinking
//	                             blocks in history (Claude Code didn't
//	                             round-trip the reasoning back), forwards
//	                             with `openaiReq.Thinking = nil` because
//	                             neither model config nor history asked for
//	                             thinking-mode handling
//	turn 2: upstream is STILL in thinking mode (default), demands
//	        reasoning_content on the prior assistant message which the
//	        proxy didn't add → 400.
//
// Fix: when the model is DeepSeek and there's no extant thinking history,
// explicitly send `thinking: disabled` so upstream switches off thinking
// mode and stops demanding reasoning_content.
func TestTransformRequestForceDisablesThinkingForDeepSeekWithoutHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "assistant", Content: json.RawMessage(`"hi"`)},
			{Role: "user", Content: json.RawMessage(`"do something"`)},
		},
	}

	// Bare DeepSeek config: no Thinking field, no ReasoningEffort.
	// Mirrors a typical user setup for the `fast` slot.
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Must explicitly disable thinking — leaving it nil lets DeepSeek's
	// default thinking-mode behavior take over and 400 on subsequent turns.
	if len(openaiReq.Thinking) == 0 {
		t.Fatal("openaiReq.Thinking is empty — must be set to {\"type\":\"disabled\"} for DeepSeek without thinking history")
	}
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("openaiReq.Thinking = %s, want %s", got, want)
	}
}

// Regression test: Claude Code emits tool_use blocks with the chain-of-
// thought attached directly via a `thinking` field, instead of as a
// separate thinking-typed block. Real shape observed:
//
//	{"type":"tool_use","id":"toolu_X","name":"...","input":{...},
//	 "thinking":"reasoning that led to the tool call"}
//
// HasThinkingBlocks must recognize this as thinking history, and
// transformAssistantMessage must extract the inline thinking string into
// reasoning_content. Without this:
//   - HasThinkingBlocks returns false → thinking mode not detected →
//     placeholder branch never fires → DeepSeek (which is in thinking mode
//     after the first reasoning response from the upstream-default mode)
//     returns 400 on the next request.
func TestTransformRequestExtractsThinkingFromToolUseBlock(t *testing.T) {
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"search for X"`)},
			{
				Role: "assistant",
				Content: json.RawMessage(`[
					{
						"type":"tool_use",
						"id":"toolu_thinking_inline",
						"name":"search",
						"input":{"q":"X"},
						"thinking":"I need to search the docs first"
					}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// (1) HasThinkingBlocks must detect the inline thinking on tool_use.
	if !HasThinkingBlocks(req.Messages) {
		t.Fatal("HasThinkingBlocks = false, want true (tool_use block has inline thinking)")
	}

	// (2) The transformed assistant message must carry the thinking content
	//     as reasoning_content so DeepSeek's validator is satisfied.
	var assistantMsg *types.ChatMessage
	for i := range openaiReq.Messages {
		if openaiReq.Messages[i].Role == "assistant" {
			assistantMsg = &openaiReq.Messages[i]
			break
		}
	}
	if assistantMsg == nil {
		t.Fatal("no assistant message in transformed request")
	}
	if assistantMsg.ReasoningContent == nil {
		t.Fatal("ReasoningContent = nil, want non-nil (thinking on tool_use must round-trip)")
	}
	if got, want := *assistantMsg.ReasoningContent, "I need to search the docs first"; got != want {
		t.Fatalf("ReasoningContent = %q, want %q", got, want)
	}
	// (3) tool_calls must still be present — extracting thinking shouldn't
	//     drop the tool invocation.
	if len(assistantMsg.ToolCalls) != 1 {
		t.Fatalf("ToolCalls = %d, want 1", len(assistantMsg.ToolCalls))
	}
	if got, want := assistantMsg.ToolCalls[0].Function.Name, "search"; got != want {
		t.Fatalf("ToolCalls[0].Name = %q, want %q", got, want)
	}
}

func mustJSONBytes(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return json.RawMessage(b)
}

func TestTransformRequestVisionModelPassesImageContent(t *testing.T) {
	transformer := NewRequestTransformer()

	// User message with both text and an image
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"What's in this image?"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
				]`),
			},
		},
	}

	// Vision-capable model: images should be passed as image_url content parts
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "kimi-k2.6",
		Vision:  true,
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	body, err := json.Marshal(openaiReq.Messages[0].Content)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !bytes.Contains(body, []byte(`"type":"image_url"`)) {
		t.Fatalf("vision model content missing image_url: %s", body)
	}
	if !bytes.Contains(body, []byte(`"data:image/png;base64,iVBORw0KGgo="`)) {
		t.Fatalf("vision model content missing image data URL: %s", body)
	}
	if !bytes.Contains(body, []byte(`"What's in this image?"`)) {
		t.Fatalf("vision model content missing text: %s", body)
	}
}

func TestTransformRequestNonVisionModelStripsImages(t *testing.T) {
	transformer := NewRequestTransformer()

	// User message with both text and an image
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"text","text":"What's in this image?"},
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
				]`),
			},
		},
	}

	// Non-vision model: images should be replaced with [Image] placeholder
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-pro",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	content := openaiReq.Messages[0].ContentText()
	if !strings.Contains(content, "[Image]") {
		t.Fatalf("non-vision model content missing [Image] placeholder: %q", content)
	}
	if !strings.Contains(content, "What's in this image?") {
		t.Fatalf("non-vision model content missing original text: %q", content)
	}
	// Verify no image_url was sent
	body, err := json.Marshal(openaiReq.Messages[0].Content)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if bytes.Contains(body, []byte(`"type":"image_url"`)) {
		t.Fatalf("non-vision model should not contain image_url: %s", body)
	}
}

func TestTransformRequestNonVisionModelImageOnly(t *testing.T) {
	transformer := NewRequestTransformer()

	// User message with only an image, no text
	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Messages: []types.Message{
			{
				Role: "user",
				Content: json.RawMessage(`[
					{"type":"image","source":{"type":"base64","media_type":"image/png","data":"iVBORw0KGgo="}}
				]`),
			},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "deepseek-v4-flash",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	content := openaiReq.Messages[0].ContentText()
	if got, want := content, "[Image]"; got != want {
		t.Fatalf("ContentText() = %q, want %q", got, want)
	}
}

func TestTransformRequestDeepSeekRewritesSystemRoleToUserMessage(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"task tools haven't been used recently"}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have 2 messages: user + rewritten system
	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	// The system message should be rewritten to a user message with <system-reminder> tags
	msg := openaiReq.Messages[1]
	if msg.Role != "user" {
		t.Fatalf("rewritten message role = %q, want %q", msg.Role, "user")
	}
	wantContent := "<system-reminder>\ntask tools haven't been used recently\n</system-reminder>"
	if got := msg.ContentText(); got != wantContent {
		t.Fatalf("rewritten content = %q, want %q", got, wantContent)
	}
}

func TestTransformRequestNonDeepSeekIgnoresSystemRoleMessages(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "qwen3.6-plus",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"a system reminder"}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "qwen3.6-plus"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Non-DeepSeek models should pass system messages to transformMessage for normal handling.
	// transformMessage's default case will produce a message with the original role.
	lastMsg := openaiReq.Messages[len(openaiReq.Messages)-1]
	if lastMsg.Role != "system" {
		t.Fatalf("non-DeepSeek system message role = %q, want %q (should pass through unchanged)", lastMsg.Role, "system")
	}
}

func TestTransformRequestDeepSeekDeduplicatesSystemMessage(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		System:    json.RawMessage(`"You are Claude. task tools haven't been used recently"`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			// This system message is a substring of the top-level system prompt — should be skipped
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"task tools haven't been used recently"}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have 2 messages: system (from top-level) + user
	// The duplicate system message should be dropped
	if got, want := len(openaiReq.Messages), 2; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	// First message should be the top-level system prompt
	if openaiReq.Messages[0].Role != "system" {
		t.Fatalf("first message role = %q, want %q", openaiReq.Messages[0].Role, "system")
	}
	// Second message should be the user message
	if openaiReq.Messages[1].Role != "user" {
		t.Fatalf("second message role = %q, want %q", openaiReq.Messages[1].Role, "user")
	}
	if got, want := openaiReq.Messages[1].ContentText(), "hello"; got != want {
		t.Fatalf("second message content = %q, want %q", got, want)
	}
}

func TestTransformRequestDeepSeekSkipsEmptySystemMessage(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			// Empty text blocks — should be skipped
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":""}]`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have exactly 1 message (just the user message)
	if got, want := len(openaiReq.Messages), 1; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}
}

func TestTransformRequestDeepSeekRewritesMultipleSystemMessages(t *testing.T) {
	t.Parallel()
	transformer := NewRequestTransformer()

	req := &types.MessageRequest{
		Model:     "deepseek-v4-pro",
		MaxTokens: 256,
		Messages: []types.Message{
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"first reminder"}]`)},
			{Role: "user", Content: json.RawMessage(`"hello"`)},
			{Role: "system", Content: json.RawMessage(`[{"type":"text","text":"second reminder"}]`)},
			{Role: "assistant", Content: json.RawMessage(`"hi"`)},
		},
	}

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "deepseek-v4-pro"})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	// Should have 4 messages: 2 rewritten system + user + assistant
	if got, want := len(openaiReq.Messages), 4; got != want {
		t.Fatalf("len(Messages) = %d, want %d", got, want)
	}

	// Both system messages should be rewritten to user with <system-reminder> tags
	for i, idx := range []int{0, 2} {
		msg := openaiReq.Messages[idx]
		if msg.Role != "user" {
			t.Fatalf("Messages[%d] role = %q, want %q", idx, msg.Role, "user")
		}
		wantSuffix := []string{"first reminder", "second reminder"}[i]
		got := msg.ContentText()
		if !strings.Contains(got, wantSuffix) {
			t.Fatalf("Messages[%d] content = %q, want it to contain %q", idx, got, wantSuffix)
		}
		if !strings.HasPrefix(got, "<system-reminder>") {
			t.Fatalf("Messages[%d] content = %q, want <system-reminder> prefix", idx, got)
		}
	}

	// The user and assistant messages should be in their original positions
	if openaiReq.Messages[1].Role != "user" || openaiReq.Messages[1].ContentText() != "hello" {
		t.Fatalf("Messages[1] expected user 'hello', got %q %q", openaiReq.Messages[1].Role, openaiReq.Messages[1].ContentText())
	}
	if openaiReq.Messages[3].Role != "assistant" || openaiReq.Messages[3].ContentText() != "hi" {
		t.Fatalf("Messages[3] expected assistant 'hi', got %q %q", openaiReq.Messages[3].Role, openaiReq.Messages[3].ContentText())
	}
}

func TestTransformRequestStandardModelIgnoresThinkingAndEffort(t *testing.T) {
	transformer := NewRequestTransformer()
	stream := true

	req := &types.MessageRequest{
		Model:     "claude-test",
		MaxTokens: 256,
		Stream:    &stream,
		Thinking:  json.RawMessage(`{"type":"enabled","budget_tokens":2048}`),
		Messages: []types.Message{
			{Role: "user", Content: json.RawMessage(`"hello"`)},
		},
	}

	// Standard model like qwen3.6-plus without explicit config
	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{
		ModelID: "qwen3.6-plus",
	})
	if err != nil {
		t.Fatalf("TransformRequest() error = %v", err)
	}

	if openaiReq.ReasoningEffort != nil {
		t.Fatalf("expected ReasoningEffort to be nil for standard model, got %v", *openaiReq.ReasoningEffort)
	}
	if openaiReq.Thinking != nil {
		t.Fatalf("expected Thinking to be nil for standard model, got %s", string(openaiReq.Thinking))
	}
}
