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

// responseWriter wraps http.ResponseWriter to track if headers were written
// and serialize concurrent writes (heartbeat + stream body copy).
type responseWriter struct {
	http.ResponseWriter
	mu          sync.Mutex
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
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
	return w.ResponseWriter.Write(b)
}

// Flush implements http.Flusher for SSE streaming support.
func (w *responseWriter) Flush() {
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
		requestDedup:        middleware.NewRequestDeduplicator(500 * time.Millisecond),
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

	requestID := r.Header.Get("X-Request-ID")
	if requestID == "" {
		requestID = h.requestIDGen.Generate()
	}
	w.Header().Set("X-Request-ID", requestID)

	clientIP := middleware.GetClientIP(r)
	if !h.rateLimiter.Allow(clientIP) {
		h.metrics.RecordRateLimited()
		h.logger.Warn("rate limited", "client", clientIP, "request_id", requestID)
		http.Error(w, "rate limited", http.StatusTooManyRequests)
		return
	}

	var rawBody json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&rawBody); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if _, ok := h.requestDedup.TryAcquire(rawBody); !ok {
		h.metrics.RecordDeduplicated()
		h.logger.Info("duplicate request skipped", "request_id", requestID)
		return
	}

	var anthropicReq types.MessageRequest
	if err := json.Unmarshal(rawBody, &anthropicReq); err != nil {
		h.sendError(w, http.StatusBadRequest, "invalid request body", err)
		return
	}

	if err := anthropicReq.Validate(); err != nil {
		h.sendError(w, http.StatusBadRequest, err.Error(), nil)
		return
	}

	isStreaming := anthropicReq.Stream != nil && *anthropicReq.Stream
	h.metrics.RecordRequest(isStreaming)

	h.logger.Info("received request",
		"model", anthropicReq.Model,
		"streaming", isStreaming,
		"messages", len(anthropicReq.Messages),
		"tools", len(anthropicReq.Tools),
		"max_tokens", anthropicReq.MaxTokens,
	)

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

	tokenCount, err := h.tokenCounter.CountMessages(systemText, tokenMessages)
	if err != nil {
		h.logger.Warn("failed to count tokens", "error", err)
		tokenCount = 0
	}

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
		h.handleStreaming(w, r, &anthropicReq, modelChain, rawBody)
	} else {
		h.handleNonStreaming(w, r, &anthropicReq, modelChain, rawBody)
	}
}

// buildModelChain resolves the primary model and fallback chain.
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

// routeOnce runs the router once, respecting the streaming toggle.
func (h *MessagesHandler) routeOnce(
	routerMessages []router.MessageContent,
	tokenCount int,
	requestedModel string,
	isStreaming bool,
) (router.RouteResult, error) {
	if isStreaming && !h.modelRouter.IsStreamingScenarioRoutingEnabled() {
		return h.modelRouter.RouteForStreaming(routerMessages, tokenCount, requestedModel), nil
	}
	return h.modelRouter.Route(routerMessages, tokenCount, requestedModel)
}

// appendUniqueModels appends models from extra if their model_id is new.
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

// handleStreaming proxies a streaming request with per-model timeouts.
func (h *MessagesHandler) handleStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
) {
	clientCtx := r.Context()

	rw := &responseWriter{ResponseWriter: w}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	rw.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	var finished int32
	heartbeatDone := make(chan struct{})
	heartbeatPaused := new(atomic.Int32)
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if atomic.LoadInt32(&finished) == 1 {
					return
				}
				if heartbeatPaused.Load() == 1 {
					continue
				}
				_, _ = fmt.Fprintf(rw, ":keepalive\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
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
		if err := clientCtx.Err(); err != nil {
			h.logger.Info("client disconnected, stopping streaming fallbacks", "error", err)
			return
		}

		h.logger.Info("attempting streaming model", "model", model.ModelID, "provider", model.Provider)

		timeout := h.client.StreamingTimeout(model)
		attemptCtx, cancel := context.WithTimeout(clientCtx, timeout)

		if client.IsZen(model) {
			endpointType := client.ClassifyEndpoint(model.ModelID)
			switch endpointType {
			case client.EndpointAnthropic:
				modelBody := replaceModelInRawBody(rawBody, model.ModelID)
				heartbeatPaused.Store(1)
				err := h.handleAnthropicStreaming(attemptCtx, rw, modelBody, model.ModelID, model)
				heartbeatPaused.Store(0)
				if err != nil {
					cancel()
					if clientCtx.Err() != nil {
						h.logger.Info("client disconnected during anthropic stream", "error", clientCtx.Err())
						return
					}
					h.logger.Warn("anthropic streaming failed", "model", model.ModelID, "error", err)
					continue
				}
				cancel()
				latency := time.Since(streamStart)
				h.metrics.RecordSuccess(model.ModelID, latency)
				h.logger.Info("streaming completed", "model", model.ModelID, "latency", latency)
				return

			case client.EndpointResponses:
				if err := h.handleResponsesStreaming(attemptCtx, rw, anthropicReq, model, clientCtx); err != nil {
					cancel()
					if clientCtx.Err() != nil {
						h.logger.Info("client disconnected during responses stream", "error", clientCtx.Err())
						return
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
				if err := h.handleGeminiStreaming(attemptCtx, rw, anthropicReq, model, clientCtx); err != nil {
					cancel()
					if clientCtx.Err() != nil {
						h.logger.Info("client disconnected during gemini stream", "error", clientCtx.Err())
						return
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
			}
		} else if client.IsAnthropicModel(model.ModelID) {
			modelBody := replaceModelInRawBody(rawBody, model.ModelID)
			heartbeatPaused.Store(1)
			err := h.handleAnthropicStreaming(attemptCtx, rw, modelBody, model.ModelID, model)
			heartbeatPaused.Store(0)
			if err != nil {
				cancel()
				if clientCtx.Err() != nil {
					h.logger.Info("client disconnected during anthropic stream", "error", clientCtx.Err())
					return
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

		openaiReq, err := h.requestTransformer.TransformRequest(anthropicReq, model)
		if err != nil {
			cancel()
			h.logger.Warn("request transform failed", "model", model.ModelID, "error", err)
			continue
		}

		streamBody, err := h.client.GetStreamingBody(attemptCtx, model.ModelID, openaiReq, model)
		if err != nil {
			cancel()
			if clientCtx.Err() != nil {
				h.logger.Info("client disconnected during upstream request", "error", clientCtx.Err())
				return
			}
			h.logger.Warn("streaming request failed", "model", model.ModelID, "error", err)
			continue
		}

		if err := h.streamHandler.ProxyStream(rw, streamBody, model.ModelID, clientCtx); err != nil {
			_ = streamBody.Close()
			cancel()
			if err == transformer.ErrClientDisconnected {
				h.logger.Info("client disconnected during stream")
				return
			}
			if clientCtx.Err() != nil {
				h.logger.Info("client disconnected during stream (context canceled)", "error", clientCtx.Err())
				return
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
) error {
	req, err := h.requestTransformer.TransformToResponses(anthropicReq, model)
	if err != nil {
		return fmt.Errorf("responses transform failed: %w", err)
	}

	streamBody, err := h.client.GetResponsesStreamingBody(ctx, model.ModelID, req, model)
	if err != nil {
		return err
	}

	if err := h.streamHandler.ProxyResponsesStream(w, streamBody, model.ModelID, clientCtx); err != nil {
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
) error {
	req, err := h.requestTransformer.TransformToGemini(anthropicReq, model)
	if err != nil {
		return fmt.Errorf("gemini transform failed: %w", err)
	}

	streamBody, err := h.client.GetGeminiStreamingBody(ctx, model.ModelID, req, model)
	if err != nil {
		return err
	}

	if err := h.streamHandler.ProxyGeminiStream(w, streamBody, model.ModelID, clientCtx); err != nil {
		_ = streamBody.Close()
		return err
	}

	_ = streamBody.Close()
	return nil
}

// replaceModelInRawBody replaces the top-level model field in a raw JSON body.
func replaceModelInRawBody(rawBody json.RawMessage, modelID string) json.RawMessage {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &obj); err != nil {
		slog.Warn("replaceModelInRawBody: unmarshal failed, using original body",
			"error", err,
			"body_preview", string(rawBody)[:min(len(rawBody), 200)])
		return rawBody
	}

	oldModelRaw, ok := obj["model"]
	if !ok {
		slog.Warn("replaceModelInRawBody: model key not found, using original body",
			"body_preview", string(rawBody)[:min(len(rawBody), 200)])
		return rawBody
	}

	var oldModel string
	if err := json.Unmarshal(oldModelRaw, &oldModel); err != nil {
		slog.Warn("replaceModelInRawBody: model value not a string, using original body",
			"error", err)
		return rawBody
	}

	modelJSON, err := json.Marshal(modelID)
	if err != nil {
		slog.Warn("replaceModelInRawBody: marshal modelID failed, using original body",
			"error", err)
		return rawBody
	}
	obj["model"] = modelJSON

	newBody, err := json.Marshal(obj)
	if err != nil {
		slog.Warn("replaceModelInRawBody: remarshal failed, using original body",
			"error", err)
		return rawBody
	}

	slog.Debug("replaced model in request body",
		"old_model", oldModel,
		"new_model", modelID,
		"success", true)
	return json.RawMessage(newBody)
}

// handleAnthropicStreaming sends a raw Anthropic request to the Anthropic endpoint.
func (h *MessagesHandler) handleAnthropicStreaming(
	ctx context.Context,
	w http.ResponseWriter,
	rawBody json.RawMessage,
	modelID string,
	model config.ModelConfig,
) error {
	h.logger.Debug("sending anthropic streaming request",
		"model_id", modelID,
		"body_preview", string(rawBody)[:min(len(rawBody), 200)])

	resp, err := h.client.SendAnthropicRequest(ctx, rawBody, true, model)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	_, err = io.Copy(w, resp.Body)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return transformer.ErrClientDisconnected
		}
		return fmt.Errorf("failed to copy response: %w", err)
	}

	return nil
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

// handleNonStreaming runs a non-streaming request with per-model timeouts.
func (h *MessagesHandler) handleNonStreaming(
	w http.ResponseWriter,
	r *http.Request,
	anthropicReq *types.MessageRequest,
	modelChain []config.ModelConfig,
	rawBody json.RawMessage,
) {
	parentCtx := r.Context()
	startTime := time.Now()

	result, responseBody, err := h.fallbackHandler.ExecuteWithFallback(
		parentCtx,
		modelChain,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			timeout := h.client.RequestTimeout(model)
			attemptCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			if client.IsZen(model) {
				endpointType := client.ClassifyEndpoint(model.ModelID)
				switch endpointType {
				case client.EndpointAnthropic:
					modelBody := replaceModelInRawBody(rawBody, model.ModelID)
					return h.executeAnthropicRequest(attemptCtx, modelBody, model)
				case client.EndpointResponses:
					return h.executeResponsesRequest(attemptCtx, anthropicReq, model)
				case client.EndpointGemini:
					return h.executeGeminiRequest(attemptCtx, anthropicReq, model)
				default:
				}
			} else if client.IsAnthropicModel(model.ModelID) {
				modelBody := replaceModelInRawBody(rawBody, model.ModelID)
				return h.executeAnthropicRequest(attemptCtx, modelBody, model)
			}

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
