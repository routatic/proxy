// Package core defines the provider abstraction, wire format types, and
// capability metadata that form the foundation of the routing engine.
package core

import (
	"context"
	"io"
	"time"

	"github.com/routatic/proxy/internal/config"
)

// WireFormat describes the upstream API format a provider uses for a given model.
type WireFormat int

const (
	// WireFormatOpenAIChat is the OpenAI Chat Completions format (/v1/chat/completions).
	WireFormatOpenAIChat WireFormat = iota
	// WireFormatAnthropic is the Anthropic Messages format (/v1/messages).
	WireFormatAnthropic
	// WireFormatOpenAIResponses is the OpenAI Responses format (/v1/responses).
	WireFormatOpenAIResponses
	// WireFormatGemini is the Google Gemini format (/v1/models/{id}).
	WireFormatGemini
)

// String returns a human-readable name for the wire format.
func (w WireFormat) String() string {
	switch w {
	case WireFormatOpenAIChat:
		return "openai"
	case WireFormatAnthropic:
		return "anthropic"
	case WireFormatOpenAIResponses:
		return "responses"
	case WireFormatGemini:
		return "gemini"
	default:
		return "unknown"
	}
}

// ProviderCapabilities describes what a provider can do at the provider level.
// Per-model refinements are returned by ModelCapabilities.
type ProviderCapabilities struct {
	SupportsStreaming  bool
	SupportsTools      bool
	SupportsThinking   bool
	SupportsImageInput bool
	MaxContextLength   int // in tokens
	DefaultMaxTokens   int
	KnownModels        []string
}

// ExecuteResult holds the result of a non-streaming provider call.
type ExecuteResult struct {
	Body    []byte
	ModelID string
	Latency time.Duration
}

// Provider is the abstraction for an upstream LLM provider.
type Provider interface {
	// Name returns the provider identifier (e.g. "opencode-go", "opencode-zen").
	Name() string

	// Capabilities returns provider-level capabilities.
	Capabilities() ProviderCapabilities

	// ModelCapabilities returns per-model capabilities. Returns false if the
	// model is unknown to this provider.
	ModelCapabilities(modelID string) (ProviderCapabilities, bool)

	// WireFormat returns the wire format for the given model on this provider.
	WireFormat(modelID string) WireFormat

	// Execute sends a non-streaming request and returns the response.
	Execute(ctx context.Context, req *NormalizedRequest, model config.ModelConfig) (*ExecuteResult, error)

	// Stream sends a streaming request and returns an io.ReadCloser for SSE
	// events. The stream emits raw SSE bytes; the handler is responsible for
	// forwarding them.
	Stream(ctx context.Context, req *NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error)

	// RoundTripName returns the model ID to use in the upstream request. This
	// may differ from the config's ModelID (e.g. for model overrides).
	RoundTripName(model config.ModelConfig) string

	// StreamIdleTimeout returns the maximum gap between bytes on an active
	// stream before it is treated as stuck and aborted.
	StreamIdleTimeout(model config.ModelConfig) time.Duration
}
