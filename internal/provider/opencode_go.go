package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// OpenCodeGoProvider implements core.Provider for the OpenCode Go backend.
type OpenCodeGoProvider struct {
	baseProvider
}

// NewOpenCodeGoProvider creates a new OpenCodeGoProvider.
func NewOpenCodeGoProvider(atomic *config.AtomicConfig) *OpenCodeGoProvider {
	return &OpenCodeGoProvider{baseProvider: newBaseProvider(atomic)}
}

// Name returns the provider identifier.
func (p *OpenCodeGoProvider) Name() string { return "opencode-go" }

// Capabilities returns provider-level capabilities.
func (p *OpenCodeGoProvider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsStreaming:  true,
		SupportsTools:      true,
		SupportsThinking:   true,
		SupportsImageInput: true,
		MaxContextLength:   128_000,
		DefaultMaxTokens:   4096,
	}
}

// ModelCapabilities returns per-model capabilities. Returns false if unknown.
func (p *OpenCodeGoProvider) ModelCapabilities(modelID string) (core.ProviderCapabilities, bool) {
	caps := p.Capabilities()
	// qwen3.7-max has a larger context window on the Go provider.
	if modelID == "qwen3.7-max" {
		caps.MaxContextLength = 1_000_000
	}
	// MiniMax models support 1M context.
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7", "minimax-m3":
		caps.MaxContextLength = 1_000_000
	}
	return caps, true
}

// WireFormat returns the wire format for the given model on the Go provider.
func (p *OpenCodeGoProvider) WireFormat(modelID string) core.WireFormat {
	if isAnthropicNativeGo(modelID) {
		return core.WireFormatAnthropic
	}
	return core.WireFormatOpenAIChat
}

func isAnthropicNativeGo(modelID string) bool {
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7", "minimax-m3",
		"qwen3.5-plus", "qwen3.6-plus", "qwen3.7-plus", "qwen3.7-max":
		return true
	default:
		return false
	}
}

// RoundTripName returns the model ID to use in the upstream request.
func (p *OpenCodeGoProvider) RoundTripName(model config.ModelConfig) string {
	return model.ModelID
}

// StreamIdleTimeout returns the maximum gap between bytes on an active stream.
func (p *OpenCodeGoProvider) StreamIdleTimeout(model config.ModelConfig) time.Duration {
	const fallback = 5 * time.Minute
	cfg := p.atomic.Get()
	ms := cfg.OpenCodeGo.StreamTimeoutMs
	if ms <= 0 {
		ms = cfg.OpenCodeGo.TimeoutMs
	}
	if ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

// Execute sends a non-streaming request and returns the response.
func (p *OpenCodeGoProvider) Execute(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.executeAnthropic(ctx, req, model)
	default:
		return p.executeOpenAI(ctx, req, model)
	}
}

// Stream sends a streaming request and returns an io.ReadCloser for SSE events.
func (p *OpenCodeGoProvider) Stream(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.streamAnthropic(ctx, req, model)
	default:
		return p.streamOpenAI(ctx, req, model)
	}
}

// ── OpenAI Chat Completions ────────────────────────────────────────────

func (p *OpenCodeGoProvider) executeOpenAI(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.BaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	openaiReq := transformer.TransformRequestFromNormalized(req, model)
	streamFalse := false
	openaiReq.Stream = &streamFalse

	start := time.Now()
	resp, err := p.doRequest(ctx, endpoint, apiKey, openaiReq, false)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var chatResp types.ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	normResp := transformer.OpenAIResponseToNormalized(&chatResp, model.ModelID)
	anthropicResp := core.DenormalizeResponse(normResp)
	resultBody, err := json.Marshal(anthropicResp)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal response: %w", err)
	}

	return &core.ExecuteResult{
		Body:    resultBody,
		ModelID: model.ModelID,
		Latency: time.Since(start),
	}, nil
}

func (p *OpenCodeGoProvider) streamOpenAI(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.BaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	openaiReq := transformer.TransformRequestFromNormalized(req, model)
	streamTrue := true
	openaiReq.Stream = &streamTrue

	resp, err := p.doRequest(ctx, endpoint, apiKey, openaiReq, true)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── Anthropic Messages ────────────────────────────────────────────────

func (p *OpenCodeGoProvider) executeAnthropic(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.AnthropicBaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	anthropicReq := transformer.NormalizedToAnthropic(req, model)
	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("x-api-key", apiKey)

	start := time.Now()
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return &core.ExecuteResult{
		Body:    body,
		ModelID: model.ModelID,
		Latency: time.Since(start),
	}, nil
}

func (p *OpenCodeGoProvider) streamAnthropic(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeGo.AnthropicBaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	anthropicReq := transformer.NormalizedToAnthropic(req, model)
	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(rawBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("x-api-key", apiKey)
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	return resp.Body, nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────

func (p *OpenCodeGoProvider) doRequest(ctx context.Context, endpoint, apiKey string, req any, stream bool) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, &client.APIError{StatusCode: resp.StatusCode, Body: string(bodyBytes)}
	}

	return resp, nil
}
