package types

import "encoding/json"

// OpenAI Responses API types.
// Reference: https://platform.openai.com/docs/api-reference/responses

// ResponsesRequest represents a request to the OpenAI Responses API.
type ResponsesRequest struct {
	Model     string              `json:"model"`
	Input     []ResponsesInput    `json:"input"`
	Stream    bool                `json:"stream,omitempty"`
	Tools     []ResponsesTool     `json:"tools,omitempty"`
	Reasoning *ResponsesReasoning `json:"reasoning,omitempty"`
}

// ResponsesInput represents a single input item in the Responses request.
type ResponsesInput struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content,omitempty"`
}

// ResponsesTool represents a tool definition for the Responses API.
type ResponsesTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name,omitempty"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ResponsesReasoning controls reasoning effort.
type ResponsesReasoning struct {
	Effort string `json:"effort,omitempty"`
}

// ResponsesResponse represents a response from the OpenAI Responses API.
type ResponsesResponse struct {
	ID      string            `json:"id"`
	Object  string            `json:"object"`
	Created int64             `json:"created"`
	Model   string            `json:"model"`
	Output  []ResponsesOutput `json:"output"`
	Usage   ResponsesUsage    `json:"usage"`
}

// ResponsesOutput represents a single output item.
type ResponsesOutput struct {
	Type      string             `json:"type"`
	ID        string             `json:"id,omitempty"`
	Role      string             `json:"role,omitempty"`
	Content   []ResponsesContent `json:"content,omitempty"`
	CallID    string             `json:"call_id,omitempty"`
	Name      string             `json:"name,omitempty"`
	Arguments string             `json:"arguments,omitempty"`
}

// ResponsesContent represents content within an output item.
type ResponsesContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// ResponsesUsage represents token usage in a Responses response.
type ResponsesUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// ResponsesChunk represents a streaming chunk from the Responses API.
type ResponsesChunk struct {
	Type   string            `json:"type"`
	ID     string            `json:"id,omitempty"`
	Delta  string            `json:"delta,omitempty"`
	Output []ResponsesOutput `json:"output,omitempty"`
	Usage  *ResponsesUsage   `json:"usage,omitempty"`
}

// Google Gemini API types.
// Reference: https://ai.google.dev/api/generate-content

// GeminiRequest represents a request to the Gemini API.
type GeminiRequest struct {
	Contents         []GeminiContent         `json:"contents"`
	GenerationConfig *GeminiGenerationConfig `json:"generationConfig,omitempty"`
	Tools            []GeminiTool            `json:"tools,omitempty"`
	Stream           bool                    `json:"stream,omitempty"`
}

// GeminiContent represents a single content item.
type GeminiContent struct {
	Role  string       `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

// GeminiPart represents a part of a content item.
type GeminiPart struct {
	Text string `json:"text,omitempty"`
}

// GeminiGenerationConfig controls generation parameters.
type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

// GeminiTool represents a tool definition.
type GeminiTool struct {
	FunctionDeclarations []GeminiFunctionDeclaration `json:"functionDeclarations,omitempty"`
}

// GeminiFunctionDeclaration represents a function declaration.
type GeminiFunctionDeclaration struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// GeminiResponse represents a response from the Gemini API.
type GeminiResponse struct {
	Candidates    []GeminiCandidate `json:"candidates"`
	UsageMetadata *GeminiUsage      `json:"usageMetadata,omitempty"`
}

// GeminiCandidate represents a response candidate.
type GeminiCandidate struct {
	Content      GeminiContent `json:"content"`
	FinishReason string        `json:"finishReason,omitempty"`
}

// GeminiUsage represents token usage in a Gemini response.
type GeminiUsage struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// GeminiStreamChunk represents a streaming chunk from the Gemini API.
type GeminiStreamChunk struct {
	Candidates    []GeminiCandidate `json:"candidates"`
	UsageMetadata *GeminiUsage      `json:"usageMetadata,omitempty"`
}
