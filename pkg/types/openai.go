// Package types defines shared data structures and interfaces.
package types

import "encoding/json"

// OpenAI API types for the Chat Completions API.
// Reference: https://platform.openai.com/docs/api-reference/chat

// ChatContentPart represents a single part in a multimodal message content array.
type ChatContentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *ImageURL `json:"image_url,omitempty"`
}

// ImageURL represents the URL source for an image in a multimodal message.
type ImageURL struct {
	URL string `json:"url"`
}

// TextContent is a helper to create a json.RawMessage for a plain-text content value.
func TextContent(s string) json.RawMessage {
	// json.Marshal(string) never fails for a valid Go string.
	b, _ := json.Marshal(s)
	return b
}

// ChatCompletionRequest represents a request to the OpenAI Chat Completions API.
type ChatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []ChatMessage   `json:"messages"`
	Stream          *bool           `json:"stream,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	MaxTokens       *int            `json:"max_tokens,omitempty"`
	ReasoningEffort *string         `json:"reasoning_effort,omitempty"`
	Thinking        json.RawMessage `json:"thinking,omitempty"`
	Tools           []ToolDef       `json:"tools,omitempty"`
	ToolChoice      interface{}     `json:"tool_choice,omitempty"`
	Stop            interface{}     `json:"stop,omitempty"`
	StreamOptions   *StreamOptions  `json:"stream_options,omitempty"`
}

// StreamOptions controls streaming response metadata from OpenAI-compatible APIs.
type StreamOptions struct {
	IncludeUsage bool `json:"include_usage,omitempty"`
}

// ChatMessage represents a single message in the conversation.
// Content can be either a plain text string or an array of ChatContentPart
// for multimodal messages containing images.
type ChatMessage struct {
	Role             string          `json:"role"`
	Content          json.RawMessage `json:"content"`
	ReasoningContent *string         `json:"reasoning_content,omitempty"`
	ToolCalls        []ToolCall      `json:"tool_calls,omitempty"`
	Name             string          `json:"name,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	CacheControl     *CacheControl   `json:"cache_control,omitempty"`
}

// ContentText extracts the text content from the message, handling both
// plain text strings and multimodal content arrays (where it concatenates
// all text-type parts). Returns empty string if content is absent or unparseable. This is intentional
// — ContentText is a best-effort text extractor for routing and display purposes, not a
// strict parser. Failures are never fatal; callers should handle empty returns gracefully.
func (m ChatMessage) ContentText() string {
	if len(m.Content) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(m.Content, &s); err == nil {
		return s
	}
	var parts []ChatContentPart
	if err := json.Unmarshal(m.Content, &parts); err == nil {
		var text string
		for _, p := range parts {
			if p.Type == "text" {
				text += p.Text
			}
		}
		return text
	}
	return ""
}

// ToolCall represents a function call made by the model.
// Index is only present in streaming deltas — it identifies which tool call
// position this delta belongs to within the tool_calls array.
type ToolCall struct {
	Index    int          `json:"index,omitempty"`
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall represents the function invocation details.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolDef represents a tool definition for function calling.
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef represents the function definition schema.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ChatCompletionResponse represents a response from the OpenAI Chat Completions API.
type ChatCompletionResponse struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created int64     `json:"created"`
	Model   string    `json:"model"`
	Choices []Choice  `json:"choices"`
	Usage   UsageInfo `json:"usage"`
}

// Choice represents a single choice in the response.
type Choice struct {
	Index        int         `json:"index"`
	Message      ChatMessage `json:"message"`
	FinishReason string      `json:"finish_reason,omitempty"`
	Delta        ChatMessage `json:"delta,omitempty"`
}

// UsageInfo represents token usage information.
type UsageInfo struct {
	PromptTokens          int `json:"prompt_tokens"`
	CompletionTokens      int `json:"completion_tokens"`
	TotalTokens           int `json:"total_tokens"`
	PromptCacheHitTokens  int `json:"prompt_cache_hit_tokens,omitempty"`
	PromptCacheMissTokens int `json:"prompt_cache_miss_tokens,omitempty"`
}

// ChatCompletionChunk represents a streaming chunk from the Chat Completions API.
type ChatCompletionChunk struct {
	ID      string     `json:"id"`
	Object  string     `json:"object"`
	Created int64      `json:"created"`
	Model   string     `json:"model"`
	Choices []Choice   `json:"choices"`
	Usage   *UsageInfo `json:"usage,omitempty"`
}

// ErrorResponse represents an error response from the OpenAI API.
type ErrorResponse struct {
	Error ErrorDetails `json:"error"`
}

// ErrorDetails contains the details of an API error.
type ErrorDetails struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code,omitempty"`
}
