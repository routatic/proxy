package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/models"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// OpenCodeZenProvider implements core.Provider for the OpenCode Zen backend.
// Zen supports four wire formats determined by model ID: Anthropic (Claude,
// Qwen), Responses (GPT), Gemini, and Chat Completions (everything else).
type OpenCodeZenProvider struct {
	baseProvider
}

// NewOpenCodeZenProvider creates a new OpenCodeZenProvider.
func NewOpenCodeZenProvider(atomic *config.AtomicConfig) *OpenCodeZenProvider {
	return &OpenCodeZenProvider{baseProvider: newBaseProvider(atomic)}
}

// Name returns the provider identifier.
func (p *OpenCodeZenProvider) Name() string { return "opencode-zen" }

// Capabilities returns provider-level capabilities.
func (p *OpenCodeZenProvider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsStreaming:  true,
		SupportsTools:      true,
		SupportsThinking:   true,
		SupportsImageInput: true,
		MaxContextLength:   200_000,
		DefaultMaxTokens:   4096,
	}
}

// ModelCapabilities returns per-model capabilities.
func (p *OpenCodeZenProvider) ModelCapabilities(modelID string) (core.ProviderCapabilities, bool) {
	caps := p.Capabilities()
	switch {
	case strings.HasPrefix(modelID, "claude-"):
		caps.MaxContextLength = 200_000
	case strings.HasPrefix(modelID, "gemini-"):
		caps.MaxContextLength = 1_000_000
	case strings.HasPrefix(modelID, "gpt-"):
		caps.MaxContextLength = 128_000
		caps.SupportsThinking = false
	case strings.HasPrefix(modelID, "minimax-"):
		caps.MaxContextLength = 1_000_000
	case strings.HasPrefix(modelID, "deepseek-"):
		caps.MaxContextLength = 1_000_000
	}
	return caps, true
}

// WireFormat returns the wire format for the given model on Zen.
// This replaces the old client.ClassifyEndpoint function.
func (p *OpenCodeZenProvider) WireFormat(modelID string) core.WireFormat {
	switch models.ClassifyEndpoint(modelID) {
	case models.EndpointAnthropic:
		return core.WireFormatAnthropic
	case models.EndpointGemini:
		return core.WireFormatGemini
	case models.EndpointResponses:
		return core.WireFormatOpenAIResponses
	default:
		return core.WireFormatOpenAIChat
	}
}

// RoundTripName returns the model ID to use in the upstream request.
func (p *OpenCodeZenProvider) RoundTripName(model config.ModelConfig) string {
	return model.ModelID
}

// StreamIdleTimeout returns the maximum gap between bytes on an active stream.
func (p *OpenCodeZenProvider) StreamIdleTimeout(model config.ModelConfig) time.Duration {
	const fallback = 5 * time.Minute
	cfg := p.atomic.Get()
	ms := cfg.OpenCodeZen.StreamTimeoutMs
	if ms <= 0 {
		ms = cfg.OpenCodeZen.TimeoutMs
	}
	if ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

// Execute sends a non-streaming request and returns the response.
func (p *OpenCodeZenProvider) Execute(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.executeAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.executeResponses(ctx, req, model)
	case core.WireFormatGemini:
		return p.executeGemini(ctx, req, model)
	default:
		return p.executeOpenAI(ctx, req, model)
	}
}

// Stream sends a streaming request and returns an io.ReadCloser for SSE events.
func (p *OpenCodeZenProvider) Stream(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.streamAnthropic(ctx, req, model)
	case core.WireFormatOpenAIResponses:
		return p.streamResponses(ctx, req, model)
	case core.WireFormatGemini:
		return p.streamGemini(ctx, req, model)
	default:
		return p.streamOpenAI(ctx, req, model)
	}
}

// ── OpenAI Chat Completions ────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeOpenAI(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.BaseURL
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

func (p *OpenCodeZenProvider) streamOpenAI(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.BaseURL
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

func (p *OpenCodeZenProvider) executeAnthropic(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.AnthropicBaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	anthropicReq := transformer.NormalizedToAnthropic(req, model)
	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
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

func (p *OpenCodeZenProvider) streamAnthropic(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.AnthropicBaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	anthropicReq := transformer.NormalizedToAnthropic(req, model)
	rawBody, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
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

// ── OpenAI Responses ──────────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeResponses(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.ResponsesBaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	responsesReq := transformer.NormalizedToResponses(req, model)
	responsesReq.Stream = false

	start := time.Now()
	resp, err := p.doJSONRequest(ctx, endpoint, apiKey, responsesReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var responsesResp types.ResponsesResponse
	if err := json.Unmarshal(body, &responsesResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	normResp := transformer.ResponsesToNormalized(&responsesResp, model.ModelID)
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

func (p *OpenCodeZenProvider) streamResponses(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.ResponsesBaseURL
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	responsesReq := transformer.NormalizedToResponses(req, model)
	responsesReq.Stream = true

	resp, err := p.doJSONRequest(ctx, endpoint, apiKey, responsesReq)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── Gemini ────────────────────────────────────────────────────────────

func (p *OpenCodeZenProvider) executeGemini(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.GeminiBaseURL + "/" + model.ModelID
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	geminiReq := transformer.NormalizedToGemini(req, model)
	geminiReq.Stream = false

	start := time.Now()
	resp, err := p.doJSONRequest(ctx, endpoint, apiKey, geminiReq)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	var geminiResp types.GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	normResp := transformer.GeminiToNormalized(&geminiResp, model.ModelID)
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

func (p *OpenCodeZenProvider) streamGemini(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.OpenCodeZen.GeminiBaseURL + "/" + model.ModelID
	apiKey := p.nextAPIKey(cfg.EffectiveAPIKeys())

	geminiReq := transformer.NormalizedToGemini(req, model)
	geminiReq.Stream = true

	resp, err := p.doJSONRequest(ctx, endpoint, apiKey, geminiReq)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── HTTP helpers ──────────────────────────────────────────────────────

func (p *OpenCodeZenProvider) doRequest(ctx context.Context, endpoint, apiKey string, req any, stream bool) (*http.Response, error) {
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

func (p *OpenCodeZenProvider) doJSONRequest(ctx context.Context, endpoint, apiKey string, req any) (*http.Response, error) {
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
