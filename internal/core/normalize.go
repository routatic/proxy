package core

import (
	"encoding/json"

	"github.com/routatic/proxy/pkg/types"
)

// thinkingConfig mirrors the Anthropic thinking field structure so we can
// decode it without coupling to a specific json.RawMessage layout.
type thinkingConfig struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens,omitempty"`
}

// NormalizeRequest converts an Anthropic MessageRequest to a NormalizedRequest.
// This is a lossless extraction: all data from the Anthropic format survives.
func NormalizeRequest(anthropicReq *types.MessageRequest) *NormalizedRequest {
	nr := &NormalizedRequest{
		Model:     anthropicReq.Model,
		MaxTokens: anthropicReq.MaxTokens,
		Stream:    anthropicReq.Stream != nil && *anthropicReq.Stream,
	}

	// Extract system prompt (string or array of content blocks).
	nr.SystemPrompt = anthropicReq.SystemText()

	// Set temperature if provided.
	if anthropicReq.Temperature != nil {
		nr.Temperature = anthropicReq.Temperature
	}

	// Extract reasoning effort and thinking budget.
	if len(anthropicReq.Thinking) > 0 {
		var tc thinkingConfig
		if err := json.Unmarshal(anthropicReq.Thinking, &tc); err == nil {
			nr.ReasoningEffort = tc.Type
			nr.ThinkingBudget = tc.BudgetTokens
		}
	}

	// Convert messages.
	for _, msg := range anthropicReq.Messages {
		nm := NormalizedMessage{
			Role: msg.Role,
		}

		blocks := msg.ContentBlocks()
		for _, block := range blocks {
			switch block.Type {
			case "text":
				nm.Content += block.Text
			case "tool_use":
				nm.ToolCalls = append(nm.ToolCalls, NormalizedToolCall{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: string(block.Input),
				})
			case "tool_result":
				nm.ToolResults = append(nm.ToolResults, NormalizedToolResult{
					ToolCallID: block.ToolUseID,
					Content:    block.TextContent(),
				})
			case "thinking":
				nm.Thinking += block.Thinking
			case "image":
				// Preserve image data so the downstream transformer can convert
				// to image_url (or append a [Image] placeholder if the model
				// does not support vision). Previously this was collapsed to
				// the literal text "[Image]" which destroyed the image bytes
				// before the transformer could inspect them.
				if block.Source != nil && block.Source.Data != "" {
					nm.Images = append(nm.Images, NormalizedImage{
						MediaType: block.Source.MediaType,
						Data:      block.Source.Data,
					})
				}
			}
		}

		nr.Messages = append(nr.Messages, nm)
	}

	// Convert tools.
	for _, tool := range anthropicReq.Tools {
		nt := NormalizedToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.InputSchema,
		}
		nr.Tools = append(nr.Tools, nt)
	}

	return nr
}

// DenormalizeResponse converts a NormalizedResponse to an Anthropic MessageResponse.
func DenormalizeResponse(nr *NormalizedResponse) *types.MessageResponse {
	resp := &types.MessageResponse{
		ID:    nr.ID,
		Type:  "message",
		Model: nr.Model,
		Usage: types.Usage{
			InputTokens:              nr.Usage.InputTokens,
			OutputTokens:             nr.Usage.OutputTokens,
			CacheCreationInputTokens: nr.Usage.CacheCreationTokens,
			CacheReadInputTokens:     nr.Usage.CacheReadTokens,
		},
	}

	// Build content blocks from messages.
	for _, msg := range nr.Messages {
		switch msg.Role {
		case "assistant":
			resp.Role = "assistant"

			// Add thinking block if present.
			if msg.Thinking != "" {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type:     "thinking",
					Thinking: msg.Thinking,
				})
			}

			// Add text block if present.
			if msg.Content != "" {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type: "text",
					Text: msg.Content,
				})
			}

			// Add tool_use blocks.
			for _, tc := range msg.ToolCalls {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type:  "tool_use",
					ID:    tc.ID,
					Name:  tc.Name,
					Input: []byte(tc.Arguments),
				})
			}
		}

		// Determine stop reason.
		switch nr.StopReason {
		case "end_turn":
			resp.StopReason = "end_turn"
		case "max_tokens":
			resp.StopReason = "max_tokens"
		case "tool_use":
			resp.StopReason = "tool_use"
		default:
			resp.StopReason = "end_turn"
		}
	}

	return resp
}
