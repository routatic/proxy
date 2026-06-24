package core

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
	Role        string                 // "user", "assistant", "system", "tool"
	Content     string                 // Concatenated text content
	Images      []NormalizedImage      // Image attachments (user messages only)
	ToolCalls   []NormalizedToolCall   // Present on assistant messages
	ToolResults []NormalizedToolResult // Present on user messages with tool results
	ToolCallID  string                 // Deprecated: use ToolResults instead. Kept for backward compat.
	Thinking    string                 // Reasoning/thinking content (assistant only)
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
	Name        string
	Description string
	InputSchema []byte // JSON bytes of the schema
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
