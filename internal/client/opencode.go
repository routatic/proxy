// Package client manages upstream API client connections.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

// OpenCodeClient handles communication with OpenCode Go API.
type OpenCodeClient struct {
	atomic     *config.AtomicConfig
	httpClient *http.Client
}

// NewOpenCodeClient creates a new OpenCode Go client.
func NewOpenCodeClient(atomic *config.AtomicConfig) *OpenCodeClient {
	cfg := atomic.Get()
	timeout := time.Duration(cfg.OpenCodeGo.TimeoutMs) * time.Millisecond
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	// Configure connection pooling for better performance
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
func IsAnthropicModel(modelID string) bool {
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7",
		"qwen3.5-plus", "qwen3.6-plus", "qwen3.7-max":
		return true
	default:
		return false
	}
}

// CleanAnthropicBody removes fields that are not compatible with the Anthropic
// /v1/messages endpoint:
//   - thinking field unless type is "enabled" (e.g. "disabled", "adaptive")
//   - top-level fields not recognized by Anthropic: context_management, output_config, metadata
//   - cache_control from message content blocks and system blocks
func CleanAnthropicBody(raw json.RawMessage) json.RawMessage {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return raw
	}

	dirty := false

	// Remove thinking unless type is "enabled".
	if thinking, ok := m["thinking"]; ok {
		var t struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(thinking, &t); err != nil || t.Type != "enabled" {
			delete(m, "thinking")
			dirty = true
		}
	}

	// Remove top-level fields not supported by Anthropic endpoint.
	for _, key := range []string{"context_management", "output_config", "metadata"} {
		if _, ok := m[key]; ok {
			delete(m, key)
			dirty = true
		}
	}

	// Strip $schema from tool input_schema (not supported by Anthropic endpoint).
	if tools, ok := m["tools"]; ok {
		var toolList []map[string]json.RawMessage
		if err := json.Unmarshal(tools, &toolList); err == nil {
			toolDirty := false
			for _, tool := range toolList {
				if schema, ok := tool["input_schema"]; ok {
					var s map[string]json.RawMessage
					if err := json.Unmarshal(schema, &s); err == nil {
						if _, ok := s["$schema"]; ok {
							delete(s, "$schema")
							newSchema, _ := json.Marshal(s)
							tool["input_schema"] = newSchema
							toolDirty = true
						}
					}
				}
			}
			if toolDirty {
				newTools, _ := json.Marshal(toolList)
				m["tools"] = newTools
				dirty = true
			}
		}
	}

	// Strip cache_control from message content blocks.
	if msgs, ok := m["messages"]; ok {
		var messages []map[string]json.RawMessage
		if err := json.Unmarshal(msgs, &messages); err == nil {
			msgDirty := false
			for _, msg := range messages {
				if content, ok := msg["content"]; ok {
					var blocks []map[string]json.RawMessage
					if err := json.Unmarshal(content, &blocks); err == nil {
						blockDirty := false
						for _, block := range blocks {
							if _, ok := block["cache_control"]; ok {
								delete(block, "cache_control")
								blockDirty = true
							}
						}
						if blockDirty {
							newContent, _ := json.Marshal(blocks)
							msg["content"] = newContent
							msgDirty = true
						}
					}
				}
			}
			if msgDirty {
				newMsgs, _ := json.Marshal(messages)
				m["messages"] = newMsgs
				dirty = true
			}
		}
	}

	// Strip cache_control from system blocks.
	if sys, ok := m["system"]; ok {
		var blocks []map[string]json.RawMessage
		if err := json.Unmarshal(sys, &blocks); err == nil {
			sysDirty := false
			for _, block := range blocks {
				if _, ok := block["cache_control"]; ok {
					delete(block, "cache_control")
					sysDirty = true
				}
			}
			if sysDirty {
				newSys, _ := json.Marshal(blocks)
				m["system"] = newSys
				dirty = true
			}
		}
	}

	if !dirty {
		return raw
	}

	cleaned, err := json.Marshal(m)
	if err != nil {
		return raw
	}
	return cleaned
}

// getEndpoint returns the appropriate endpoint config for a model.
func (c *OpenCodeClient) getEndpoint(modelID string) endpointConfig {
	cfg := c.atomic.Get()
	if IsAnthropicModel(modelID) {
		return endpointConfig{
			BaseURL: cfg.OpenCodeGo.AnthropicBaseURL,
			APIKey:  cfg.APIKey,
		}
	}
	return endpointConfig{
		BaseURL: cfg.OpenCodeGo.BaseURL,
		APIKey:  cfg.APIKey,
	}
}

// endpointConfig holds configuration for a specific API endpoint.
type endpointConfig struct {
	BaseURL string
	APIKey  string
}

// ChatCompletion sends a chat completion request to the OpenCode Go API.
// Returns the raw HTTP response for the caller to handle (streaming or body read).
func (c *OpenCodeClient) ChatCompletion(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
) (*http.Response, error) {
	endpoint := c.getEndpoint(modelID)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.BaseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+endpoint.APIKey)

	// Add streaming header if requested
	if req.Stream != nil && *req.Stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check for error status codes
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
) (*types.ChatCompletionResponse, error) {
	// Force non-streaming
	streamFalse := false
	req.Stream = &streamFalse

	resp, err := c.ChatCompletion(ctx, modelID, req)
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
// The caller is responsible for closing the returned ReadCloser.
func (c *OpenCodeClient) GetStreamingBody(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
) (io.ReadCloser, error) {
	// Force streaming
	streamTrue := true
	req.Stream = &streamTrue

	resp, err := c.ChatCompletion(ctx, modelID, req)
	if err != nil {
		return nil, err
	}

	return resp.Body, nil
}

// SendAnthropicRequest sends a raw Anthropic-format request (for MiniMax models).
// This skips the OpenAI transformation entirely.
func (c *OpenCodeClient) SendAnthropicRequest(
	ctx context.Context,
	body []byte,
	stream bool,
) (*http.Response, error) {
	cfg := c.atomic.Get()
	baseURL := cfg.OpenCodeGo.AnthropicBaseURL
	apiKey := cfg.APIKey

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	// Incase OpenCode Go expects x-api-key instead
	httpReq.Header.Set("x-api-key", apiKey)

	// Add streaming header if requested
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Check for error status codes
	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return resp, nil
}
