package transformer

import (
	"encoding/json"

	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/pkg/types"
)

// ── Request-side: NormalizedRequest → wire format ─────────────────────

// TransformRequestFromNormalized converts a NormalizedRequest to OpenAI
// ChatCompletionRequest by first reconstructing the Anthropic format and
// running it through the existing TransformRequest pipeline.
func TransformRequestFromNormalized(req *core.NormalizedRequest, model config.ModelConfig) *types.ChatCompletionRequest {
	anthropicReq := normalizedToMessageRequest(req)
	t := NewRequestTransformer()
	openaiReq, err := t.TransformRequest(anthropicReq, model)
	if err != nil {
		// The Anthropic reconstruction should never fail for valid normalized
		// requests, but if it does, return a minimal valid request so the
		// upstream gets a usable payload rather than a nil pointer.
		stream := req.Stream
		maxTokens := req.MaxTokens
		return &types.ChatCompletionRequest{
			Model:     model.ModelID,
			Messages:  []types.ChatMessage{{Role: "user", Content: types.TextContent(req.SystemPrompt + "\n" + joinMessageText(req.Messages))}},
			Stream:    &stream,
			MaxTokens: &maxTokens,
		}
	}
	return openaiReq
}

// NormalizedToAnthropic converts a NormalizedRequest to an Anthropic MessageRequest.
func NormalizedToAnthropic(req *core.NormalizedRequest, model config.ModelConfig) *types.MessageRequest {
	anthropicReq := normalizedToMessageRequest(req)
	// Override model ID with the config's model ID.
	anthropicReq.Model = model.ModelID
	return anthropicReq
}

// NormalizedToResponses converts a NormalizedRequest to a ResponsesRequest.
func NormalizedToResponses(req *core.NormalizedRequest, model config.ModelConfig) *types.ResponsesRequest {
	responsesReq := &types.ResponsesRequest{
		Model: model.ModelID,
	}

	// System prompt becomes a "developer" role input.
	if req.SystemPrompt != "" {
		responsesReq.Input = append(responsesReq.Input, types.ResponsesInput{
			Role:    "developer",
			Content: rawJSONString(req.SystemPrompt),
		})
	}

	// Convert messages.
	for _, msg := range req.Messages {
		input := types.ResponsesInput{Role: msg.Role}
		content := msg.Content

		// For assistant messages with tool calls, serialize as text.
		if len(msg.ToolCalls) > 0 {
			for _, tc := range msg.ToolCalls {
				content += "[Tool: " + tc.Name + "(" + tc.Arguments + ")]"
			}
		}

		if content != "" {
			input.Content = rawJSONString(content)
		}
		responsesReq.Input = append(responsesReq.Input, input)
	}

	// Convert tools.
	for _, tool := range req.Tools {
		responsesReq.Tools = append(responsesReq.Tools, types.ResponsesTool{
			Type:        "function",
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.InputSchema,
		})
	}

	return responsesReq
}

// NormalizedToGemini converts a NormalizedRequest to a GeminiRequest.
func NormalizedToGemini(req *core.NormalizedRequest, model config.ModelConfig) *types.GeminiRequest {
	geminiReq := &types.GeminiRequest{
		GenerationConfig: &types.GeminiGenerationConfig{
			MaxOutputTokens: req.MaxTokens,
		},
	}

	if req.Temperature != nil {
		geminiReq.GenerationConfig.Temperature = *req.Temperature
	}

	// System prompt is prepended as a user message (Gemini has no system role).
	var contents []types.GeminiContent
	if req.SystemPrompt != "" {
		contents = append(contents, types.GeminiContent{
			Role:  "user",
			Parts: []types.GeminiPart{{Text: req.SystemPrompt}},
		})
	}

	// Convert messages.
	for _, msg := range req.Messages {
		gc := types.GeminiContent{Role: msg.Role}
		gc.Parts = append(gc.Parts, types.GeminiPart{Text: msg.Content})
		contents = append(contents, gc)
	}

	geminiReq.Contents = contents

	// Convert tools.
	if len(req.Tools) > 0 {
		var functions []types.GeminiFunctionDeclaration
		for _, tool := range req.Tools {
			functions = append(functions, types.GeminiFunctionDeclaration{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  tool.InputSchema,
			})
		}
		geminiReq.Tools = []types.GeminiTool{
			{FunctionDeclarations: functions},
		}
	}

	return geminiReq
}

// ── Response-side: wire format → NormalizedResponse ───────────────────

// OpenAIResponseToNormalized converts an OpenAI ChatCompletionResponse to NormalizedResponse.
func OpenAIResponseToNormalized(openaiResp *types.ChatCompletionResponse, modelID string) *core.NormalizedResponse {
	nr := &core.NormalizedResponse{
		ID:    openaiResp.ID,
		Model: modelID,
	}

	for _, choice := range openaiResp.Choices {
		msg := choice.Message

		nm := core.NormalizedMessage{Role: msg.Role}

		// Extract text content.
		if msg.Content != nil {
			nm.Content = msg.ContentText()
		}

		// Extract reasoning content (pointer field).
		if msg.ReasoningContent != nil {
			nm.Thinking = *msg.ReasoningContent
		}

		// Extract tool calls.
		for _, tc := range msg.ToolCalls {
			nm.ToolCalls = append(nm.ToolCalls, core.NormalizedToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			})
		}

		nr.Messages = append(nr.Messages, nm)

		// Map finish reason.
		switch choice.FinishReason {
		case "stop":
			nr.StopReason = "end_turn"
		case "length":
			nr.StopReason = "max_tokens"
		case "tool_calls":
			nr.StopReason = "tool_use"
		default:
			nr.StopReason = "end_turn"
		}
	}

	// Map usage. UsageInfo is a value type; check if it was populated.
	if openaiResp.Usage.PromptTokens > 0 || openaiResp.Usage.CompletionTokens > 0 {
		nr.Usage = core.NormalizedUsage{
			InputTokens:         openaiResp.Usage.PromptTokens,
			OutputTokens:        openaiResp.Usage.CompletionTokens,
			CacheReadTokens:     openaiResp.Usage.PromptCacheHitTokens,
			CacheCreationTokens: openaiResp.Usage.PromptCacheMissTokens,
		}
	}

	return nr
}

// ResponsesToNormalized converts an OpenAI ResponsesResponse to NormalizedResponse.
func ResponsesToNormalized(responsesResp *types.ResponsesResponse, modelID string) *core.NormalizedResponse {
	nr := &core.NormalizedResponse{
		ID:    responsesResp.ID,
		Model: modelID,
	}

	for _, output := range responsesResp.Output {
		switch output.Type {
		case "message":
			nm := core.NormalizedMessage{Role: output.Role}
			for _, c := range output.Content {
				if c.Type == "output_text" {
					nm.Content += c.Text
				}
			}
			nr.Messages = append(nr.Messages, nm)
		case "function_call":
			nm := core.NormalizedMessage{
				Role: "assistant",
				ToolCalls: []core.NormalizedToolCall{
					{
						ID:        output.CallID,
						Name:      output.Name,
						Arguments: output.Arguments,
					},
				},
			}
			nr.Messages = append(nr.Messages, nm)
		}
	}

	nr.StopReason = "end_turn"

	nr.Usage = core.NormalizedUsage{
		InputTokens:  responsesResp.Usage.InputTokens,
		OutputTokens: responsesResp.Usage.OutputTokens,
	}

	return nr
}

// GeminiToNormalized converts a GeminiResponse to NormalizedResponse.
func GeminiToNormalized(geminiResp *types.GeminiResponse, modelID string) *core.NormalizedResponse {
	nr := &core.NormalizedResponse{
		Model: modelID,
	}

	if len(geminiResp.Candidates) > 0 {
		candidate := geminiResp.Candidates[0]
		nm := core.NormalizedMessage{Role: candidate.Content.Role}

		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				nm.Content += part.Text
			}
		}

		nr.Messages = append(nr.Messages, nm)

		switch candidate.FinishReason {
		case "STOP":
			nr.StopReason = "end_turn"
		case "MAX_TOKENS":
			nr.StopReason = "max_tokens"
		default:
			nr.StopReason = "end_turn"
		}
	}

	if geminiResp.UsageMetadata != nil {
		nr.Usage = core.NormalizedUsage{
			InputTokens:  geminiResp.UsageMetadata.PromptTokenCount,
			OutputTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	return nr
}

// ── Helpers ───────────────────────────────────────────────────────────

// normalizedToMessageRequest reconstructs an Anthropic MessageRequest from a
// NormalizedRequest. This is used as input to the existing TransformRequest
// pipeline.
func normalizedToMessageRequest(req *core.NormalizedRequest) *types.MessageRequest {
	anthropicReq := &types.MessageRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
	}

	// Set system prompt.
	if req.SystemPrompt != "" {
		if b, err := json.Marshal(req.SystemPrompt); err == nil {
			anthropicReq.System = json.RawMessage(b)
		}
	}

	// Set stream.
	if req.Stream {
		t := true
		anthropicReq.Stream = &t
	}

	// Set temperature.
	if req.Temperature != nil {
		anthropicReq.Temperature = req.Temperature
	}

	// Set thinking.
	if req.ReasoningEffort != "" || req.ThinkingBudget > 0 {
		tc := map[string]any{
			"type":          req.ReasoningEffort,
			"budget_tokens": req.ThinkingBudget,
		}
		if b, err := json.Marshal(tc); err == nil {
			anthropicReq.Thinking = b
		}
	}

	// Convert messages.
	for _, nm := range req.Messages {
		msg := types.Message{Role: nm.Role}

		var blocks []types.ContentBlock
		if nm.Content != "" {
			blocks = append(blocks, types.ContentBlock{Type: "text", Text: nm.Content})
		}
		// Reconstruct image blocks from the normalized representation so the
		// downstream transformer can decide whether to convert them to
		// image_url (vision-capable model) or to a [Image] text placeholder.
		for _, img := range nm.Images {
			blocks = append(blocks, types.ContentBlock{
				Type: "image",
				Source: &types.ImageSource{
					Type:      "base64",
					MediaType: img.MediaType,
					Data:      img.Data,
				},
			})
		}
		if nm.Thinking != "" {
			blocks = append(blocks, types.ContentBlock{Type: "thinking", Thinking: nm.Thinking})
		}
		for _, tc := range nm.ToolCalls {
			blocks = append(blocks, types.ContentBlock{
				Type:  "tool_use",
				ID:    tc.ID,
				Name:  tc.Name,
				Input: []byte(tc.Arguments),
			})
		}
		if len(nm.ToolResults) > 0 {
			for _, tr := range nm.ToolResults {
				content, _ := json.Marshal(tr.Content)
				blocks = append(blocks, types.ContentBlock{
					Type:      "tool_result",
					ToolUseID: tr.ToolCallID,
					Content:   content,
				})
			}
		} else if nm.ToolCallID != "" {
			content, _ := json.Marshal(nm.Content)
			blocks = append(blocks, types.ContentBlock{
				Type:      "tool_result",
				ToolUseID: nm.ToolCallID,
				Content:   content,
			})
		}

		if len(blocks) > 0 {
			b, _ := json.Marshal(blocks)
			msg.Content = b
		} else {
			msg.Content = json.RawMessage(`""`)
		}

		anthropicReq.Messages = append(anthropicReq.Messages, msg)
	}

	// Convert tools.
	for _, nt := range req.Tools {
		anthropicReq.Tools = append(anthropicReq.Tools, types.Tool{
			Name:        nt.Name,
			Description: nt.Description,
			InputSchema: nt.InputSchema,
		})
	}

	return anthropicReq
}

func rawJSONString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return json.RawMessage(b)
}

// joinMessageText concatenates the content of all messages for use as a
// fallback when the transform pipeline fails.
func joinMessageText(messages []core.NormalizedMessage) string {
	var text string
	for _, m := range messages {
		if m.Content != "" {
			if text != "" {
				text += "\n"
			}
			text += m.Role + ": " + m.Content
		}
	}
	return text
}
