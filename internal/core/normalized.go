package core

import (
	"encoding/json"

	"github.com/routatic/proxy/pkg/types"
)

// NormalizedContentBlock is one ordered content block. Raw preserves unknown
// provider-specific JSON so adapters can decide whether and how to forward it.
type NormalizedContentBlock struct {
	Type         string
	Text         string
	ID           string
	ToolUseID    string
	Name         string
	Input        json.RawMessage
	Content      json.RawMessage
	IsError      *bool
	Thinking     string
	Signature    string
	Image        *NormalizedImage
	CacheControl *types.CacheControl
	Raw          json.RawMessage
}

// NormalizedToolResult represents a single tool result in the normalized format.
type NormalizedToolResult struct {
	ToolCallID string
	Content    string
}

// NormalizedImage is a single image attachment in a normalized message.
type NormalizedImage struct {
	MediaType string // MIME type (e.g. "image/png")
	Data      string // Base64-encoded image data
}

// NormalizedMessage is a single message in the internal canonical format.
// All wire formats (Anthropic, OpenAI, Responses, Gemini) map to and from
// this representation.
type NormalizedMessage struct {
	Role        string                   // "user", "assistant", "system", "tool"
	Blocks      []NormalizedContentBlock // Ordered content, including unknown blocks.
	Content     string                   // Concatenated text content
	Images      []NormalizedImage        // Image attachments (user messages only)
	ToolCalls   []NormalizedToolCall     // Present on assistant messages
	ToolResults []NormalizedToolResult   // Present on user messages with tool results
	ToolCallID  string                   // Deprecated: use ToolResults instead. Kept for backward compat.
	Thinking    string                   // Reasoning/thinking content (assistant only)
}

// NormalizedToolCall represents a tool invocation in the internal format.
type NormalizedToolCall struct {
	ID        string
	Name      string
	Arguments string // JSON string
}

// NormalizedRequest is the canonical internal request format.
type NormalizedRequest struct {
	Model           string
	SystemPrompt    string
	SystemBlocks    []NormalizedContentBlock
	CacheControl    *types.CacheControl
	Messages        []NormalizedMessage
	MaxTokens       int
	Temperature     *float64
	TopP            *float64
	Stream          bool
	Tools           []NormalizedToolDef
	ReasoningEffort string // "low", "medium", "high"
	ThinkingBudget  int    // budget_tokens for thinking mode
}

// NormalizedToolDef is a tool definition in the internal format.
type NormalizedToolDef struct {
	Name         string
	Description  string
	InputSchema  []byte // JSON bytes of the schema
	CacheControl *types.CacheControl
}

// NormalizedResponse is the canonical internal response format.
type NormalizedResponse struct {
	ID         string
	Model      string
	Messages   []NormalizedMessage
	StopReason string // "end_turn", "max_tokens", "tool_use"
	Usage      NormalizedUsage
}

// NormalizedUsage holds token counts in the internal format.
type NormalizedUsage struct {
	InputTokens         int
	OutputTokens        int
	CacheReadTokens     int
	CacheCreationTokens int
}
