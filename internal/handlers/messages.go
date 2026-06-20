// Package handlers contains HTTP request handlers for API endpoints.
package handlers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
	"github.com/routatic/proxy/internal/core"
	"github.com/routatic/proxy/internal/metrics"
	"github.com/routatic/proxy/internal/middleware"
	"github.com/routatic/proxy/internal/router"
	"github.com/routatic/proxy/internal/token"
	"github.com/routatic/proxy/internal/transformer"
	"github.com/routatic/proxy/pkg/types"
)

// MessagesHandler handles /v1/messages requests.
type MessagesHandler struct {
	client              *client.OpenCodeClient // kept for backward compat during migration
	providerRegistry    *core.ProviderRegistry // new: provider dispatch
	modelRouter         *router.ModelRouter
	fallbackHandler     *router.FallbackHandler
	streamProxy         *StreamProxy // new: SSE proxy by wire format
	requestTransformer  *transformer.RequestTransformer
	responseTransformer *transformer.ResponseTransformer
	streamHandler       *transformer.StreamHandler
	tokenCounter        *token.Counter
	logger              *slog.Logger
	rateLimiter         *middleware.RateLimiter
	requestDedup        *middleware.RequestDeduplicator
	requestIDGen        *middleware.RequestIDGenerator
	metrics             *metrics.Metrics
}

// responseWriter wraps http.ResponseWriter to track if headers were written.
// It is safe for concurrent use: Write, WriteHeader, and Flush are serialized
// via an internal mutex so that concurrent goroutines (e.g. heartbeat and
// stream proxy) don't interleave SSE frames.
type responseWriter struct {
	http.ResponseWriter
	mu                sync.Mutex
	wroteHeader       bool
	ssePayloadWritten bool
}

func (w *responseWriter) WriteHeader(code int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(code)
	}
}

func (w *responseWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader {
		w.wroteHeader = true
		w.ResponseWriter.WriteHeader(http.StatusOK)
	}
	if len(b) > 0 {
		w.ssePayloadWritten = true
	}
	return w.ResponseWriter.Write(b)
}

// headerWritten returns true if headers have been written to the response.
// Safe for concurrent use.
func (w *responseWriter) headerWritten() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.wroteHeader
}

// Flush implements http.Flusher for SSE streaming support.
// The mutex is held across the flush call to ensure Write, WriteHeader, and
// Flush remain serialized. Without this, a concurrent Flush and Write on the
// underlying http.ResponseWriter's *bufio.Writer would be a data race.
func (w *responseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// WriteKeepalive writes a keepalive comment frame (":keepalive\n\n") to the
// response. Unlike Write, it does NOT set ssePayloadWritten — keepalives are
// not real SSE events and should not block fallback logic on idle timeout.
func (w *responseWriter) WriteKeepalive() {
	w.mu.Lock()
	defer w.mu.Unlock()
	_, _ = fmt.Fprintf(w.ResponseWriter, ":keepalive\n\n")
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// NewMessagesHandler creates a new messages handler.
func NewMessagesHandler(
	openCodeClient *client.OpenCodeClient,
	providerRegistry *core.ProviderRegistry,
	modelRouter *router.ModelRouter,
	fallbackHandler *router.FallbackHandler,
	tokenCounter *token.Counter,
	metrics *metrics.Metrics,
) *MessagesHandler {
	return &MessagesHandler{
		client:              openCodeClient,
		providerRegistry:    providerRegistry,
		modelRouter:         modelRouter,
		fallbackHandler:     fallbackHandler,
		streamProxy:         NewStreamProxy(),
		requestTransformer:  transformer.NewRequestTransformer(),
		responseTransformer: transformer.NewResponseTransformer(),
		streamHandler:       transformer.NewStreamHandler(),
		tokenCounter:        tokenCounter,
		logger:              slog.Default(),
		rateLimiter:         middleware.NewRateLimiter(100, time.Minute),
		requestDedup:        nil,
		requestIDGen:        middleware.NewRequestIDGenerator(),
		metrics:             metrics,
	}
}

// HandleMessages handles POST /v1/messages.
func (h *MessagesHandler) HandleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Generate or get request ID for correlation.
	// Cap externally-provided IDs at 256 bytes to prevent header abuse.
	requestID := r.Header.Get("X-Request-ID")
	if len(requestID) > 256 {
		requestID = requestID[:256]
	}
	if requestID == "" {
		requestID = h.requestIDGen.Generate()
	}
	w.Header().Set("X-Request-ID", requestID)

	// Rate limiting
	clientIP := middleware.GetClientIP(r)
	if !h.rateLimiter.Allow(clientIP) {
		h.metrics.RecordRateLimited()
		h.logger.Warn("rate limited", "client", clientIP, "request_id", requestID)
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	// Read the raw request body with a size limit to prevent memory exhaustion.
	const maxBodySize = 104857600 // 100 MB
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	var rawBody json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			h.sendError(w, http.StatusRequestEntityTooLarge, "request body too large", err)
			return
		}
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Deduplicate - skip duplicate requests. Skip when the deduplicator is
	// not configured (nil requestDedup) — it is an optional component.
	if h.requestDedup != nil {
		if _, ok := h.requestDedup.TryAcquire(rawBody); !ok {
			h.metrics.RecordDeduplicated()
			h.logger.Info("duplicate request skipped", "request_id", requestID)
			return
		}
	}

	// Parse into Anthropic request
	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	// Validate request
	if err := anthropicReq.Validate(); err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	// Record metrics
	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
	h.metrics.RecordRequest(isStreaming)

	h.logger.Info("received request",
		"model", anthropicReq.Model,
		"streaming", isStreaming,
		"messages", len(anthropicReq.Messages),
		"tools", len(anthropicReq.Tools),
		"max_tokens", anthropicReq.MaxTokens,
	)

	// Build message content for routing and token counting.
	var routerMessages []router.MessageContent
	var tokenMessages []token.MessageContent
	systemText := anthropicReq.SystemText()

	for _, msg := range anthropicReq.Messages {
		blocks := msg.ContentBlocks()
		content := extractTextFromBlocks(blocks)
		mc := router.MessageContent{
			Role:        msg.Role,
			Content:     content,
			HasImage:    blocksHaveImage(blocks),
			ImageHashes: imageHashesFromBlocks(blocks),
		}
		routerMessages = append(routerMessages, mc)
		tokenMessages = append(tokenMessages, token.MessageContent{
			Role:        msg.Role,
			Content:     content,
			ExtraTokens: imageTokenEstimate(blocks),
		})
	}

	// Count tokens.
	tokenCount, err := h.tokenCounter.CountMessages(systemText, tokenMessages)
	if err != nil {
		h.logger.Warn("failed to count tokens", "error", err)
		tokenCount = 0
	}

	// Route to appropriate model and build fallback chain.
	facts := router.AnalyzeRequestFacts(routerMessages)
	needsTools := len(anthropicReq.Tools) > 0
	modelChain, routeResult, err := h.buildModelChain(anthropicReq.Model, routerMessages, tokenCount, isStreaming, anthropicReq.MaxTokens, facts.NeedsVision, needsTools)
	if err != nil {
		h.sendError(w, http.StatusInternalServerError, "routing failed", err)
		return
	}

	h.logger.Info("routing request",
		"scenario", routeResult.Scenario,
		"model", routeResult.Primary.ModelID,
		"provider", routeResult.Primary.Provider,
		"tokens", tokenCount,
	)

	normalizedReq := core.NormalizeRequest(&anthropicReq)
	normalizedReq.Stream = isStreaming

	if isStreaming {
		h.handleStreaming(w, r, &anthropicReq, normalizedReq, modelChain, rawBody)
	} else {
		h.handleNonStreaming(w, r, &anthropicReq, normalizedReq, modelChain, rawBody)
	}
}

// buildModelChain resolves the request to a model chain (primary + fallbacks),
// honoring model_overrides (with a deduplicated scenario safety-net) and
// respecting the streaming-scenario-routing toggle.
//
// Precedence:
//  1. If requestedModel matches an entry in model_overrides, use that as the
//     primary and append the scenario chain as a deduplicated safety net.
//  2. Otherwise, fall through to scenario-based routing via routeOnce.
func (h *MessagesHandler) buildModelChain(
	requestedModel string,
	routerMessages []router.MessageContent,
	tokenCount int,
	isStreaming bool,
	requestedMaxTokens int,
	needsVision bool,
	needsTools bool,
) ([]config.ModelConfig, router.RouteResult, error) {
	var chain []config.ModelConfig
	var result router.RouteResult

	if requestedModel != "" {
		if overrideResult, ok := h.modelRouter.RouteWithOverride(requestedModel); ok {
			scenarioResult, err := h.routeOnce(routerMessages, tokenCount, "", isStreaming)
			if err != nil {
				return overrideResult.GetModelChain(), overrideResult, err
			}
			chain = appendUniqueModels(overrideResult.GetModelChain(), scenarioResult.GetModelChain())
			result = overrideResult
		}
	}

	if chain == nil {
		var err error
		result, err = h.routeOnce(routerMessages, tokenCount, requestedModel, isStreaming)
		if err != nil {
			return nil, result, err
		}
		chain = result.GetModelChain()
	}

	decision, err := router.FilterByCapacity(chain, tokenCount, requestedMaxTokens, needsVision, needsTools)
	if err != nil {
		return nil, result, err
	}

	for _, s := range decision.Skipped {
		h.logger.Info("model skipped by capacity filter", "model", s.ModelID, "reason", s.Reason)
	}

	return decision.Models, result, nil
}

// routeOnce performs scenario-based routing, honoring the streaming-scenario-routing
// toggle. Pass requestedModel="" to force scenario routing (used for the override
// safety-net chain), or a non-empty value to let resolveRequestedModel kick in
// (only when respect_requested_model is enabled and no override matched).
func (h *MessagesHandler) routeOnce(
	routerMessages []router.MessageContent,
	tokenCount int,
	requestedModel string,
	isStreaming bool,
) (router.RouteResult, error) {
	if isStreaming && !h.modelRouter.IsStreamingScenarioRoutingEnabled() {
		// Streaming: use faster models to minimize TTFT (time-to-first-token)
		return h.modelRouter.RouteForStreaming(routerMessages, tokenCount, requestedModel)
	}
	return h.modelRouter.Route(routerMessages, tokenCount, requestedModel)
}

// appendUniqueModels appends models from extra to base, skipping any model_id
// already present in base. The first occurrence of a ModelID is kept; later
// duplicates are dropped. Order of the base chain is preserved.
func appendUniqueModels(base, extra []config.ModelConfig) []config.ModelConfig {
	if len(extra) == 0 {
		return base
	}
	seen := make(map[string]struct{}, len(base))
	for _, m := range base {
		seen[m.ModelID] = struct{}{}
	}
	for _, m := range extra {
		if _, ok := seen[m.ModelID]; ok {
			continue
		}
		base = append(base, m)
		seen[m.ModelID] = struct{}{}
	}
	return base
}

// handleStreaming handles a streaming request with real-time SSE proxying.
func (h *MessagesHandler) handleStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	normalizedReq *core.NormalizedRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
) {
	clientCtx := r.Context()

	rw := &responseWriter{ResponseWriter: w}

	// Set SSE headers immediately
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rw.WriteHeader(http.StatusOK)
	rw.Flush()

	// Start heartbeat
	var finished int32
	var heartbeatPaused int32
	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if atomic.LoadInt32(&finished) == 1 {
					return
				}
				if atomic.LoadInt32(&heartbeatPaused) == 0 {
					rw.WriteKeepalive()
				}
			case <-heartbeatDone:
				return
			case <-clientCtx.Done():
				return
			}
		}
	}()
	defer func() {
		atomic.StoreInt32(&finished, 1)
		close(heartbeatDone)
	}()

	streamStart := time.Now()

	for _, model := range modelChain {
		select {
		case <-clientCtx.Done():
			h.logger.Debug("client disconnected, stopping streaming fallbacks")
			return
		default:
		}

		h.logger.Info("attempting streaming model", "model", model.ModelID, "provider", model.Provider)

		// Upstream context carries the streaming timeout configured for the model.
		timeout := h.client.StreamingTimeout(model)
		attemptCtx, cancelAttempt := context.WithTimeout(clientCtx, timeout)
		idleTimeout := h.client.StreamIdleTimeout(model)

		// recordStreamSuccess records a successful stream completion and
		// marks the model attempt as done.
		recordStreamSuccess := func(model config.ModelConfig) {
			cancelAttempt()
			latency := time.Since(streamStart)
			h.metrics.RecordSuccess(model.ModelID, latency)
			h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
		}

		// handleStreamError checks the error from a streaming attempt and
		// decides whether to retry the next model or abort. It returns true
		// if the caller should continue (fallback to next model), or false
		// if it should return.
		handleStreamError := func(err error, model config.ModelConfig, action string) bool {
			cancelAttempt()
			if clientCtx.Err() != nil {
				h.logger.Debug("client disconnected during " + action + " stream")
				return false // abort
			}
			if err == transformer.ErrStreamIdle {
				h.logger.Warn("upstream "+action+" stream idle, trying next model",
					"model", model.ModelID, "idle_timeout", idleTimeout)
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, "stream idle after SSE payload started")
					h.metrics.RecordFailure()
					return false // abort
				}
				return true // continue to next model
			}
			h.logger.Warn(action+" streaming failed", "model", model.ModelID, "error", err)
			if rw.ssePayloadWritten {
				h.sendStreamError(rw, "all upstream models failed after SSE payload started")
				h.metrics.RecordFailure()
				return false // abort — cannot fallback after SSE payload started
			}
			return true // continue to next model
		}

		// Try new provider-based dispatch first.
		if h.providerRegistry != nil {
			if prov, ok := h.providerRegistry.Get(model.Provider); ok {
				caps, ok := prov.ModelCapabilities(model.ModelID)
				if !ok || !caps.SupportsStreaming {
					h.logger.Warn("model does not support streaming", "model", model.ModelID, "provider", model.Provider)
					cancelAttempt()
					continue
				}

				streamBody, err := prov.Stream(attemptCtx, normalizedReq, model)
				if err != nil {
					cancelAttempt()
					if clientCtx.Err() != nil {
						h.logger.Debug("client disconnected during upstream request")
						return
					}
					h.logger.Warn("streaming request failed via provider", "model", model.ModelID, "provider", model.Provider, "error", err)
					continue
				}

				// Bind body read to attemptCtx so streaming_timeout_ms aborts mid-stream.
				streamReader := transformer.NewCtxReadCloser(attemptCtx, streamBody)

				wireFormat := prov.WireFormat(model.ModelID)
				if wireFormat == core.WireFormatAnthropic {
					atomic.StoreInt32(&heartbeatPaused, 1)
				}
				errProxy := h.streamProxy.ProxyStream(rw, streamReader, wireFormat, model.ModelID, attemptCtx, idleTimeout, cancelAttempt)
				if wireFormat == core.WireFormatAnthropic {
					atomic.StoreInt32(&heartbeatPaused, 0)
				}
				if errProxy != nil {
					if errProxy == transformer.ErrClientDisconnected {
						if clientCtx.Err() != nil {
							h.logger.Debug("client disconnected during stream")
							return
						}
						errProxy = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
					}
					if !handleStreamError(errProxy, model, wireFormat.String()) {
						return
					}
					continue
				}

				recordStreamSuccess(model)
				return
			}
		}

		// Legacy path for backward compatibility while old client is still in
		// use. Falls through to the old endpoint-classification logic.
		h.logger.Warn("provider not found in registry, falling back to old client",
			"provider", model.Provider, "model", model.ModelID)

		// Zen models use their own endpoint classification
		if client.IsZen(model) {
			endpointType := client.ClassifyEndpoint(model.ModelID)
			switch endpointType {
			case client.EndpointAnthropic:
				if model.AnthropicToolsDisabled {
					// Fall through to OpenAI-compatible transform path below.
				} else {
					modelBody := replaceModelInRawBody(rawBody, model.ModelID)
					if err := h.handleAnthropicStreaming(attemptCtx, rw, modelBody, model.ModelID, model, idleTimeout, cancelAttempt, clientCtx, &heartbeatPaused); err != nil {
						if !handleStreamError(err, model, "anthropic") {
							return
						}
						continue
					}
					recordStreamSuccess(model)
					return
				}

			case client.EndpointResponses:
				if err := h.handleResponsesStreaming(attemptCtx, rw, anthropicReq, model, clientCtx, idleTimeout, cancelAttempt); err != nil {
					if err == transformer.ErrClientDisconnected {
						if clientCtx.Err() != nil {
							h.logger.Debug("client disconnected during responses stream")
							return
						}
						err = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
					}
					if !handleStreamError(err, model, "responses") {
						return
					}
					continue
				}
				recordStreamSuccess(model)
				return

			case client.EndpointGemini:
				if err := h.handleGeminiStreaming(attemptCtx, rw, anthropicReq, model, clientCtx, idleTimeout, cancelAttempt); err != nil {
					if err == transformer.ErrClientDisconnected {
						if clientCtx.Err() != nil {
							h.logger.Debug("client disconnected during gemini stream")
							return
						}
						err = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
					}
					if !handleStreamError(err, model, "gemini") {
						return
					}
					continue
				}
				recordStreamSuccess(model)
				return

			default:
				// Fall through to OpenAI-compatible handling
			}
		}

		// Go provider Anthropic-native models (qwen3.7-max) that require raw
		// Anthropic format rather than the OpenAI Chat Completions transform.
		if !client.IsZen(model) && client.IsAnthropicModel(model.ModelID) {
			modelBody := replaceModelInRawBody(rawBody, model.ModelID)
			if err := h.handleAnthropicStreaming(attemptCtx, rw, modelBody, model.ModelID, model, idleTimeout, cancelAttempt, clientCtx, &heartbeatPaused); err != nil {
				if !handleStreamError(err, model, "anthropic") {
					return
				}
				continue
			}
			recordStreamSuccess(model)
			return
		}

		// OpenAI-compatible models (both Go and Zen)
		openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
		if err != nil {
			cancelAttempt()
			h.logger.Warn("request transform failed", "model", model.ModelID, "error", err)
			continue
		}

		streamBody, err := h.client.GetStreamingBody(attemptCtx, model.ModelID, openaiReq, model)
		if err != nil {
			cancelAttempt()
			if clientCtx.Err() != nil {
				h.logger.Debug("client disconnected during upstream request")
				return
			}
			h.logger.Warn("streaming request failed", "model", model.ModelID, "error", err)
			continue
		}

		// Bind body read to attemptCtx so streaming_timeout_ms aborts mid-stream.
		streamReader := transformer.NewCtxReadCloser(attemptCtx, streamBody)

		if err := h.streamHandler.ProxyStream(rw, streamReader, model.ModelID, attemptCtx, idleTimeout, cancelAttempt); err != nil {
			if err == transformer.ErrClientDisconnected {
				if clientCtx.Err() != nil {
					h.logger.Debug("client disconnected during stream")
					return
				}
				err = fmt.Errorf("streaming timeout (%v) exceeded", timeout)
			}
			if !handleStreamError(err, model, "openai") {
				return
			}
			continue
		}

		recordStreamSuccess(model)
		return
	}

	h.metrics.RecordFailure()
	if rw.ssePayloadWritten {
		// SSE payload was already sent — do not attempt further writes
		// beyond the error event.  The client has a partial stream.
		return
	}
	if !rw.wroteHeader {
		h.sendError(w, http.StatusBadGateway, "all streaming models failed", nil)
	} else {
		h.sendStreamError(rw, "all upstream models failed")
	}
}

// handleResponsesStreaming handles streaming for OpenAI Responses endpoint.
// ctx is the per-attempt context (carries streaming_timeout_ms); clientCtx is the
// broader request context used only for client-disconnect signaling.
func (h *MessagesHandler) handleResponsesStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	req, err := h.requestTransformer.TransformToResponses(anthropicReq, model)
	if err != nil {
		return fmt.Errorf("responses transform failed: %w", err)
	}

	streamBody, err := h.client.GetResponsesStreamingBody(ctx, model.ModelID, req, model)
	if err != nil {
		return err
	}

	// Bind body read to ctx so streaming_timeout_ms aborts mid-stream.
	streamReader := transformer.NewCtxReadCloser(ctx, streamBody)

	if err := h.streamHandler.ProxyResponsesStream(w, streamReader, model.ModelID, clientCtx, idleTimeout, cancel); err != nil {
		return err
	}

	return nil
}

// handleGeminiStreaming handles streaming for Gemini endpoint.
// ctx is the per-attempt context (carries streaming_timeout_ms); clientCtx is the
// broader request context used only for client-disconnect signaling.
func (h *MessagesHandler) handleGeminiStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	req, err := h.requestTransformer.TransformToGemini(anthropicReq, model)
	if err != nil {
		return fmt.Errorf("gemini transform failed: %w", err)
	}

	streamBody, err := h.client.GetGeminiStreamingBody(ctx, model.ModelID, req, model)
	if err != nil {
		return err
	}

	// Bind body read to ctx so streaming_timeout_ms aborts mid-stream.
	streamReader := transformer.NewCtxReadCloser(ctx, streamBody)

	if err := h.streamHandler.ProxyGeminiStream(w, streamReader, model.ModelID, clientCtx, idleTimeout, cancel); err != nil {
		return err
	}

	return nil
}

// sanitizeAnthropicBody removes the "type" field from tools whose value is
// "custom" (server-tool shorthands used by Claude Code for MCP tools that some
// upstream models don't understand). The upstream treats the tool as absent
// when type is missing rather than rejecting type:"custom".
// Returns the original body unchanged if no tools array is present or if no
// tool has type:"custom".
func sanitizeAnthropicBody(rawBody json.RawMessage) json.RawMessage {
	var body map[string]any
	if err := json.Unmarshal(rawBody, &body); err != nil {
		return rawBody
	}

	tools, ok := body["tools"].([]any)
	if !ok || len(tools) == 0 {
		return rawBody
	}

	modified := false
	for _, tool := range tools {
		toolMap, ok := tool.(map[string]any)
		if !ok {
			continue
		}
		if toolType, ok := toolMap["type"].(string); ok && toolType == "custom" {
			delete(toolMap, "type")
			modified = true
		}
	}

	if !modified {
		return rawBody
	}

	result, err := json.Marshal(body)
	if err != nil {
		return rawBody
	}
	return json.RawMessage(result)
}

// replaceModelInRawBody replaces the top-level "model" field in raw JSON body
// with the actual model ID.  Uses JSON unmarshal/marshal rather than string
// search so that nested occurrences of "model" in user content, tool schemas,
// or escaped strings are never touched.
func replaceModelInRawBody(rawBody json.RawMessage, modelID string) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &obj); err != nil {
		slog.Error("could not parse request body for model replacement, using original",
			"error", err)
		return rawBody
	}
	if _, ok := obj["model"]; !ok {
		return rawBody
	}
	encoded, err := json.Marshal(modelID)
	if err != nil {
		// json.Marshal on a string should never fail, but guard anyway.
		slog.Error("failed to marshal model ID for body replacement",
			"error", err, "model_id", modelID)
		return rawBody
	}
	obj["model"] = encoded
	result, err := json.Marshal(obj)
	if err != nil {
		slog.Error("could not marshal request body after model replacement, using original",
			"error", err)
		return rawBody
	}
	slog.Debug("replaced model in request body",
		"new_model", modelID,
		"success", true)
	return json.RawMessage(result)
}

// handleAnthropicStreaming sends a raw Anthropic request to the Anthropic endpoint.
func (h *MessagesHandler) handleAnthropicStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	rawBody json.RawMessage,
	modelID string,
	model config.ModelConfig,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
	clientCtx context.Context,
	heartbeatPaused *int32,
) error {
	atomic.StoreInt32(heartbeatPaused, 1)
	defer atomic.StoreInt32(heartbeatPaused, 0)
	// Sanitize Anthropic-specific fields (e.g., tool type shorthands) that
	// upstream models may not understand.
	rawBody = sanitizeAnthropicBody(rawBody)

	h.logger.Debug("sending anthropic streaming request",
		"model_id", modelID,
		"body_preview", string(rawBody)[:min(len(rawBody), 200)])

	resp, err := h.client.SendAnthropicRequest(ctx, rawBody, true, model)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	defer cancel()

	// Bind body read to ctx so streaming_timeout_ms aborts mid-stream.
	bodyReader := transformer.NewCtxReader(ctx, resp.Body)

	// Stream the body chunk-by-chunk with an idle watchdog. The stream lives
	// as long as data keeps flowing and is aborted when no byte arrives
	// within idleTimeout.
	buf := make([]byte, 4096)
	ping := transformer.StartIdleWatchdog(ctx, cancel, idleTimeout)
	for {
		select {
		case <-ctx.Done():
			// ctx is canceled by either the idle watchdog or client disconnect.
			// Distinguish: watchdog fires while client is still connected.
			if clientCtx.Err() == nil {
				return transformer.ErrStreamIdle
			}
			return transformer.ErrClientDisconnected
		default:
		}
		n, rerr := bodyReader.Read(buf)
		if n > 0 {
			ping()
			if _, werr := w.Write(buf[:n]); werr != nil {
				return transformer.ErrClientDisconnected
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if rerr == io.EOF {
			return nil
		}
		if rerr != nil {
			if errors.Is(rerr, transformer.ErrStreamReadCanceled) {
				if clientCtx.Err() == nil {
					return transformer.ErrStreamIdle
				}
				return transformer.ErrClientDisconnected
			}
			if transformer.IsIdleTimeout(rerr) {
				return transformer.ErrStreamIdle
			}
			if errors.Is(rerr, context.Canceled) || ctx.Err() == context.Canceled {
				if clientCtx.Err() == nil {
					return transformer.ErrStreamIdle
				}
				return transformer.ErrClientDisconnected
			}
			return fmt.Errorf("failed to copy response: %w", rerr)
		}
	}
}

// sendStreamError sends an error event in the SSE stream.
func (h *MessagesHandler) sendStreamError(w http.ResponseWriter, message string) {
	h.logger.Error("sending stream error", "message", message)

	errorEvent := map[string]interface{}{
		"type": "error",
		"error": map[string]interface{}{
			"type":    "api_error",
			"message": message,
		},
	}

	data, _ := json.Marshal(errorEvent)
	if _, err := fmt.Fprintf(w, "event: error\ndata: %s\n\n", string(data)); err != nil {
		h.logger.Debug("failed to write stream error event", "error", err, "message", message)
	}

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handleNonStreaming handles a non-streaming request with fallback.
func (h *MessagesHandler) handleNonStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	normalizedReq *core.NormalizedRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
) {
	ctx := r.Context()
	startTime := time.Now()

	result, responseBody, err := h.fallbackHandler.ExecuteWithFallback(
		ctx,
		modelChain,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			timeout := h.client.RequestTimeout(model)
			attemptCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			// Try new provider-based dispatch first.
			if h.providerRegistry != nil {
				if prov, ok := h.providerRegistry.Get(model.Provider); ok {
					execResult, execErr := prov.Execute(attemptCtx, normalizedReq, model)
					if execErr != nil {
						return nil, execErr
					}
					return execResult.Body, nil
				}
			}

			h.logger.Warn("provider not found in registry, falling back to old client",
				"provider", model.Provider, "model", model.ModelID)

			// Legacy path: Zen models use their own endpoint classification
			if client.IsZen(model) {
				endpointType := client.ClassifyEndpoint(model.ModelID)
				switch endpointType {
				case client.EndpointAnthropic:
					if model.AnthropicToolsDisabled {
						// Fall through to OpenAI-compatible handling below.
					} else {
						return h.executeAnthropicRequest(attemptCtx, replaceModelInRawBody(rawBody, model.ModelID), model)
					}
				case client.EndpointResponses:
					return h.executeResponsesRequest(attemptCtx, anthropicReq, model)
				case client.EndpointGemini:
					return h.executeGeminiRequest(attemptCtx, anthropicReq, model)
				default:
					// Fall through to OpenAI-compatible handling
				}
			} else if client.IsAnthropicModel(model.ModelID) {
				// Go provider Anthropic-native models (MiniMax, Qwen)
				return h.executeAnthropicRequest(attemptCtx, replaceModelInRawBody(rawBody, model.ModelID), model)
			}

			// OpenAI-compatible models (both Go and Zen)
			return h.executeOpenAIRequest(attemptCtx, anthropicReq, model)
		},
	)

	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			h.logger.Info("request context canceled during non-streaming fallback", "error", err)
			return
		}
		h.metrics.RecordFailure()
		h.sendError(w, http.StatusBadGateway, "all models failed", err)
		return
	}

	latency := time.Since(startTime)
	h.metrics.RecordSuccess(result.ModelID, latency)

	h.logger.Info("request completed",
		"model", result.ModelID,
		"attempts", result.Attempted,
		"latency", latency,
	)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(responseBody)
}

// executeAnthropicRequest executes a request to the Anthropic endpoint (for MiniMax models).
func (h *MessagesHandler) executeAnthropicRequest(
	ctx context.Context,
	rawBody json.RawMessage,
	model config.ModelConfig,
) ([]byte, error) {
	// Sanitize Anthropic-specific fields (e.g., tool type shorthands) that
	// upstream models may not understand.
	rawBody = sanitizeAnthropicBody(rawBody)

	resp, err := h.client.SendAnthropicRequest(ctx, rawBody, false, model)
	if err != nil {
		return nil, fmt.Errorf("anthropic request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	h.logger.Debug("anthropic response", "body", string(body))

	return body, nil
}

// executeOpenAIRequest executes a request to the OpenAI endpoint with transformation.
func (h *MessagesHandler) executeOpenAIRequest(
	ctx context.Context,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) ([]byte, error) {
	openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
	if err != nil {
		return nil, fmt.Errorf("request transform failed: %w", err)
	}

	resp, err := h.client.ChatCompletionNonStreaming(ctx, model.ModelID, openaiReq, model)
	if err != nil {
		return nil, fmt.Errorf("chat completion failed: %w", err)
	}

	anthropicResp, err := h.responseTransformer.TransformResponse(resp, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}

	return json.Marshal(anthropicResp)
}

// executeResponsesRequest executes a request to the OpenAI Responses endpoint.
func (h *MessagesHandler) executeResponsesRequest(
	ctx context.Context,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) ([]byte, error) {
	req, err := h.requestTransformer.TransformToResponses(anthropicReq, model)
	if err != nil {
		return nil, fmt.Errorf("responses transform failed: %w", err)
	}

	resp, err := h.client.ResponsesCompletionNonStreaming(ctx, model.ModelID, req, model)
	if err != nil {
		return nil, fmt.Errorf("responses completion failed: %w", err)
	}

	anthropicResp, err := h.responseTransformer.TransformResponsesResponse(resp, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}

	return json.Marshal(anthropicResp)
}

// executeGeminiRequest executes a request to the Gemini endpoint.
func (h *MessagesHandler) executeGeminiRequest(
	ctx context.Context,
	anthropicReq *types.MessageRequest,
	model config.ModelConfig,
) ([]byte, error) {
	req, err := h.requestTransformer.TransformToGemini(anthropicReq, model)
	if err != nil {
		return nil, fmt.Errorf("gemini transform failed: %w", err)
	}

	resp, err := h.client.GeminiCompletionNonStreaming(ctx, model.ModelID, req, model)
	if err != nil {
		return nil, fmt.Errorf("gemini completion failed: %w", err)
	}

	anthropicResp, err := h.responseTransformer.TransformGeminiResponse(resp, model.ModelID)
	if err != nil {
		return nil, fmt.Errorf("response transform failed: %w", err)
	}

	return json.Marshal(anthropicResp)
}

// extractTextFromBlocks extracts plain text from Anthropic content blocks.
func extractTextFromBlocks(blocks []types.ContentBlock) string {
	var content string
	for _, block := range blocks {
		switch block.Type {
		case "text":
			content += block.Text
		case "tool_use":
			content += fmt.Sprintf("[Tool Use: %s]", block.Name)
		case "tool_result":
			content += block.TextContent()
		case "thinking":
			// Skip thinking blocks for text extraction
		case "image":
			content += "[Image]"
		}
	}
	return content
}

func blocksHaveImage(blocks []types.ContentBlock) bool {
	for _, block := range blocks {
		if block.Type == "image" && block.Source != nil {
			return true
		}
	}
	return false
}

func imageHashesFromBlocks(blocks []types.ContentBlock) []string {
	var hashes []string
	for _, block := range blocks {
		if block.Type != "image" || block.Source == nil {
			continue
		}
		source := block.Source.Type + "\x00" + block.Source.MediaType + "\x00" + block.Source.Data + "\x00" + block.Source.URL
		sum := sha256.Sum256([]byte(source))
		hashes = append(hashes, hex.EncodeToString(sum[:]))
	}
	return hashes
}

// sendError sends an error response in Anthropic format.
func (h *MessagesHandler) sendError(w http.ResponseWriter, statusCode int, message string, err error) {
	h.logger.Error("request error",
		"status", statusCode,
		"message", message,
		"error", err,
	)

	if rw, ok := w.(*responseWriter); ok && rw.headerWritten() {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := transformer.TransformErrorResponse(statusCode, message)
	_ = json.NewEncoder(w).Encode(errorResp)
}
