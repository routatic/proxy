// Package client manages upstream API client connections.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

const (
	ProviderOpenCodeGo  = "opencode-go"
	ProviderOpenCodeZen = "opencode-zen"
)

// OpenCodeClient handles communication with OpenCode Go and Zen APIs.
type OpenCodeClient struct {
	atomic     *config.AtomicConfig
	httpClient *http.Client
}

// NewOpenCodeClient creates a new OpenCode client.
func NewOpenCodeClient(atomic *config.AtomicConfig) *OpenCodeClient {
	cfg := atomic.Get()
	timeout := time.Duration(cfg.OpenCodeGo.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 20,
		IdleConnTimeout:     90 * time.Second,
		MaxConnsPerHost:     50,
		DisableKeepAlives:   false,
	}

	return &OpenCodeClient{
		atomic: atomic,
		httpClient: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}
}

// IsAnthropicModel returns true if the model requires the Anthropic endpoint.
// This includes both Go models (minimax) and Zen models (claude, qwen).
func IsAnthropicModel(modelID string) bool {
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7", "qwen3.7-max":
		return true
	default:
		return isZenAnthropicModel(modelID)
	}
}

// isZenAnthropicModel returns true for models on Zen that use the Anthropic endpoint.
func isZenAnthropicModel(modelID string) bool {
	// Qwen models on Zen use the Anthropic endpoint
	if strings.HasPrefix(modelID, "qwen") {
		return true
	}
	return false
}

// Provider returns the provider string for a model config.
// Defaults to ProviderOpenCodeGo if empty.
func Provider(model config.ModelConfig) string {
	if model.Provider != "" {
		return model.Provider
	}
	return ProviderOpenCodeGo
}

// IsZen returns true if the model uses the OpenCode Zen provider.
func IsZen(model config.ModelConfig) bool {
	return Provider(model) == ProviderOpenCodeZen
}

// EndpointType determines which Zen endpoint format to use.
type EndpointType int

const (
	EndpointChatCompletions EndpointType = iota // /v1/chat/completions (OpenAI-compatible)
	EndpointAnthropic                           // /v1/messages (Anthropic format)
	EndpointResponses                           // /v1/responses (OpenAI native)
	EndpointGemini                              // /v1/models/{id} (Google Gemini)
)

// ClassifyEndpoint determines the endpoint type for a model on Zen.
func ClassifyEndpoint(modelID string) EndpointType {
	switch {
	case IsAnthropicModel(modelID):
		return EndpointAnthropic
	case isGeminiModel(modelID):
		return EndpointGemini
	case isResponsesModel(modelID):
		return EndpointResponses
	default:
		return EndpointChatCompletions
	}
}

func isGeminiModel(modelID string) bool {
	switch modelID {
	case "gemini-3.5-flash", "gemini-3.1-pro", "gemini-3-flash":
		return true
	default:
		return false
	}
}

func isResponsesModel(modelID string) bool {
	switch modelID {
	case "gpt-5.5", "gpt-5.5-pro", "gpt-5.4", "gpt-5.4-pro", "gpt-5.4-mini", "gpt-5.4-nano",
		"gpt-5.3-codex", "gpt-5.3-codex-spark", "gpt-5.2", "gpt-5.2-codex",
		"gpt-5.1", "gpt-5.1-codex", "gpt-5.1-codex-max", "gpt-5.1-codex-mini",
		"gpt-5", "gpt-5-codex", "gpt-5-nano":
		return true
	default:
		return false
	}
}

// getEndpoint returns the appropriate endpoint config for a model.
func (c *OpenCodeClient) getEndpoint(modelID string, modelConfig config.ModelConfig) endpointConfig {
	cfg := c.atomic.Get()

	if IsZen(modelConfig) {
		zen := cfg.OpenCodeZen
		switch ClassifyEndpoint(modelID) {
		case EndpointAnthropic:
			return endpointConfig{BaseURL: zen.AnthropicBaseURL, APIKey: cfg.APIKey}
		case EndpointResponses:
			return endpointConfig{BaseURL: zen.ResponsesBaseURL, APIKey: cfg.APIKey}
		case EndpointGemini:
			return endpointConfig{BaseURL: zen.GeminiBaseURL + "/" + modelID, APIKey: cfg.APIKey}
		default:
			return endpointConfig{BaseURL: zen.BaseURL, APIKey: cfg.APIKey}
		}
	}

	// Default: OpenCode Go
	if IsAnthropicModel(modelID) {
		return endpointConfig{BaseURL: cfg.OpenCodeGo.AnthropicBaseURL, APIKey: cfg.APIKey}
	}
	return endpointConfig{BaseURL: cfg.OpenCodeGo.BaseURL, APIKey: cfg.APIKey}
}

// endpointConfig holds configuration for a specific API endpoint.
type endpointConfig struct {
	BaseURL string
	APIKey  string
}

// ChatCompletion sends a chat completion request.
func (c *OpenCodeClient) ChatCompletion(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) (*http.Response, error) {
	endpoint := c.getEndpoint(modelID, modelConfig)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	// Anthropic endpoint uses x-api-key; OpenAI endpoint uses Bearer
	if IsAnthropicModel(modelID) {
		httpReq.Header.Set("x-api-key", endpoint.APIKey)
	} else {
		httpReq.Header.Set("Authorization", "Bearer "+endpoint.APIKey)
	}

	if req.Stream != nil && *req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// ChatCompletionNonStreaming sends a non-streaming request and returns the full parsed response.
func (c *OpenCodeClient) ChatCompletionNonStreaming(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) (*types.ChatCompletionResponse, error) {
	streamFalse := false
	req.Stream = &streamFalse

	resp, err := c.ChatCompletion(ctx, modelID, req, modelConfig)
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

	return &chatResp, nil
}

// GetStreamingBody returns the response body for streaming consumption.
func (c *OpenCodeClient) GetStreamingBody(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
	modelConfig config.ModelConfig,
) (io.ReadCloser, error) {
	streamTrue := true
	req.Stream = &streamTrue

	resp, err := c.ChatCompletion(ctx, modelID, req, modelConfig)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// SendAnthropicRequest sends a raw Anthropic-format request.
func (c *OpenCodeClient) SendAnthropicRequest(
	ctx context.Context,
	body []byte,
	stream bool,
	modelConfig config.ModelConfig,
) (*http.Response, error) {
	cfg := c.atomic.Get()
	var baseURL string

	if IsZen(modelConfig) {
		baseURL = cfg.OpenCodeZen.AnthropicBaseURL
	} else {
		baseURL = cfg.OpenCodeGo.AnthropicBaseURL
	}
	apiKey := cfg.APIKey

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("x-api-key", apiKey)

	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// ResponsesCompletion sends a request to the OpenAI Responses endpoint.
func (c *OpenCodeClient) ResponsesCompletion(
	ctx context.Context,
	modelID string,
	req *types.ResponsesRequest,
	modelConfig config.ModelConfig,
) (*http.Response, error) {
	endpoint := c.getEndpoint(modelID, modelConfig)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+endpoint.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// ResponsesCompletionNonStreaming sends a non-streaming Responses request.
func (c *OpenCodeClient) ResponsesCompletionNonStreaming(
	ctx context.Context,
	modelID string,
	req *types.ResponsesRequest,
	modelConfig config.ModelConfig,
) (*types.ResponsesResponse, error) {
	req.Stream = false

	resp, err := c.ResponsesCompletion(ctx, modelID, req, modelConfig)
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

	return &responsesResp, nil
}

// GetResponsesStreamingBody returns the response body for Responses streaming.
func (c *OpenCodeClient) GetResponsesStreamingBody(
	ctx context.Context,
	modelID string,
	req *types.ResponsesRequest,
	modelConfig config.ModelConfig,
) (io.ReadCloser, error) {
	req.Stream = true

	resp, err := c.ResponsesCompletion(ctx, modelID, req, modelConfig)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// GeminiCompletion sends a request to the Gemini endpoint.
func (c *OpenCodeClient) GeminiCompletion(
	ctx context.Context,
	modelID string,
	req *types.GeminiRequest,
	modelConfig config.ModelConfig,
) (*http.Response, error) {
	endpoint := c.getEndpoint(modelID, modelConfig)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+endpoint.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}

// GeminiCompletionNonStreaming sends a non-streaming Gemini request.
func (c *OpenCodeClient) GeminiCompletionNonStreaming(
	ctx context.Context,
	modelID string,
	req *types.GeminiRequest,
	modelConfig config.ModelConfig,
) (*types.GeminiResponse, error) {
	req.Stream = false

	resp, err := c.GeminiCompletion(ctx, modelID, req, modelConfig)
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

	return &geminiResp, nil
}

// GetGeminiStreamingBody returns the response body for Gemini streaming.
func (c *OpenCodeClient) GetGeminiStreamingBody(
	ctx context.Context,
	modelID string,
	req *types.GeminiRequest,
	modelConfig config.ModelConfig,
) (io.ReadCloser, error) {
	req.Stream = true

	resp, err := c.GeminiCompletion(ctx, modelID, req, modelConfig)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}
