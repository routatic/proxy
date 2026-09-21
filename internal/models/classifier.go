// Package models provides model classification utilities shared across
// the client and provider packages. It consolidates endpoint type detection
// and model-specific classification logic to avoid duplication.
package models

import "strings"

// EndpointType determines which API endpoint format to use for a model.
type EndpointType int

const (
	// EndpointChatCompletions is the OpenAI-compatible /v1/chat/completions endpoint.
	EndpointChatCompletions EndpointType = iota
	// EndpointAnthropic is the Anthropic /v1/messages endpoint.
	EndpointAnthropic
	// EndpointResponses is the OpenAI native /v1/responses endpoint.
	EndpointResponses
	// EndpointGemini is the Google Gemini /v1/models/{id} endpoint.
	EndpointGemini
)

// ClassifyEndpoint determines the endpoint type for a model on Zen.
// This is Zen-specific: minimax models use chat completions on Zen
// (they use Anthropic only on the Go provider).
func ClassifyEndpoint(modelID string) EndpointType {
	switch {
	case IsZenAnthropicModel(modelID):
		return EndpointAnthropic
	case IsGeminiModel(modelID):
		return EndpointGemini
	case IsResponsesModel(modelID):
		return EndpointResponses
	default:
		return EndpointChatCompletions
	}
}

// IsAnthropicModel returns true if the Go provider model requires the Anthropic endpoint.
// Most Go provider models use the Chat Completions transform path for broader
// compatibility (tool format, message roles, etc.). Exceptions are models whose
// upstream backends don't support the OpenAI Chat Completions format and only
// accept Anthropic Messages format.
//
// Only Zen models use the raw Anthropic endpoint via ClassifyEndpoint.
func IsAnthropicModel(modelID string) bool {
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7", "minimax-m3",
		"qwen3.5-plus", "qwen3.6-plus", "qwen3.7-plus", "qwen3.7-max":
		return true
	default:
		return false
	}
}

// IsZenAnthropicModel returns true for models on Zen that use the Anthropic endpoint.
func IsZenAnthropicModel(modelID string) bool {
	// Claude models on Zen use the Anthropic endpoint
	if strings.HasPrefix(modelID, "claude-") {
		return true
	}
	// Qwen models on Zen use the Anthropic endpoint
	if strings.HasPrefix(modelID, "qwen") {
		return true
	}
	return false
}

// IsGeminiModel returns true for models using the Gemini endpoint.
// Uses prefix check to cover all current and future Gemini models.
func IsGeminiModel(modelID string) bool {
	return strings.HasPrefix(modelID, "gemini-")
}

// IsResponsesModel returns true for models using the OpenAI Responses endpoint.
// Uses prefix checks to cover all current and future model families.
func IsResponsesModel(modelID string) bool {
	return strings.HasPrefix(modelID, "gpt-") ||
		strings.HasPrefix(modelID, "grok-") ||
		strings.HasPrefix(modelID, "muse-spark-")
}
