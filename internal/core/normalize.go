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
		Model:        anthropicReq.Model,
		MaxTokens:    anthropicReq.MaxTokens,
		Stream:       anthropicReq.Stream != nil && *anthropicReq.Stream,
		CacheControl: anthropicReq.CacheControl,
	}

	// Extract system prompt (string or array of content blocks).
	nr.SystemPrompt = anthropicReq.SystemText()
	nr.SystemBlocks = normalizeSystemBlocks(anthropicReq.System)

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
			nm.Blocks = append(nm.Blocks, normalizeContentBlock(block))
		}

		nr.Messages = append(nr.Messages, nm)
	}

	// Convert tools.
	for _, tool := range anthropicReq.Tools {
		nt := NormalizedToolDef{
			Name:         tool.Name,
			Description:  tool.Description,
			InputSchema:  tool.InputSchema,
			CacheControl: tool.CacheControl,
		}
		nr.Tools = append(nr.Tools, nt)
	}

	return nr
}

func normalizeSystemBlocks(raw json.RawMessage) []NormalizedContentBlock {
	if len(raw) == 0 {
		return nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return []NormalizedContentBlock{{Type: "text", Text: text}}
	}
	var blocks []types.ContentBlock
	if json.Unmarshal(raw, &blocks) != nil {
		return nil
	}
	out := make([]NormalizedContentBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, normalizeContentBlock(block))
	}
	return out
}

func normalizeContentBlock(block types.ContentBlock) NormalizedContentBlock {
	content := block.Content
	if len(content) == 0 && len(block.Output) > 0 {
		content = block.Output
	}

	return NormalizedContentBlock{
		Type:      block.Type,
		Text:      block.Text,
		ID:        block.ID,
		ToolUseID: block.ToolUseID,
		Name:      block.Name,
		Input:     append(json.RawMessage(nil), block.Input...),
		Content:   append(json.RawMessage(nil), content...),
		IsError:   block.IsError,
		Thinking:  block.Thinking,
		Signature: block.Signature,
		Image: func() *NormalizedImage {
			if block.Source == nil {
				return nil
			}
			return &NormalizedImage{
				MediaType: block.Source.MediaType,
				Data:      block.Source.Data,
			}
		}(),
		CacheControl: block.CacheControl,
		Raw:          append(json.RawMessage(nil), block.Raw...),
	}
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
			for _, block := range msg.Blocks {
				resp.Content = append(resp.Content, types.ContentBlock{
					Type: block.Type, Text: block.Text, ID: block.ID,
					ToolUseID: block.ToolUseID, Name: block.Name,
					Input: block.Input, Content: block.Content,
					IsError: block.IsError, Thinking: block.Thinking,
					Signature: block.Signature, CacheControl: block.CacheControl,
					Raw: block.Raw,
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
