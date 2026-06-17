// Package handlers contains HTTP request handlers for API endpoints.
package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"oc-go-cc/internal/client"
	"oc-go-cc/internal/config"
	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/middleware"
	"oc-go-cc/internal/router"
	"oc-go-cc/internal/token"
	"oc-go-cc/internal/transformer"
	"oc-go-cc/pkg/types"
)

// MessagesHandler handles /v1/messages requests.
type MessagesHandler struct {
	client              *client.OpenCodeClient
	modelRouter         *router.ModelRouter
	fallbackHandler     *router.FallbackHandler
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

// Flush implements http.Flusher for SSE streaming support.
func (w *responseWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// NewMessagesHandler creates a new messages handler.
func NewMessagesHandler(
	openCodeClient *client.OpenCodeClient,
	modelRouter *router.ModelRouter,
	fallbackHandler *router.FallbackHandler,
	tokenCounter *token.Counter,
	metrics *metrics.Metrics,
) *MessagesHandler {
	return &MessagesHandler{
		client:              openCodeClient,
		modelRouter:         modelRouter,
		fallbackHandler:     fallbackHandler,
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

	// Generate or get request ID for correlation
	requestID := r.Header.Get("X-Request-ID")
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

	// Deduplicate - skip duplicate requests
	if _, ok := h.requestDedup.TryAcquire(rawBody); !ok {
		h.metrics.RecordDeduplicated()
		h.logger.Info("duplicate request skipped", "request_id", requestID)
		return
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
			Role:    msg.Role,
			Content: content,
		}
		routerMessages = append(routerMessages, mc)
		tokenMessages = append(tokenMessages, token.MessageContent{
			Role:    msg.Role,
			Content: content,
		})
	}

	// Count tokens.
	tokenCount, err := h.tokenCounter.CountMessages(systemText, tokenMessages)
	if err != nil {
		h.logger.Warn("failed to count tokens", "error", err)
		tokenCount = 0
	}

	// Route to appropriate model and build fallback chain.
	modelChain, routeResult, err := h.buildModelChain(anthropicReq.Model, routerMessages, tokenCount, isStreaming)
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

	if isStreaming {
		// Streaming: use ProxyStream for real-time SSE transformation
		h.handleStreaming(w, r, &anthropicReq, modelChain, rawBody)
	} else {
		// Non-streaming: execute with fallback and return full response
		h.handleNonStreaming(w, r, &anthropicReq, modelChain, rawBody)
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
) ([]config.ModelConfig, router.RouteResult, error) {
	if requestedModel != "" {
		if overrideResult, ok := h.modelRouter.RouteWithOverride(requestedModel); ok {
			scenarioResult, err := h.routeOnce(routerMessages, tokenCount, "", isStreaming)
			if err != nil {
				// Override is valid; surface the scenario routing error rather
				// than silently dropping the safety net.
				return overrideResult.GetModelChain(), overrideResult, err
			}
			chain := appendUniqueModels(overrideResult.GetModelChain(), scenarioResult.GetModelChain())
			return chain, overrideResult, nil
		}
	}

	result, err := h.routeOnce(routerMessages, tokenCount, requestedModel, isStreaming)
	if err != nil {
		return nil, result, err
	}
	return result.GetModelChain(), result, nil
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
		return h.modelRouter.RouteForStreaming(routerMessages, tokenCount, requestedModel), nil
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
				_, _ = fmt.Fprintf(rw, ":keepalive\n\n")
				rw.Flush()
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

		// Upstream context: no total deadline. The stream lives as long as
		// data keeps flowing. Per-Read idle deadline is enforced in stream.go
		// via http.ResponseController, so a stuck stream is still caught.
		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			<-clientCtx.Done()
			cancel()
		}()
		idleTimeout := h.client.StreamIdleTimeout(model)

		// Zen models use their own endpoint classification
		if client.IsZen(model) {
			endpointType := client.ClassifyEndpoint(model.ModelID)
			switch endpointType {
			case client.EndpointAnthropic:
				if model.AnthropicToolsDisabled {
					// Fall through to OpenAI-compatible transform path below.
				} else {
					modelBody := replaceModelInRawBody(rawBody, model.ModelID)
					if err := h.handleAnthropicStreaming(ctx, rw, modelBody, model.ModelID, model, idleTimeout, cancel, clientCtx); err != nil {
						cancel()
						if clientCtx.Err() == context.Canceled {
							h.logger.Debug("client disconnected during anthropic stream")
							return
						}
						if err == transformer.ErrStreamIdle {
							h.logger.Warn("upstream anthropic stream idle, trying next model", "model", model.ModelID, "idle_timeout", idleTimeout)
							continue
						}
						h.logger.Warn("anthropic streaming failed", "model", model.ModelID, "error", err)
						continue
					}
					cancel()
					latency := time.Since(streamStart)
					h.metrics.RecordSuccess(model.ModelID, latency)
					h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
					return
				}

			case client.EndpointResponses:
				if err := h.handleResponsesStreaming(ctx, rw, anthropicReq, model, clientCtx, idleTimeout, cancel); err != nil {
					cancel()
					if clientCtx.Err() == context.Canceled {
						h.logger.Debug("client disconnected during responses stream")
						return
					}
					if err == transformer.ErrStreamIdle {
						h.logger.Warn("upstream responses stream idle, trying next model", "model", model.ModelID, "idle_timeout", idleTimeout)
						continue
					}
					h.logger.Warn("responses streaming failed", "model", model.ModelID, "error", err)
					continue
				}
				cancel()
				latency := time.Since(streamStart)
				h.metrics.RecordSuccess(model.ModelID, latency)
				h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
				return

			case client.EndpointGemini:
				if err := h.handleGeminiStreaming(ctx, rw, anthropicReq, model, clientCtx, idleTimeout, cancel); err != nil {
					cancel()
					if clientCtx.Err() == context.Canceled {
						h.logger.Debug("client disconnected during gemini stream")
						return
					}
					if err == transformer.ErrStreamIdle {
						h.logger.Warn("upstream gemini stream idle, trying next model", "model", model.ModelID, "idle_timeout", idleTimeout)
						continue
					}
					h.logger.Warn("gemini streaming failed", "model", model.ModelID, "error", err)
					continue
				}
				cancel()
				latency := time.Since(streamStart)
				h.metrics.RecordSuccess(model.ModelID, latency)
				h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
				return

			default:
				// Fall through to OpenAI-compatible handling
			}
		}

		// Go provider Anthropic-native models (qwen3.7-max) that require raw
		// Anthropic format rather than the OpenAI Chat Completions transform.
		if !client.IsZen(model) && client.IsAnthropicModel(model.ModelID) {
			modelBody := replaceModelInRawBody(rawBody, model.ModelID)
			if err := h.handleAnthropicStreaming(ctx, rw, modelBody, model.ModelID, model, idleTimeout, cancel, clientCtx); err != nil {
				cancel()
				if clientCtx.Err() == context.Canceled {
					h.logger.Debug("client disconnected during anthropic stream")
					return
				}
				if err == transformer.ErrStreamIdle {
					h.logger.Warn("upstream anthropic stream idle, trying next model", "model", model.ModelID, "idle_timeout", idleTimeout)
					if rw.ssePayloadWritten {
						h.sendStreamError(rw, "upstream stream idle after SSE payload started")
						h.metrics.RecordFailure()
						return
					}
					continue
				}
				h.logger.Warn("anthropic streaming failed", "model", model.ModelID, "error", err)
				if rw.ssePayloadWritten {
					h.sendStreamError(rw, fmt.Sprintf("all upstream models failed after SSE payload started: %v", err))
					h.metrics.RecordFailure()
					return
				}
				continue
			}
			cancel()
			latency := time.Since(streamStart)
			h.metrics.RecordSuccess(model.ModelID, latency)
			h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
			return
		}

		// OpenAI-compatible models (both Go and Zen)
		openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
		if err != nil {
			cancel()
			h.logger.Warn("request transform failed", "model", model.ModelID, "error", err)
			continue
		}

		streamBody, err := h.client.GetStreamingBody(ctx, model.ModelID, openaiReq, model)
		if err != nil {
			cancel()
			if clientCtx.Err() == context.Canceled {
				h.logger.Debug("client disconnected during upstream request")
				return
			}
			h.logger.Warn("streaming request failed", "model", model.ModelID, "error", err)
			continue
		}

		if err := h.streamHandler.ProxyStream(rw, streamBody, model.ModelID, clientCtx, idleTimeout, cancel); err != nil {
			_ = streamBody.Close()
			cancel()
			if err == transformer.ErrClientDisconnected {
				h.logger.Debug("client disconnected during stream")
				return
			}
			if clientCtx.Err() == context.Canceled {
				h.logger.Debug("client disconnected during stream (context canceled)")
				return
			}
			if err == transformer.ErrStreamIdle {
				h.logger.Warn("upstream stream idle, trying next model", "model", model.ModelID, "idle_timeout", idleTimeout)
				continue
			}
			h.logger.Warn("stream proxy failed", "model", model.ModelID, "error", err)
			continue
		}

		_ = streamBody.Close()
		cancel()
		latency := time.Since(streamStart)
		h.metrics.RecordSuccess(model.ModelID, latency)
		h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
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

	if err := h.streamHandler.ProxyResponsesStream(w, streamBody, model.ModelID, clientCtx, idleTimeout, cancel); err != nil {
		_ = streamBody.Close()
		return err
	}

	_ = streamBody.Close()
	return nil
}

// handleGeminiStreaming handles streaming for Gemini endpoint.
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

	if err := h.streamHandler.ProxyGeminiStream(w, streamBody, model.ModelID, clientCtx, idleTimeout, cancel); err != nil {
		_ = streamBody.Close()
		return err
	}

	_ = streamBody.Close()
	return nil
}

// sanitizeAnthropicBody removes Anthropic-specific tool fields that some upstream
// models don't understand (e.g., "type":"custom" server-tool shorthands used by
// Claude Code for MCP tools). Returns the original body unchanged if no tools
// array is present or if no tool has a type field.
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
		if _, hasType := toolMap["type"]; hasType {
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
		slog.Warn("could not parse request body for model replacement, using original",
			"error", err)
		return rawBody
	}
	encoded, _ := json.Marshal(modelID)
	obj["model"] = encoded
	result, err := json.Marshal(obj)
	if err != nil {
		slog.Warn("could not marshal request body after model replacement, using original",
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
) error {
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

	// Stream the body byte-by-byte with an idle watchdog. The stream lives
	// as long as data keeps flowing and is aborted when no byte arrives
	// within idleTimeout.
	buf := make([]byte, 4096)
	ping := transformer.StartIdleWatchdog(ctx, cancel, idleTimeout)
	for {
		select {
		case <-ctx.Done():
			return transformer.ErrClientDisconnected
		default:
		}
		n, rerr := resp.Body.Read(buf)
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
	_, _ = fmt.Fprintf(w, "event: error\ndata: %s\n\n", string(data))

	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// handleNonStreaming handles a non-streaming request with fallback.
func (h *MessagesHandler) handleNonStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
) {
	ctx := r.Context()
	startTime := time.Now()

	result, responseBody, err := h.fallbackHandler.ExecuteWithFallback(
		ctx,
		modelChain,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			// Zen models use their own endpoint classification
			if client.IsZen(model) {
				endpointType := client.ClassifyEndpoint(model.ModelID)
				switch endpointType {
				case client.EndpointAnthropic:
					if model.AnthropicToolsDisabled {
						// Fall through to OpenAI-compatible handling below.
					} else {
						return h.executeAnthropicRequest(ctx, replaceModelInRawBody(rawBody, model.ModelID), model)
					}
				case client.EndpointResponses:
					return h.executeResponsesRequest(ctx, anthropicReq, model)
				case client.EndpointGemini:
					return h.executeGeminiRequest(ctx, anthropicReq, model)
				default:
					// Fall through to OpenAI-compatible handling
				}
			} else if client.IsAnthropicModel(model.ModelID) {
				// Go provider Anthropic-native models (MiniMax, Qwen)
				return h.executeAnthropicRequest(ctx, replaceModelInRawBody(rawBody, model.ModelID), model)
			}

			// OpenAI-compatible models (both Go and Zen)
			return h.executeOpenAIRequest(ctx, anthropicReq, model)
		},
	)

	if err != nil {
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

// sendError sends an error response in Anthropic format.
func (h *MessagesHandler) sendError(w http.ResponseWriter, statusCode int, message string, err error) {
	h.logger.Error("request error",
		"status", statusCode,
		"message", message,
		"error", err,
	)

	if rw, ok := w.(*responseWriter); ok && rw.wroteHeader {
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := transformer.TransformErrorResponse(statusCode, message)
	_ = json.NewEncoder(w).Encode(errorResp)
}
