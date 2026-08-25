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
		content := msg.TextContent()

		// For assistant messages with tool calls, serialize as text.
		for _, tc := range msg.ToolCallsList() {
			content += "[Tool: " + tc.Name + "(" + tc.Arguments + ")]"
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
		gc.Parts = append(gc.Parts, types.GeminiPart{Text: msg.TextContent()})
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
			nm.Blocks = append(nm.Blocks, core.NormalizedContentBlock{
				Type: "text", Text: msg.ContentText(),
			})
		}

		// Extract reasoning content (pointer field).
		if msg.ReasoningContent != nil {
			nm.Blocks = append(nm.Blocks, core.NormalizedContentBlock{
				Type: "thinking", Thinking: *msg.ReasoningContent,
			})
		}

		// Extract tool calls.
		for _, tc := range msg.ToolCalls {
			nm.Blocks = append(nm.Blocks, core.NormalizedContentBlock{
				Type: "tool_use", ID: tc.ID, Name: tc.Function.Name,
				Input: []byte(tc.Function.Arguments),
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
					nm.Blocks = append(nm.Blocks, core.NormalizedContentBlock{
						Type: "text", Text: c.Text,
					})
				}
			}
			nr.Messages = append(nr.Messages, nm)
		case "function_call":
			nm := core.NormalizedMessage{
				Role: "assistant",
				Blocks: []core.NormalizedContentBlock{{
					Type: "tool_use", ID: output.CallID, Name: output.Name,
					Input: []byte(output.Arguments),
				}},
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
				nm.Blocks = append(nm.Blocks, core.NormalizedContentBlock{
					Type: "text", Text: part.Text,
				})
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
		Model:        req.Model,
		MaxTokens:    req.MaxTokens,
		CacheControl: req.CacheControl,
	}

	// Set system prompt.
	if len(req.SystemBlocks) > 0 {
		if b, err := json.Marshal(normalizedBlocksToAnthropic(req.SystemBlocks)); err == nil {
			anthropicReq.System = b
		}
	} else if req.SystemPrompt != "" {
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

		blocks := normalizedMessageBlocks(nm)

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
			Name:         nt.Name,
			Description:  nt.Description,
			InputSchema:  nt.InputSchema,
			CacheControl: nt.CacheControl,
		})
	}

	return anthropicReq
}

func normalizedMessageBlocks(nm core.NormalizedMessage) []types.ContentBlock {
	return normalizedBlocksToAnthropic(nm.Blocks)
}

func normalizedBlocksToAnthropic(blocks []core.NormalizedContentBlock) []types.ContentBlock {
	out := make([]types.ContentBlock, 0, len(blocks))
	for _, block := range blocks {
		converted := types.ContentBlock{
			Type: block.Type, Text: block.Text, ID: block.ID, ToolUseID: block.ToolUseID,
			Name: block.Name, Input: block.Input, Content: block.Content,
			IsError: block.IsError, Thinking: block.Thinking, Signature: block.Signature,
			CacheControl: block.CacheControl, Raw: block.Raw,
		}
		if block.Image != nil {
			converted.Source = &types.ImageSource{
				Type: "base64", MediaType: block.Image.MediaType, Data: block.Image.Data,
			}
		}
		out = append(out, converted)
	}
	return out
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
		if content := m.TextContent(); content != "" {
			if text != "" {
				text += "\n"
			}
			text += m.Role + ": " + content
		}
	}
	return text
}
