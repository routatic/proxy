// Package client manages upstream API client connections.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"oc-go-cc/internal/config"
	"oc-go-cc/pkg/types"
)

// keyCooldownDuration is how long a key stays "cold" after a 429.
// Picked to outlast the typical OpenCode Go per-minute rate window without
// permanently shelving a key that recovers within the minute.
const keyCooldownDuration = 60 * time.Second

// OpenCodeClient handles communication with OpenCode Go API.
type OpenCodeClient struct {
	atomic     *config.AtomicConfig
	httpClient *http.Client

	// keyNextIdx is a lock-free round-robin counter (atomic.Uint64).
	// It is incremented once per pickKey call regardless of success.
	keyNextIdx atomic.Uint64

	// keyCooldown tracks per-key rate-limit cooldowns. Mutex guards the map.
	keyMu       sync.Mutex
	keyCooldown map[string]time.Time
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
		keyCooldown: make(map[string]time.Time),
	}
}

// configuredKeys returns the active api_keys list, preferring APIKeys but
// falling back to APIKey for backward compat. Always returns at least one
// entry when the config validated.
func (c *OpenCodeClient) configuredKeys() []string {
	cfg := c.atomic.Get()
	if len(cfg.APIKeys) > 0 {
		return cfg.APIKeys
	}
	if cfg.APIKey != "" {
		return []string{cfg.APIKey}
	}
	return nil
}

// pickKey returns the next non-cold key from the rotation, or empty when all
// keys are in cooldown. Round-robin across configured keys.
//
// Lock-free for the index: atomic.Uint64 guarantees each concurrent caller
// gets a unique starting position, eliminating mutex contention on the hot path.
// The mutex is held only while inspecting / mutating the cooldown map.
func (c *OpenCodeClient) pickKey() string {
	keys := c.configuredKeys()
	if len(keys) == 0 {
		return ""
	}

	// Atomically claim a round-robin slot (wrap-around safe on 64-bit).
	startIdx := int(c.keyNextIdx.Add(1)-1) % len(keys)
	if startIdx < 0 {
		startIdx += len(keys)
	}

	c.keyMu.Lock()
	defer c.keyMu.Unlock()

	now := time.Now()

	// GC expired entries while we have the lock.
	for k, until := range c.keyCooldown {
		if now.After(until) {
			delete(c.keyCooldown, k)
		}
	}

	for attempt := 0; attempt < len(keys); attempt++ {
		idx := (startIdx + attempt) % len(keys)
		key := keys[idx]
		if until, cold := c.keyCooldown[key]; !cold || now.After(until) {
			return key
		}
	}
	return ""
}

// markKeyCold parks a key for keyCooldownDuration so subsequent picks skip it.
func (c *OpenCodeClient) markKeyCold(key string) {
	if key == "" {
		return
	}
	c.keyMu.Lock()
	defer c.keyMu.Unlock()
	c.keyCooldown[key] = time.Now().Add(keyCooldownDuration)
}

// IsAnthropicModel returns true if the model requires the Anthropic endpoint.
func IsAnthropicModel(modelID string) bool {
	switch modelID {
	case "minimax-m2.5", "minimax-m2.7":
		return true
	default:
		return false
	}
}

// baseURLFor returns the upstream URL appropriate for the model's endpoint family.
func (c *OpenCodeClient) baseURLFor(modelID string) string {
	cfg := c.atomic.Get()
	if IsAnthropicModel(modelID) {
		return cfg.OpenCodeGo.AnthropicBaseURL
	}
	return cfg.OpenCodeGo.BaseURL
}

// ChatCompletion sends a chat completion request to the OpenCode Go API.
// On HTTP 429 (rate limit) it marks the current api_key cold and retries with
// the next non-cold key. Returns the upstream 429 only after every configured
// key has been tried.
func (c *OpenCodeClient) ChatCompletion(
	ctx context.Context,
	modelID string,
	req *types.ChatCompletionRequest,
) (*http.Response, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	baseURL := c.baseURLFor(modelID)
	keyCount := len(c.configuredKeys())
	if keyCount == 0 {
		return nil, fmt.Errorf("no api_keys configured")
	}

	var lastErr error
	for attempt := 0; attempt < keyCount; attempt++ {
		key := c.pickKey()
		if key == "" {
			break
		}

		resp, statusCode, attemptErr := c.doRequest(ctx, baseURL, key, body, req.Stream != nil && *req.Stream, false)
		if attemptErr == nil {
			return resp, nil
		}
		lastErr = attemptErr
		if statusCode == http.StatusTooManyRequests {
			c.markKeyCold(key)
			continue
		}
		return nil, attemptErr
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all api_keys exhausted (rate limited)")
	}
	return nil, lastErr
}

// doRequest performs a single attempt against `baseURL` with the supplied key.
// On success returns the live response. On HTTP >= 400 returns (nil, statusCode, err)
// so the caller can decide whether to retry with another key (429) or fail fast.
// The `anthropicHeaders` flag mirrors the dual auth scheme used by
// SendAnthropicRequest.
func (c *OpenCodeClient) doRequest(
	ctx context.Context,
	baseURL string,
	apiKey string,
	body []byte,
	streaming bool,
	anthropicHeaders bool,
) (*http.Response, int, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL, bytes.NewReader(body))
	if err != nil {
		return nil, 0, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	if anthropicHeaders {
		// Some OpenCode Go endpoints expect x-api-key for Anthropic-format requests.
		httpReq.Header.Set("x-api-key", apiKey)
	}
	if streaming {
		httpReq.Header.Set("Accept", "text/event-stream")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}

	if resp.StatusCode >= http.StatusBadRequest {
		bodyBytes, _ := io.ReadAll(resp.Body)
		retryAfter := resp.Header.Get("Retry-After")
		_ = resp.Body.Close()
		// Structured error so the fallback loop + handler can propagate
		// quota-exhaustion (429 + Retry-After) instead of flattening it
		// into a generic 502. See pkg/types.UpstreamError.
		return nil, resp.StatusCode, &types.UpstreamError{
			StatusCode: resp.StatusCode,
			RetryAfter: retryAfter,
			Body:       string(bodyBytes),
		}
	}

	return resp, resp.StatusCode, nil
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
// This skips the OpenAI transformation entirely. Honors the same per-key
// rate-limit rotation as ChatCompletion.
func (c *OpenCodeClient) SendAnthropicRequest(
	ctx context.Context,
	body []byte,
	stream bool,
) (*http.Response, error) {
	cfg := c.atomic.Get()
	baseURL := cfg.OpenCodeGo.AnthropicBaseURL
	keyCount := len(c.configuredKeys())
	if keyCount == 0 {
		return nil, fmt.Errorf("no api_keys configured")
	}

	var lastErr error
	for attempt := 0; attempt < keyCount; attempt++ {
		key := c.pickKey()
		if key == "" {
			break
		}

		resp, statusCode, attemptErr := c.doRequest(ctx, baseURL, key, body, stream, true)
		if attemptErr == nil {
			return resp, nil
		}
		lastErr = attemptErr
		if statusCode == http.StatusTooManyRequests {
			c.markKeyCold(key)
			continue
		}
		return nil, attemptErr
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("all api_keys exhausted (rate limited)")
	}
	return nil, lastErr
}
