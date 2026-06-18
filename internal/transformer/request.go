// Package transformer handles request and response format conversion
// between Anthropic Messages API and OpenAI Chat Completions API.
package transformer

import (
	"encoding/json"
	"fmt"
	"strings"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

// contentText is a convenience wrapper around types.TextContent for brevity
// at call sites that construct ChatMessage values.
func contentText(s string) json.RawMessage {
	return types.TextContent(s)
}

// RequestTransformer converts Anthropic requests to OpenAI format.
type RequestTransformer struct{}

// NewRequestTransformer creates a new request transformer.
func NewRequestTransformer() *RequestTransformer {
	return &RequestTransformer{}
}

// isThinkingDisabled checks if the thinking JSON config explicitly sets type to "disabled".
func isThinkingDisabled(thinking json.RawMessage) bool {
	var m map[string]interface{}
	if err := json.Unmarshal(thinking, &m); err != nil {
		return false
	}
	t, ok := m["type"].(string)
	return ok && t == "disabled"
}

// isDeepSeekModel returns true for DeepSeek models that require thinking mode handling.
func isDeepSeekModel(modelID string) bool {
	return strings.HasPrefix(modelID, "deepseek-")
}

// isOpenAIReasoningModel returns true for OpenAI o1 and o3 models.
func isOpenAIReasoningModel(modelID string) bool {
	return strings.HasPrefix(modelID, "o1-") || strings.HasPrefix(modelID, "o3-")
}

// needsPlaceholderReasoning returns true for providers whose validators require
// a non-empty reasoning_content field on assistant tool-call messages.
func needsPlaceholderReasoning(modelID string) bool {
	// Moonshot's validator treats an empty string as missing.
	return strings.HasPrefix(modelID, "kimi-")
}

// stripCacheControl removes cache_control from all messages in the list.
// The caller must not hold references to the slice elements.
func stripCacheControl(messages []types.ChatMessage) {
	for i := range messages {
		messages[i].CacheControl = nil
	}
}

// TransformRequest converts an Anthropic MessageRequest to OpenAI ChatCompletionRequest.
func (t *RequestTransformer) TransformRequest(
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) (*types.ChatCompletionRequest, error) {
	// Transform messages
	messages, err := t.transformMessages(anthropicReq, model.ModelID, model.Vision)
	if err != nil {
		return nil, fmt.Errorf("failed to transform messages: %w", err)
	}

	// Strip cache_control for models that don't support it
	if !isDeepSeekModel(model.ModelID) {
		stripCacheControl(messages)
	}

	// Build OpenAI request
	openaiReq := &types.ChatCompletionRequest{
		Model:    model.ModelID,
		Messages: messages,
		Stream:   anthropicReq.Stream,
	}
	if anthropicReq.Stream != nil && *anthropicReq.Stream {
		openaiReq.StreamOptions = &types.StreamOptions{IncludeUsage: true}
	}

	// Copy optional parameters from Anthropic request
	if anthropicReq.Temperature != nil {
		openaiReq.Temperature = anthropicReq.Temperature
	}
	if anthropicReq.TopP != nil {
		openaiReq.TopP = anthropicReq.TopP
	}

	// Map max_tokens
	if anthropicReq.MaxTokens > 0 {
		maxTokens := anthropicReq.MaxTokens
		openaiReq.MaxTokens = &maxTokens
	}

	// Apply model-specific overrides
	if model.Temperature > 0 {
		openaiReq.Temperature = &model.Temperature
	}
	if model.MaxTokens > 0 {
		maxTokens := model.MaxTokens
		openaiReq.MaxTokens = &maxTokens
	}

	// Determine thinking and reasoning_effort for the upstream request.
	// Priority: explicit config → history continuity → safety guard.
	//
	// The safety guard (thinking: disabled) only engages when the history
	// contains assistant messages that lack thinking blocks — DeepSeek
	// validates reasoning_content on every assistant message in thinking
	// mode and will 400 if any are missing.  On a first turn (no assistant
	// messages) or when the user explicitly opts in via config, we send
	// thinking: enabled so the model can produce reasoning.
	resolveThinkingAndEffort(anthropicReq, model, openaiReq)

	// Transform tools if present
	if len(anthropicReq.Tools) > 0 {
		openaiReq.Tools = t.transformTools(anthropicReq.Tools)
	}

	return openaiReq, nil
}

// HasThinkingBlocks returns true if any assistant message contains
// thinking content — either as a dedicated `thinking`-typed block, or
// attached as a non-empty `thinking` field on a `tool_use` block.
//
// Claude Code emits both shapes: dedicated thinking blocks for text-only
// reasoning, and tool_use blocks with an inline `thinking` field when the
// assistant turn ends in a tool call. Both forms must mark the
// conversation as having thinking history so the proxy enables thinking
// mode on subsequent upstream calls (DeepSeek defaults to thinking mode
// and demands `reasoning_content` once it's been engaged).
func HasThinkingBlocks(messages []types.Message) bool {
	for _, msg := range messages {
		if msg.Role != "assistant" {
			continue
		}
		for _, block := range msg.ContentBlocks() {
			if block.Type == "thinking" {
				return true
			}
			if block.Type == "tool_use" && block.Thinking != "" {
				return true
			}
		}
	}
	return false
}

// resolveThinkingAndEffort applies thinking/reasoning_effort to the OpenAI
// request. Decision priority:
//
//  1. Client request — anthropicReq.Thinking set and not disabled
//     → forward thinking config; map budget_tokens to reasoning_effort.
//  2. History continuity — a prior turn used thinking → keep it enabled.
//  3. Explicit config — model.Thinking set → use it verbatim.
//  4. Config intent — model.ReasoningEffort set without model.Thinking
//     → enable on first turn (no assistant messages), disable only when
//     safety guard fires (DeepSeek + history assistant msgs lack thinking).
//  5. No config, no history → leave both unset (safety guard for DeepSeek).
//
// budgetTokensToEffort maps Anthropic budget_tokens to OpenAI reasoning_effort.
func budgetTokensToEffort(budget int) string {
	switch {
	case budget <= 2048:
		return "low"
	case budget <= 8192:
		return "medium"
	case budget <= 32768:
		return "high"
	default:
		return "max"
	}
}

// parseBudgetTokens extracts budget_tokens from a thinking JSON field.
func parseBudgetTokens(thinking json.RawMessage) int {
	var m struct {
		BudgetTokens int `json:"budget_tokens"`
	}
	if err := json.Unmarshal(thinking, &m); err != nil {
		return 0
	}
	return m.BudgetTokens
}

func resolveThinkingAndEffort(
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
	openaiReq *types.ChatCompletionRequest,
) {
	hasThinking := HasThinkingBlocks(anthropicReq.Messages)
	hasAssistant := hasAssistantMessages(anthropicReq.Messages)
	explicitThinking := len(model.Thinking) > 0
	explicitEffort := model.ReasoningEffort != ""
	isDeepSeek := isDeepSeekModel(model.ModelID)
	isOpenAIReasoning := isOpenAIReasoningModel(model.ModelID)
	requestThinkingDisabled := isThinkingDisabled(anthropicReq.Thinking)
	requestThinking := !requestThinkingDisabled && len(anthropicReq.Thinking) > 0

	allowThinkingParam := isDeepSeek || explicitThinking
	allowEffortParam := isOpenAIReasoning || isDeepSeek || explicitEffort

	if requestThinkingDisabled {
		if allowThinkingParam {
			openaiReq.Thinking = anthropicReq.Thinking
		}
		return
	}

	if isDeepSeek && hasAssistant && !hasThinking {
		if allowThinkingParam {
			openaiReq.Thinking = json.RawMessage(`{"type":"disabled"}`)
		}
		return
	}

	switch {
	case requestThinking:
		// Client explicitly opted into thinking mode via the request
		// (e.g., effortLevel in Claude Code sends thinking: {type:"enabled", budget_tokens:N}).
		// Forward the raw thinking config if allowed, and map budget_tokens to reasoning_effort if allowed.
		if allowThinkingParam {
			openaiReq.Thinking = anthropicReq.Thinking
		}
		if allowEffortParam {
			if budget := parseBudgetTokens(anthropicReq.Thinking); budget > 0 {
				effort := budgetTokensToEffort(budget)
				openaiReq.ReasoningEffort = &effort
			}
		}

	case hasThinking:
		// History has thinking blocks — maintain continuity.
		if allowThinkingParam {
			if explicitThinking {
				openaiReq.Thinking = model.Thinking
			} else {
				openaiReq.Thinking = json.RawMessage(`{"type":"enabled"}`)
			}
		}
		if allowEffortParam {
			if !isThinkingDisabled(openaiReq.Thinking) || !isDeepSeek {
				setReasoningEffort(openaiReq, model.ReasoningEffort)
			}
		}

	case explicitThinking:
		// Config explicitly sets thinking — respect it.
		if allowThinkingParam {
			openaiReq.Thinking = model.Thinking
		}
		if allowEffortParam {
			if !isThinkingDisabled(openaiReq.Thinking) || !isDeepSeek {
				setReasoningEffort(openaiReq, model.ReasoningEffort)
			}
		}

	case explicitEffort:
		// User set reasoning_effort but not thinking. Intent is clear.
		if allowThinkingParam {
			openaiReq.Thinking = json.RawMessage(`{"type":"enabled"}`)
		}
		if allowEffortParam {
			setReasoningEffort(openaiReq, model.ReasoningEffort)
		}

	default:
		// No config, no history: leave both unset.
	}
}

// setReasoningEffort sets reasoning_effort on the request, defaulting to
// "high" when the config value is empty.
func setReasoningEffort(openaiReq *types.ChatCompletionRequest, effort string) {
	if effort != "" {
		openaiReq.ReasoningEffort = &effort
	} else {
		defaultEffort := "high"
		openaiReq.ReasoningEffort = &defaultEffort
	}
}

// hasAssistantMessages returns true when the conversation contains at least
// one assistant message.
func hasAssistantMessages(messages []types.Message) bool {
	for _, msg := range messages {
		if msg.Role == "assistant" {
			return true
		}
	}
	return false
}

// transformMessages converts Anthropic messages to OpenAI format.
func (t *RequestTransformer) transformMessages(anthropicReq *types.MessageRequest, modelID string, vision bool) ([]types.ChatMessage, error) {
	hasThinking := HasThinkingBlocks(anthropicReq.Messages)

	var result []types.ChatMessage

	// Add system message from top-level field if present, preserving cache_control.
	// DeepSeek V3.x / V4 reorders all system-role messages to the front internally.
	// When Claude Code injects periodic system reminders mid-conversation, any
	// extra {"role": "system"} in the messages array would shift every subsequent
	// user/assistant/tool message after the reorder, blowing the prefix cache.
	systemText := anthropicReq.SystemText()
	if systemText != "" {
		systemMsg := types.ChatMessage{
			Role:    "system",
			Content: contentText(systemText),
		}
		if !strings.HasPrefix(modelID, "kimi-") && len(anthropicReq.System) > 0 {
			var blocks []types.SystemContentBlock
			if err := json.Unmarshal(anthropicReq.System, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "text" && b.CacheControl != nil {
						systemMsg.CacheControl = b.CacheControl
						break
					}
				}
			}
		}
		result = append(result, systemMsg)
	}

	// Transform remaining messages.
	//
	// DeepSeek V3.x / V4 internally reorders all system-role messages to the
	// front of the effective prompt.  When Claude Code injects periodic system
	// reminders mid-conversation (e.g. "task tools haven't been used recently"),
	// forwarding them as {"role": "system"} would cause DeepSeek's reordering
	// to shift every user/assistant/tool message, invalidating the prefix cache
	// from the insertion point onward.
	//
	// For DeepSeek models only, convert non-top-level system messages into user
	// messages wrapped in <system-reminder> tags.  This preserves the semantic
	// intent while preventing DeepSeek from reordering them past the
	// conversational history.
	rewriteSystem := isDeepSeekModel(modelID)
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "system" && rewriteSystem {
			blocks := msg.ContentBlocks()
			var sb strings.Builder
			for _, b := range blocks {
				if b.Type == "text" {
					sb.WriteString(b.Text)
				}
			}
			text := sb.String()
			if text == "" {
				continue
			}
			// Deduplicate: skip if this text is already part of the top-level
			// system prompt (matched after trimming whitespace on both sides).
			if canonicalSystem := strings.TrimSpace(systemText); canonicalSystem != "" {
				canonicalText := strings.TrimSpace(text)
				if strings.Contains(canonicalSystem, canonicalText) {
					continue
				}
			}
			result = append(result, types.ChatMessage{
				Role:    "user",
				Content: contentText("<system-reminder>\n" + text + "\n</system-reminder>"),
			})
			continue
		}
		openaiMsgs, err := t.transformMessage(msg, modelID, hasThinking, vision)
		if err != nil {
			return nil, err
		}
		result = append(result, openaiMsgs...)
	}

	return result, nil
}

// transformMessage converts a single Anthropic message to one or more OpenAI messages.
// Tool_use and tool_result require special handling to map to OpenAI's function calling format.
func (t *RequestTransformer) transformMessage(msg types.Message, modelID string, hasThinkingInHistory bool, vision bool) ([]types.ChatMessage, error) {
	blocks := msg.ContentBlocks()

	switch msg.Role {
	case "user":
		return t.transformUserMessage(blocks, vision)
	case "assistant":
		return t.transformAssistantMessage(blocks, modelID, hasThinkingInHistory)
	default:
		// Fallback: concatenate all text
		var text string
		for _, b := range blocks {
			if b.Type == "text" {
				text += b.Text
			}
		}
		return []types.ChatMessage{{Role: msg.Role, Content: contentText(text)}}, nil
	}
}

// transformUserMessage converts a user message with potential tool_result and image blocks.
// Image blocks are converted to OpenAI's multimodal content format (content array
// with image_url parts) so that vision-capable models receive the actual image data.
// For models without vision support, image blocks are replaced with a "[Image]" text
// placeholder to prevent upstream 400 errors from unsupported image_url parts.
func (t *RequestTransformer) transformUserMessage(blocks []types.ContentBlock, vision bool) ([]types.ChatMessage, error) {
	var result []types.ChatMessage
	var textParts []string
	var imageParts []types.ChatContentPart
	hasImage := false

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_result":
			// In OpenAI, tool results are separate messages with role "tool"
			toolContent := block.TextContent()
			result = append(result, types.ChatMessage{
				Role:       "tool",
				Content:    contentText(toolContent),
				ToolCallID: block.GetToolID(),
			})
		case "image":
			if block.Source != nil {
				if vision {
					imageParts = append(imageParts, types.ChatContentPart{
						Type: "image_url",
						ImageURL: &types.ImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", block.Source.MediaType, block.Source.Data),
						},
					})
				} else {
					hasImage = true
				}
			}
		}
	}

	// If there's text or image content, add it as a user message.
	// OpenAI-compatible tool calling requires tool responses to appear
	// immediately after the assistant message that emitted tool_calls.
	// If the Anthropic user turn also includes free-form text and/or images,
	// emit it as a subsequent user message after all tool results.
	if len(textParts) > 0 || len(imageParts) > 0 || hasImage {
		if len(imageParts) > 0 {
			// Multimodal message: build content array with text + image_url parts
			var parts []types.ChatContentPart
			if len(textParts) > 0 {
				parts = append(parts, types.ChatContentPart{
					Type: "text",
					Text: strings.Join(textParts, ""),
				})
			}
			parts = append(parts, imageParts...)
			contentJSON, err := json.Marshal(parts)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal multimodal content: %w", err)
			}
			result = append(result, types.ChatMessage{Role: "user", Content: contentJSON})
		} else {
			// Text-only message (possibly with image placeholder for non-vision models)
			text := strings.Join(textParts, "")
			if hasImage {
				if text != "" {
					text += "\n\n[Image]"
				} else {
					text = "[Image]"
				}
			}
			result = append(result, types.ChatMessage{
				Role:    "user",
				Content: contentText(text),
			})
		}
	}

	return result, nil
}

// transformAssistantMessage converts an assistant message with potential tool_use blocks.
func (t *RequestTransformer) transformAssistantMessage(blocks []types.ContentBlock, modelID string, hasThinkingInHistory bool) ([]types.ChatMessage, error) {
	var textParts []string
	var thinkingParts []string
	var toolCalls []types.ToolCall

	for _, block := range blocks {
		switch block.Type {
		case "text":
			textParts = append(textParts, block.Text)
		case "thinking":
			// Preserve chain-of-thought so it can be forwarded back to providers
			// that require reasoning_content to be preserved across turns.
			if block.Thinking != "" {
				thinkingParts = append(thinkingParts, block.Thinking)
			}
		case "tool_use":
			// Claude Code can attach reasoning directly to the tool_use block
			// (instead of emitting a separate thinking-typed block) when the
			// assistant turn ends in a tool call. Extract that here so it
			// round-trips back to upstream as reasoning_content — otherwise
			// DeepSeek (which always operates in thinking mode after the
			// first reasoning response) returns 400 on the next request.
			if block.Thinking != "" {
				thinkingParts = append(thinkingParts, block.Thinking)
			}
			arguments := "{}"
			if len(block.Input) > 0 {
				arguments = string(block.Input)
			}
			toolCalls = append(toolCalls, types.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: types.FunctionCall{
					Name:      block.Name,
					Arguments: arguments,
				},
			})
		}
	}

	// Build the assistant message
	content := ""
	for _, p := range textParts {
		content += p
	}
	reasoningContent := ""
	for _, p := range thinkingParts {
		reasoningContent += p
	}

	var reasoningContentPtr *string
	if reasoningContent != "" {
		// Real thinking content from the upstream history — preserve it.
		reasoningContentPtr = &reasoningContent
	} else if hasThinkingInHistory && isDeepSeekModel(modelID) {
		// DeepSeek in thinking mode requires reasoning_content on EVERY
		// assistant message — text-only continuation turns and tool_use
		// turns alike — whenever the conversation was opened in thinking
		// mode. Without this, upstream returns:
		//   400 invalid_request_error: "The `reasoning_content` in the
		//   thinking mode must be passed back to the API."
		// Use a single-space placeholder for assistant turns whose original
		// thinking blocks were stripped by Claude Code (compact summaries,
		// dropped reasoning blocks, etc.) — DeepSeek checks for the field's
		// presence and non-empty content, not its semantic value.
		placeholder := " "
		reasoningContentPtr = &placeholder
	} else if len(toolCalls) > 0 && needsPlaceholderReasoning(modelID) {
		// Moonshot's validator treats an empty string as missing, so use a
		// non-empty placeholder when we must provide the field.
		placeholder := " "
		reasoningContentPtr = &placeholder
	}

	msg := types.ChatMessage{
		Role:             "assistant",
		Content:          contentText(content),
		ReasoningContent: reasoningContentPtr,
		ToolCalls:        toolCalls,
	}

	return []types.ChatMessage{msg}, nil
}

// transformTools converts Anthropic tools to OpenAI tools.
func (t *RequestTransformer) transformTools(tools []types.Tool) []types.ToolDef {
	var result []types.ToolDef

	for _, tool := range tools {
		// InputSchema is already json.RawMessage, use it directly
		schema := tool.InputSchema
		if len(schema) == 0 {
			schema = []byte(`{"type":"object","properties":{}}`)
		}

		result = append(result, types.ToolDef{
			Type: "function",
			Function: types.FunctionDef{
				Name:        tool.Name,
				Description: tool.Description,
				Parameters:  json.RawMessage(schema),
			},
		})
	}

	return result
}
