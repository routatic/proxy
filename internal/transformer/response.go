// Package transformer handles request/response transformation and token counting.
package transformer

import (
	"encoding/json"
	"fmt"

	"oc-go-cc/pkg/types"
)

// ResponseTransformer converts OpenAI responses to Anthropic format.
type ResponseTransformer struct{}

// nonNegative clamps an integer to zero. Used when subtracting cache-token
// counts from prompt totals: defends against upstream payloads where the
// reported parts don't consistently sum to the whole.
func nonNegative(n int) int {
	if n < 0 {
		return 0
	}
	return n
}

// NewResponseTransformer creates a new response transformer.
func NewResponseTransformer() *ResponseTransformer {
	return &ResponseTransformer{}
}

// TransformResponse converts an OpenAI ChatCompletionResponse to Anthropic MessageResponse.
func (t *ResponseTransformer) TransformResponse(
	openaiResp *types.ChatCompletionResponse,
	originalModel string,
) (*types.MessageResponse, error) {
	if len(openaiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := openaiResp.Choices[0]

	// Transform content blocks from the OpenAI message.
	contentBlocks, err := t.transformContent(choice.Message)
	if err != nil {
		return nil, fmt.Errorf("failed to transform content: %w", err)
	}

	// Map OpenAI finish reason to Anthropic stop reason.
	stopReason := t.mapFinishReason(choice.FinishReason)

	// Build Anthropic response.
	anthropicResp := &types.MessageResponse{
		ID:           openaiResp.ID,
		Type:         "message",
		Role:         "assistant",
		Content:      contentBlocks,
		Model:        originalModel,
		StopReason:   stopReason,
		StopSequence: "",
		Usage: types.Usage{
			// Per Anthropic Messages API spec, `input_tokens` is the count of
			// regular input tokens — i.e. tokens that were neither read from
			// the cache nor written to the cache this turn. OpenAI's
			// `prompt_tokens` is the *total* prompt size including both. When
			// the upstream reports prompt-cache fields we have to subtract
			// them out, otherwise Claude Code's local context counter sees an
			// inflated input_tokens on every turn and trips auto-compact ~5x
			// too early on long-prefix sessions.
			InputTokens:              nonNegative(openaiResp.Usage.PromptTokens - openaiResp.Usage.PromptCacheHitTokens - openaiResp.Usage.PromptCacheMissTokens),
			OutputTokens:             openaiResp.Usage.CompletionTokens,
			CacheCreationInputTokens: openaiResp.Usage.PromptCacheMissTokens,
			CacheReadInputTokens:     openaiResp.Usage.PromptCacheHitTokens,
		},
	}

	return anthropicResp, nil
}

// transformContent converts an OpenAI message to Anthropic content blocks.
func (t *ResponseTransformer) transformContent(msg types.ChatMessage) ([]types.ContentBlock, error) {
	var blocks []types.ContentBlock

	// Preserve reasoning content as a thinking block so it round-trips correctly
	// on multi-turn tool-calling conversations.
	if msg.ReasoningContent != nil && *msg.ReasoningContent != "" {
		blocks = append(blocks, types.ContentBlock{
			Type:     "thinking",
			Thinking: *msg.ReasoningContent,
		})
	}

	// Handle tool calls — each becomes a tool_use content block.
	for _, tc := range msg.ToolCalls {
		// Arguments come as a JSON string from OpenAI, pass as raw JSON
		inputJSON := json.RawMessage(`{}`)
		if tc.Function.Arguments != "" {
			inputJSON = json.RawMessage(tc.Function.Arguments)
		}

		blocks = append(blocks, types.ContentBlock{
			Type:  "tool_use",
			ID:    tc.ID,
			Name:  tc.Function.Name,
			Input: inputJSON,
		})
	}

	// Handle text content.
	if msg.Content != "" {
		blocks = append(blocks, types.ContentBlock{
			Type: "text",
			Text: msg.Content,
		})
	}

	// Ensure at least one content block exists
	if len(blocks) == 0 {
		blocks = append(blocks, types.ContentBlock{
			Type: "text",
			Text: "",
		})
	}

	return blocks, nil
}

// mapFinishReason maps OpenAI finish reasons to Anthropic stop reasons.
func (t *ResponseTransformer) mapFinishReason(reason string) string {
	switch reason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "end_turn"
	default:
		return "end_turn"
	}
}

// TransformErrorResponse converts an HTTP error into an Anthropic-style error map.
func TransformErrorResponse(statusCode int, message string) map[string]interface{} {
	return map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    mapHTTPStatusToErrorType(statusCode),
			"message": message,
		},
	}
}

// mapHTTPStatusToErrorType maps HTTP status codes to Anthropic error type strings.
func mapHTTPStatusToErrorType(statusCode int) string {
	switch {
	case statusCode == 400:
		return "invalid_request_error"
	case statusCode == 401:
		return "authentication_error"
	case statusCode == 403:
		return "permission_error"
	case statusCode == 404:
		return "not_found_error"
	case statusCode == 429:
		return "rate_limit_error"
	case statusCode >= 500:
		return "api_error"
	default:
		return "api_error"
	}
}
