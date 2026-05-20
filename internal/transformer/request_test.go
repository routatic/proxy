package transformer

import (
	"bytes"
	"encoding/json"
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
				Content:          "The answer is 42",
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

func TestTransformRequestStripsReasoningEffortWhenNoThinkingHistory(t *testing.T) {
	transformer := NewRequestTransformer()

	// When the conversation history has NO thinking blocks, reasoning_effort
	// and thinking should be stripped to avoid DeepSeek's validation error:
	// "The reasoning_content in the thinking mode must be passed back to the API."
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
		t.Fatalf("ReasoningEffort = %v, want nil (stripped because no thinking history)", *openaiReq.ReasoningEffort)
	}
	// We explicitly send thinking: {"type":"disabled"} so DeepSeek knows
	// not to require reasoning_content on assistant messages.
	if got, want := string(openaiReq.Thinking), `{"type":"disabled"}`; got != want {
		t.Fatalf("Thinking = %s, want %s", got, want)
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
	if got, want := systemMsg.Content, "You are helpful"; got != want {
		t.Fatalf("Messages[0].Content = %q, want %q", got, want)
	}
	if systemMsg.CacheControl == nil {
		t.Fatal("Messages[0].CacheControl = nil, want non-nil")
	}
	if got, want := systemMsg.CacheControl.Type, "ephemeral"; got != want {
		t.Fatalf("Messages[0].CacheControl.Type = %q, want %q", got, want)
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

	openaiReq, err := transformer.TransformRequest(req, config.ModelConfig{ModelID: "kimi-k2.6"})
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
	if got, want := openaiReq.Messages[2].Content, "now continue"; got != want {
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
