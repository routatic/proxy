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

// AWSBedrockProvider implements core.Provider for the AWS Bedrock Mantle backend.
// Bedrock Mantle exposes OpenAI-compatible and optionally Anthropic-compatible endpoints.
type AWSBedrockProvider struct {
	baseProvider
}

// NewAWSBedrockProvider creates a new AWSBedrockProvider.
func NewAWSBedrockProvider(atomic *config.AtomicConfig) *AWSBedrockProvider {
	return &AWSBedrockProvider{baseProvider: newBaseProvider(atomic)}
}

// Name returns the provider identifier.
func (p *AWSBedrockProvider) Name() string { return "aws-bedrock" }

// Capabilities returns provider-level capabilities.
func (p *AWSBedrockProvider) Capabilities() core.ProviderCapabilities {
	return core.ProviderCapabilities{
		SupportsStreaming:  true,
		SupportsTools:      true,
		SupportsThinking:   true,
		SupportsImageInput: true,
		MaxContextLength:   200_000,
		DefaultMaxTokens:   4096,
	}
}

// ModelCapabilities returns per-model capabilities. Returns true for all models
// since Bedrock hosts many different model families.
func (p *AWSBedrockProvider) ModelCapabilities(modelID string) (core.ProviderCapabilities, bool) {
	return p.Capabilities(), true
}

// WireFormat returns the wire format for Bedrock models. Defaults to OpenAI
// Chat Completions. Set wire_format: "anthropic" in aws_bedrock config for
// models that need raw Anthropic Messages format.
func (p *AWSBedrockProvider) WireFormat(modelID string) core.WireFormat {
	cfg := p.atomic.Get()
	if cfg != nil && cfg.AWSBedrock.WireFormat == "anthropic" {
		return core.WireFormatAnthropic
	}
	return core.WireFormatOpenAIChat
}

// RoundTripName returns the model ID to use in the upstream request.
func (p *AWSBedrockProvider) RoundTripName(model config.ModelConfig) string {
	return model.ModelID
}

// StreamIdleTimeout returns the maximum gap between bytes on an active stream.
func (p *AWSBedrockProvider) StreamIdleTimeout(model config.ModelConfig) time.Duration {
	const fallback = 5 * time.Minute
	cfg := p.atomic.Get()
	ms := cfg.AWSBedrock.StreamTimeoutMs
	if ms <= 0 {
		ms = cfg.AWSBedrock.TimeoutMs
	}
	if ms <= 0 {
		return fallback
	}
	return time.Duration(ms) * time.Millisecond
}

// Execute sends a non-streaming request and returns the response.
func (p *AWSBedrockProvider) Execute(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.executeAnthropic(ctx, req, model)
	default:
		return p.executeOpenAI(ctx, req, model)
	}
}

// Stream sends a streaming request and returns an io.ReadCloser for SSE events.
func (p *AWSBedrockProvider) Stream(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	switch p.WireFormat(model.ModelID) {
	case core.WireFormatAnthropic:
		return p.streamAnthropic(ctx, req, model)
	default:
		return p.streamOpenAI(ctx, req, model)
	}
}

// ── OpenAI Chat Completions ────────────────────────────────────────────

func (p *AWSBedrockProvider) executeOpenAI(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.AWSBedrock.BaseURL
	apiKey := p.bedrockAPIKey(cfg)

	openaiReq := transformer.TransformRequestFromNormalized(req, model)
	streamFalse := false
	openaiReq.Stream = &streamFalse

	start := time.Now()
	resp, err := p.doBedrockRequest(ctx, endpoint, apiKey, cfg.AWSBedrock.ProjectID, openaiReq, false)
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

func (p *AWSBedrockProvider) streamOpenAI(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.AWSBedrock.BaseURL
	apiKey := p.bedrockAPIKey(cfg)

	openaiReq := transformer.TransformRequestFromNormalized(req, model)
	streamTrue := true
	openaiReq.Stream = &streamTrue

	resp, err := p.doBedrockRequest(ctx, endpoint, apiKey, cfg.AWSBedrock.ProjectID, openaiReq, true)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// ── Anthropic Messages ────────────────────────────────────────────────

func (p *AWSBedrockProvider) executeAnthropic(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (*core.ExecuteResult, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.AWSBedrock.AnthropicBaseURL
	if endpoint == "" {
		return nil, fmt.Errorf("anthropic_base_url not configured for aws-bedrock provider")
	}
	apiKey := p.bedrockAPIKey(cfg)

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
	if cfg.AWSBedrock.ProjectID != "" {
		httpReq.Header.Set("OpenAI-Project", cfg.AWSBedrock.ProjectID)
	}

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

func (p *AWSBedrockProvider) streamAnthropic(ctx context.Context, req *core.NormalizedRequest, model config.ModelConfig) (io.ReadCloser, error) {
	cfg := p.atomic.Get()
	endpoint := cfg.AWSBedrock.AnthropicBaseURL
	if endpoint == "" {
		return nil, fmt.Errorf("anthropic_base_url not configured for aws-bedrock provider")
	}
	apiKey := p.bedrockAPIKey(cfg)

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
	if cfg.AWSBedrock.ProjectID != "" {
		httpReq.Header.Set("OpenAI-Project", cfg.AWSBedrock.ProjectID)
	}
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

// ── Helpers ────────────────────────────────────────────────────────────

// bedrockAPIKey returns the Bedrock-specific API key if configured, otherwise
// falls back to the global API key pool.
func (p *AWSBedrockProvider) bedrockAPIKey(cfg *config.Config) string {
	if cfg.AWSBedrock.APIKey != "" {
		return cfg.AWSBedrock.APIKey
	}
	return p.nextAPIKey(cfg.EffectiveAPIKeys())
}

// doBedrockRequest sends an HTTP request to the Bedrock Mantle endpoint with
// the OpenAI-Project header when configured.
func (p *AWSBedrockProvider) doBedrockRequest(ctx context.Context, endpoint, apiKey, projectID string, req any, stream bool) (*http.Response, error) {
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
	if projectID != "" {
		httpReq.Header.Set("OpenAI-Project", projectID)
	}
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
