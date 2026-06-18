// Package transformer handles request/response transformation and token counting.
package transformer

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"oc-go-cc/pkg/types"
)

// ErrClientDisconnected is returned when the client disconnects during streaming.
var ErrClientDisconnected = fmt.Errorf("client disconnected")

// ErrStreamIdle is returned when no bytes arrive within idleTimeout on the
// upstream stream. The connection is stale (e.g. backend hang or network
// partition). The handler decides whether to fall back to another model.
var ErrStreamIdle = fmt.Errorf("upstream stream idle")

// IsIdleTimeout reports whether err is a read-timeout (network deadline
// exceeded on an otherwise live stream).
func IsIdleTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return true
	}
	return false
}

// StreamHandler handles streaming SSE transformation from OpenAI to Anthropic format.
type StreamHandler struct {
	responseTransformer *ResponseTransformer
}

// NewStreamHandler creates a new stream handler.
func NewStreamHandler() *StreamHandler {
	return &StreamHandler{
		responseTransformer: NewResponseTransformer(),
	}
}

// ProxyStream takes an OpenAI streaming response and writes Anthropic-format SSE to the writer.
// It reads OpenAI ChatCompletionChunk SSE events and transforms them into Anthropic MessageEvent SSE events.
// The clientCtx is used to detect client disconnection and abort early.
//
// CRITICAL: This function reads directly from resp.Body without buffering to minimize latency.
// Per deep research: "Don't use bufio.Scanner or bufio.Reader on the response body - it adds buffering"
//
// idleTimeout is the maximum gap between bytes on the upstream stream. The
// stream lives as long as data keeps flowing; only an idle period longer than
// idleTimeout is treated as a stuck connection and surfaces as ErrStreamIdle.
// Pass 0 to disable (stream lives until EOF or error).
func (h *StreamHandler) ProxyStream(
	w http.ResponseWriter,
	openaiResp io.ReadCloser,
	originalModel string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by response writer")
	}

	// Generate a unique message ID for this stream.
	msgID := "msg_" + generateID()

	// Send message_start event with the full message envelope.
	msgStart := types.MessageEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   originalModel,
		},
	}
	if err := writeSSEEvent(w, msgStart); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	// Read directly from response body without buffering.
	// Use a tight loop with a line buffer - no bufio.Reader.
	contentIndex := 0
	var lineBuf bytes.Buffer
	contentStarted := false
	reasoningStarted := false
	stopSent := false
	toolUseCount := 0
	startedToolCalls := make(map[int]int) // maps OpenAI tool call index → Anthropic content block index
	decodeErrors := 0                     // consecutive SSE decode failures

	// Read in larger chunks for efficiency, then parse lines
	readBuf := make([]byte, 4096)

	// Start the idle watchdog. Each successful read pings the watchdog so
	// the stream lives as long as data keeps flowing. If no bytes arrive
	// within idleTimeout, cancel() is called, which aborts the upstream
	// HTTP request and causes the next Read to return a context error.
	ping := StartIdleWatchdog(clientCtx, cancel, idleTimeout)

	for {
		// Check if client disconnected
		select {
		case <-clientCtx.Done():
			return ErrClientDisconnected
		default:
		}

		// Read chunk from upstream
		n, err := openaiResp.Read(readBuf)
		if n > 0 {
			// Data is flowing — reset the idle watchdog so the stream
			// lives as long as data keeps arriving.
			ping()
			// Process bytes immediately
			for i := 0; i < n; i++ {
				b := readBuf[i]
				if b == '\n' {
					line := lineBuf.String()
					lineBuf.Reset()

					// Process complete line
					if err := h.processSSELine(w, flusher, line, &contentIndex, &contentStarted, &reasoningStarted, &stopSent, &toolUseCount, startedToolCalls, originalModel, &decodeErrors); err != nil {
						return err
					}
				} else {
					lineBuf.WriteByte(b)
				}
			}
		}

		if err == io.EOF {
			// Process any remaining data in buffer
			if lineBuf.Len() > 0 {
				line := lineBuf.String()
				if err := h.processSSELine(w, flusher, line, &contentIndex, &contentStarted, &reasoningStarted, &stopSent, &toolUseCount, startedToolCalls, originalModel, &decodeErrors); err != nil {
					return err
				}
			}
			break
		}
		if err != nil {
			if IsIdleTimeout(err) {
				return ErrStreamIdle
			}
			// When the idle watchdog fires, it cancels the upstream context
			// which produces context.Canceled on Read. Distinguish that
			// from a client disconnect by checking clientCtx.
			if errors.Is(err, context.Canceled) && clientCtx.Err() == nil {
				return ErrStreamIdle
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	// Close any open content block (text or reasoning)
	if contentStarted || reasoningStarted {
		stopEvent := types.MessageEvent{
			Type:  "content_block_stop",
			Index: &contentIndex,
		}
		if err := writeSSEEvent(w, stopEvent); err != nil {
			return ErrClientDisconnected
		}
		contentStarted = false
		reasoningStarted = false
	}

	// Send stop events for any tool blocks not yet closed (e.g. upstream
	// disconnected without sending a finish_reason chunk).
	if len(startedToolCalls) > 0 {
		type toolBlockEntry struct {
			oi       int
			blockIdx int
		}
		entries := make([]toolBlockEntry, 0, len(startedToolCalls))
		for oi, blockIdx := range startedToolCalls {
			entries = append(entries, toolBlockEntry{oi, blockIdx})
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].blockIdx < entries[j].blockIdx
		})
		for _, e := range entries {
			if err := writeContentBlockStop(w, e.blockIdx); err != nil {
				return ErrClientDisconnected
			}
		}
	}

	// Send message_delta if not already sent.
	// If tool calls were in progress when the stream ended,
	// the stop reason should be "tool_use" rather than "end_turn".
	if !stopSent {
		stopReason := "end_turn"
		if len(startedToolCalls) > 0 {
			stopReason = "tool_use"
		}
		msgDelta := types.MessageEvent{
			Type: "message_delta",
			Delta: &types.Delta{
				StopReason: stopReason,
			},
			Usage: usageInfoToAnthropic(nil),
		}
		if err := writeSSEEvent(w, msgDelta); err != nil {
			return ErrClientDisconnected
		}
		stopSent = true
	}

	// Send message_stop event to signal stream completion.
	stopEvent := types.MessageEvent{
		Type: "message_stop",
	}
	if err := writeSSEEvent(w, stopEvent); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	return nil
}

// processSSELine processes a single SSE line from upstream.
// Per deep research: "Treat SSE primarily as a text protocol" - minimize JSON parsing.
func (h *StreamHandler) processSSELine(
	w http.ResponseWriter,
	flusher http.Flusher,
	line string,
	contentIndex *int,
	contentStarted *bool,
	reasoningStarted *bool,
	stopSent *bool,
	toolUseCount *int,
	startedToolCalls map[int]int,
	originalModel string,
	decodeErrors *int,
) error {
	line = strings.TrimSpace(line)

	// Skip empty lines
	if line == "" {
		return nil
	}

	// Skip non-data lines (event: lines, id: lines, etc.)
	if !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if data == "" {
		return nil
	}

	// Handle [DONE] marker
	if data == "[DONE]" {
		return nil
	}

	// Fast path: check if this is a content chunk without full JSON parsing.
	// Skip the fast path when reasoning_content is also present in the same
	// chunk — falling through to JSON parsing ensures both fields are handled
	// correctly. Otherwise reasoning_content gets silently dropped, and on the
	// next turn DeepSeek rejects the request with:
	//   "The reasoning_content in the thinking mode must be passed back to the API."
	if !strings.Contains(data, `"reasoning_content"`) &&
		!strings.Contains(data, `"finish_reason"`) &&
		!strings.Contains(data, `"tool_calls"`) &&
		!strings.Contains(data, `"usage"`) {
		if idx := strings.Index(data, `"delta":{"content":"`); idx != -1 {
			// Walk past JSON escape sequences to find the real closing
			// quote. A naive strings.Index would stop at an escaped
			// \" inside the content.
			start := idx + len(`"delta":{"content":"`)
			suffix := data[start:]
			end := -1
			for i := 0; i < len(suffix); i++ {
				if suffix[i] == '\\' {
					i++ // skip the escaped character
					continue
				}
				if suffix[i] == '"' {
					end = i
					break
				}
			}
			if end != -1 {
				content := data[start : start+end]
				if content != "" {
					if !*contentStarted {
						// If reasoning was already started, close it first
						if *reasoningStarted {
							if err := writeContentBlockStop(w, *contentIndex); err != nil {
								return ErrClientDisconnected
							}
							*contentIndex++
							*reasoningStarted = false
						}
						*contentStarted = true
						// Send content_block_start
						startEvent := types.MessageEvent{
							Type:         "content_block_start",
							Index:        contentIndex,
							ContentBlock: &types.ContentBlock{Type: "text", Text: ""},
						}
						if err := writeSSEEvent(w, startEvent); err != nil {
							return ErrClientDisconnected
						}
					}

					// Send content_block_delta
					delta := types.Delta{
						Type: "text_delta",
						Text: content,
					}
					event := types.MessageEvent{
						Type:  "content_block_delta",
						Index: contentIndex,
						Delta: &delta,
					}
					if err := writeSSEEvent(w, event); err != nil {
						return ErrClientDisconnected
					}
					flusher.Flush()
				}
				// Valid SSE line accepted via fast path — reset the
				// consecutive decode failure counter so interleaved valid
				// chunks don't accumulate spurious "too many failures".
				*decodeErrors = 0
				return nil
			}
		}
	}

	// For tool calls and other complex cases, fall back to full JSON parsing
	var chunk types.ChatCompletionChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		// Track consecutive decode failures. A transient glitch is tolerated,
		// but persistent corruption terminates the stream rather than silently
		// dropping content.
		*decodeErrors++
		if *decodeErrors > 3 {
			return fmt.Errorf("too many consecutive SSE decode failures (%d)", *decodeErrors)
		}
		return nil
	}
	*decodeErrors = 0

	if len(chunk.Choices) == 0 {
		if chunk.Usage != nil {
			if *stopSent {
				// Stop reason already sent — emit usage-only message_delta (no duplicate stop_reason).
				event := types.MessageEvent{
					Type:  "message_delta",
					Delta: &types.Delta{},
					Usage: usageInfoToAnthropic(chunk.Usage),
				}
				if err := writeSSEEvent(w, event); err != nil {
					return ErrClientDisconnected
				}
				flusher.Flush()
			} else {
				if err := h.sendUsageDelta(w, flusher, chunk.Usage); err != nil {
					return err
				}
				*stopSent = true
			}
		}
		return nil
	}

	choice := chunk.Choices[0]

	// Handle reasoning content deltas
	if choice.Delta.ReasoningContent != nil && *choice.Delta.ReasoningContent != "" {
		if !*reasoningStarted {
			// If text was already started, close it first
			if *contentStarted {
				stopEvent := types.MessageEvent{
					Type:  "content_block_stop",
					Index: contentIndex,
				}
				if err := writeSSEEvent(w, stopEvent); err != nil {
					return ErrClientDisconnected
				}
				*contentIndex++
				*contentStarted = false
			}
			*reasoningStarted = true
			startEvent := types.MessageEvent{
				Type:         "content_block_start",
				Index:        contentIndex,
				ContentBlock: &types.ContentBlock{Type: "thinking", Thinking: ""},
			}
			if err := writeSSEEvent(w, startEvent); err != nil {
				return ErrClientDisconnected
			}
		}

		delta := types.Delta{
			Type:     "thinking_delta",
			Thinking: *choice.Delta.ReasoningContent,
		}
		event := types.MessageEvent{
			Type:  "content_block_delta",
			Index: contentIndex,
			Delta: &delta,
		}
		if err := writeSSEEvent(w, event); err != nil {
			return ErrClientDisconnected
		}
		flusher.Flush()
	}

	// Handle text content deltas
	if textContent := choice.Delta.ContentText(); textContent != "" {
		if !*contentStarted {
			// If reasoning was already started, close it first
			if *reasoningStarted {
				stopEvent := types.MessageEvent{
					Type:  "content_block_stop",
					Index: contentIndex,
				}
				if err := writeSSEEvent(w, stopEvent); err != nil {
					return ErrClientDisconnected
				}
				*contentIndex++
				*reasoningStarted = false
			}
			*contentStarted = true
			startEvent := types.MessageEvent{
				Type:         "content_block_start",
				Index:        contentIndex,
				ContentBlock: &types.ContentBlock{Type: "text", Text: ""},
			}
			if err := writeSSEEvent(w, startEvent); err != nil {
				return ErrClientDisconnected
			}
		}

		delta := types.Delta{
			Type: "text_delta",
			Text: textContent,
		}
		event := types.MessageEvent{
			Type:  "content_block_delta",
			Index: contentIndex,
			Delta: &delta,
		}
		if err := writeSSEEvent(w, event); err != nil {
			return ErrClientDisconnected
		}
		flusher.Flush()
	}

	// Handle tool call deltas.
	// OpenAI streams tool calls incrementally: the first chunk for a given
	// tool call carries id + name (+ possibly empty arguments), subsequent
	// chunks carry only incremental arguments.  We must create exactly one
	// content_block_start per tool call, then stream deltas for it.
	if len(choice.Delta.ToolCalls) > 0 {
		for _, tc := range choice.Delta.ToolCalls {
			oi := tc.Index // OpenAI tool_calls array index

			blockIdx, exists := startedToolCalls[oi]
			if !exists {
				if tc.Function.Name == "" {
					// Ghost chunk: this index was closed and recycled, but
					// has no name/id. Ignore — the real tool call was
					// already fully processed.
					continue
				}
				// Close any existing content/reasoning block before opening the
				// tool block.  Capture the state first so we know whether to
				// advance contentIndex — the close itself clears the flags.
				hadStartedBlock := *contentStarted || *reasoningStarted
				if hadStartedBlock {
					stopEvent := types.MessageEvent{
						Type:  "content_block_stop",
						Index: contentIndex,
					}
					if err := writeSSEEvent(w, stopEvent); err != nil {
						return ErrClientDisconnected
					}
					*contentStarted = false
					*reasoningStarted = false
				}
				// First time seeing this logical tool call — start a new block.
				// Only increment contentIndex when a previous text or reasoning
				// block was already started, OR when a prior tool call has already
				// claimed index 0 (parallel or sequential tool calls).  If nothing
				// was started yet (single-tool response), the first tool block
				// keeps contentIndex at 0 so the Anthropic SSE content block
				// indices are contiguous.
				if hadStartedBlock || len(startedToolCalls) > 0 {
					*contentIndex++
				}
				*toolUseCount++
				blockIdx = *contentIndex
				startedToolCalls[oi] = blockIdx

				toolID := tc.ID
				if toolID == "" {
					toolID = fmt.Sprintf("toolu_%s", generateID())
				}
				startEvent := types.MessageEvent{
					Type:  "content_block_start",
					Index: &blockIdx,
					ContentBlock: &types.ContentBlock{
						Type:  "tool_use",
						ID:    toolID,
						Name:  tc.Function.Name,
						Input: json.RawMessage(`{}`),
					},
				}
				if err := writeSSEEvent(w, startEvent); err != nil {
					return ErrClientDisconnected
				}
			}

			// Send argument delta (if any) — whether new or continuation.
			if tc.Function.Arguments != "" {
				delta := types.Delta{
					Type:        "input_json_delta",
					PartialJSON: tc.Function.Arguments,
				}
				event := types.MessageEvent{
					Type:  "content_block_delta",
					Index: &blockIdx,
					Delta: &delta,
				}
				if err := writeSSEEvent(w, event); err != nil {
					return ErrClientDisconnected
				}
			}
			flusher.Flush()
		}
	}

	// Handle finish reason
	if choice.FinishReason != "" {
		// Close any open content block (reasoning or text)
		if *contentStarted || *reasoningStarted {
			stopEvent := types.MessageEvent{
				Type:  "content_block_stop",
				Index: contentIndex,
			}
			if err := writeSSEEvent(w, stopEvent); err != nil {
				return ErrClientDisconnected
			}
			*contentStarted = false
			*reasoningStarted = false
		}

		// Close any open tool_use blocks in ascending index order.
		if len(startedToolCalls) > 0 {
			type toolBlockEntry struct {
				oi       int
				blockIdx int
			}
			entries := make([]toolBlockEntry, 0, len(startedToolCalls))
			for oi, blockIdx := range startedToolCalls {
				entries = append(entries, toolBlockEntry{oi, blockIdx})
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].blockIdx < entries[j].blockIdx
			})
			for _, e := range entries {
				idx := e.blockIdx
				stopEvent := types.MessageEvent{
					Type:  "content_block_stop",
					Index: &idx,
				}
				if err := writeSSEEvent(w, stopEvent); err != nil {
					return ErrClientDisconnected
				}
			}
			// Clear so EOF cleanup won't emit duplicate stops
			for oi := range startedToolCalls {
				delete(startedToolCalls, oi)
			}
		}
		*toolUseCount = 0

		msgDelta := types.MessageEvent{
			Type: "message_delta",
			Delta: &types.Delta{
				StopReason: h.responseTransformer.mapFinishReason(choice.FinishReason),
			},
			Usage: usageInfoToAnthropic(chunk.Usage),
		}
		if err := writeSSEEvent(w, msgDelta); err != nil {
			return ErrClientDisconnected
		}
		*stopSent = true
		flusher.Flush()
	}

	return nil
}

func (h *StreamHandler) sendUsageDelta(w http.ResponseWriter, flusher http.Flusher, usage *types.UsageInfo) error {
	event := types.MessageEvent{
		Type: "message_delta",
		Delta: &types.Delta{
			StopReason: "end_turn",
		},
		Usage: usageInfoToAnthropic(usage),
	}
	if err := writeSSEEvent(w, event); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()
	return nil
}

func usageInfoToAnthropic(usage *types.UsageInfo) *types.Usage {
	if usage == nil {
		return &types.Usage{
			InputTokens:  0,
			OutputTokens: 0,
		}
	}
	return &types.Usage{
		// Per Anthropic Messages API spec, `input_tokens` is the count of
		// regular input tokens — i.e. tokens that were neither read from the
		// cache nor written to the cache this turn. OpenAI's `prompt_tokens`
		// is the *total* prompt size. We must subtract the cache parts here
		// for the same reason TransformResponse does — see the longer comment
		// in response.go.
		InputTokens:              nonNegative(usage.PromptTokens - usage.PromptCacheHitTokens - usage.PromptCacheMissTokens),
		OutputTokens:             usage.CompletionTokens,
		CacheCreationInputTokens: usage.PromptCacheMissTokens,
		CacheReadInputTokens:     usage.PromptCacheHitTokens,
	}
}

// writeContentBlockStop writes a content_block_stop SSE event at the given index.
func writeContentBlockStop(w http.ResponseWriter, index int) error {
	return writeSSEEvent(w, types.MessageEvent{
		Type:  "content_block_stop",
		Index: &index,
	})
}

// writeSSEEvent writes a single SSE event to the HTTP response writer.
// Format: "event: <type>\ndata: <json>\n\n"
func writeSSEEvent(w http.ResponseWriter, event types.MessageEvent) error {
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	_, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, string(data))
	return err
}

// generateID creates a unique identifier based on current time.
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ProxyResponsesStream takes an OpenAI Responses streaming response and writes Anthropic-format SSE.
func (h *StreamHandler) ProxyResponsesStream(
	w http.ResponseWriter,
	responsesResp io.ReadCloser,
	originalModel string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by response writer")
	}

	msgID := "msg_" + generateID()
	msgStart := types.MessageEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   originalModel,
		},
	}
	if err := writeSSEEvent(w, msgStart); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	contentIndex := 0
	var lineBuf bytes.Buffer
	contentStarted := false
	stopSent := false
	readBuf := make([]byte, 4096)

	ping := StartIdleWatchdog(clientCtx, cancel, idleTimeout)

	for {
		select {
		case <-clientCtx.Done():
			return ErrClientDisconnected
		default:
		}

		n, err := responsesResp.Read(readBuf)
		if n > 0 {
			ping()
			for i := 0; i < n; i++ {
				b := readBuf[i]
				if b == '\n' {
					line := lineBuf.String()
					lineBuf.Reset()
					if err := h.processResponsesSSELine(w, flusher, line, &contentIndex, &contentStarted, &stopSent, originalModel); err != nil {
						return err
					}
				} else {
					lineBuf.WriteByte(b)
				}
			}
		}

		if err == io.EOF {
			if lineBuf.Len() > 0 {
				line := lineBuf.String()
				if err := h.processResponsesSSELine(w, flusher, line, &contentIndex, &contentStarted, &stopSent, originalModel); err != nil {
					return err
				}
			}
			break
		}
		if err != nil {
			if IsIdleTimeout(err) {
				return ErrStreamIdle
			}
			if errors.Is(err, context.Canceled) && clientCtx.Err() == nil {
				return ErrStreamIdle
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	if contentStarted {
		stopEvent := types.MessageEvent{
			Type:  "content_block_stop",
			Index: &contentIndex,
		}
		if err := writeSSEEvent(w, stopEvent); err != nil {
			return ErrClientDisconnected
		}
	}

	if !stopSent {
		msgDelta := types.MessageEvent{
			Type: "message_delta",
			Delta: &types.Delta{
				StopReason: "end_turn",
			},
			Usage: &types.Usage{InputTokens: 0, OutputTokens: 0},
		}
		if err := writeSSEEvent(w, msgDelta); err != nil {
			return ErrClientDisconnected
		}
		stopSent = true
	}

	stopEvent := types.MessageEvent{
		Type: "message_stop",
	}
	if err := writeSSEEvent(w, stopEvent); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	return nil
}

func (h *StreamHandler) processResponsesSSELine(
	w http.ResponseWriter,
	flusher http.Flusher,
	line string,
	contentIndex *int,
	contentStarted *bool,
	stopSent *bool,
	originalModel string,
) error {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if data == "" || data == "[DONE]" {
		return nil
	}

	var chunk types.ResponsesChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil
	}

	if chunk.Type == "response.output_text.delta" && chunk.Delta != "" {
		if !*contentStarted {
			*contentStarted = true
			startEvent := types.MessageEvent{
				Type:         "content_block_start",
				Index:        contentIndex,
				ContentBlock: &types.ContentBlock{Type: "text", Text: ""},
			}
			if err := writeSSEEvent(w, startEvent); err != nil {
				return ErrClientDisconnected
			}
		}

		delta := types.Delta{
			Type: "text_delta",
			Text: chunk.Delta,
		}
		event := types.MessageEvent{
			Type:  "content_block_delta",
			Index: contentIndex,
			Delta: &delta,
		}
		if err := writeSSEEvent(w, event); err != nil {
			return ErrClientDisconnected
		}
		flusher.Flush()
	}

	if chunk.Type == "response.completed" || chunk.Type == "response.done" {
		if !*stopSent {
			msgDelta := types.MessageEvent{
				Type: "message_delta",
				Delta: &types.Delta{
					StopReason: "end_turn",
				},
				Usage: usageInfoToAnthropic(nil),
			}
			if err := writeSSEEvent(w, msgDelta); err != nil {
				return ErrClientDisconnected
			}
			*stopSent = true
			flusher.Flush()
		}
	}

	return nil
}

// ProxyGeminiStream takes a Gemini streaming response and writes Anthropic-format SSE.
func (h *StreamHandler) ProxyGeminiStream(
	w http.ResponseWriter,
	geminiResp io.ReadCloser,
	originalModel string,
	clientCtx context.Context,
	idleTimeout time.Duration,
	cancel context.CancelFunc,
) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported by response writer")
	}

	msgID := "msg_" + generateID()
	msgStart := types.MessageEvent{
		Type: "message_start",
		Message: &types.MessageResponse{
			ID:      msgID,
			Type:    "message",
			Role:    "assistant",
			Content: []types.ContentBlock{},
			Model:   originalModel,
		},
	}
	if err := writeSSEEvent(w, msgStart); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	contentIndex := 0
	var lineBuf bytes.Buffer
	contentStarted := false
	stopSent := false
	readBuf := make([]byte, 4096)

	ping := StartIdleWatchdog(clientCtx, cancel, idleTimeout)

	for {
		select {
		case <-clientCtx.Done():
			return ErrClientDisconnected
		default:
		}

		n, err := geminiResp.Read(readBuf)
		if n > 0 {
			ping()
			for i := 0; i < n; i++ {
				b := readBuf[i]
				if b == '\n' {
					line := lineBuf.String()
					lineBuf.Reset()
					if err := h.processGeminiSSELine(w, flusher, line, &contentIndex, &contentStarted, &stopSent, originalModel); err != nil {
						return err
					}
				} else {
					lineBuf.WriteByte(b)
				}
			}
		}

		if err == io.EOF {
			if lineBuf.Len() > 0 {
				line := lineBuf.String()
				if err := h.processGeminiSSELine(w, flusher, line, &contentIndex, &contentStarted, &stopSent, originalModel); err != nil {
					return err
				}
			}
			break
		}
		if err != nil {
			if IsIdleTimeout(err) {
				return ErrStreamIdle
			}
			if errors.Is(err, context.Canceled) && clientCtx.Err() == nil {
				return ErrStreamIdle
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	if contentStarted {
		stopEvent := types.MessageEvent{
			Type:  "content_block_stop",
			Index: &contentIndex,
		}
		if err := writeSSEEvent(w, stopEvent); err != nil {
			return ErrClientDisconnected
		}
	}

	if !stopSent {
		msgDelta := types.MessageEvent{
			Type: "message_delta",
			Delta: &types.Delta{
				StopReason: "end_turn",
			},
			Usage: &types.Usage{InputTokens: 0, OutputTokens: 0},
		}
		if err := writeSSEEvent(w, msgDelta); err != nil {
			return ErrClientDisconnected
		}
		stopSent = true
	}

	stopEvent := types.MessageEvent{
		Type: "message_stop",
	}
	if err := writeSSEEvent(w, stopEvent); err != nil {
		return ErrClientDisconnected
	}
	flusher.Flush()

	return nil
}

func (h *StreamHandler) processGeminiSSELine(
	w http.ResponseWriter,
	flusher http.Flusher,
	line string,
	contentIndex *int,
	contentStarted *bool,
	stopSent *bool,
	originalModel string,
) error {
	line = strings.TrimSpace(line)
	if line == "" || !strings.HasPrefix(line, "data: ") {
		return nil
	}

	data := strings.TrimPrefix(line, "data: ")
	if data == "" {
		return nil
	}

	var chunk types.GeminiStreamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return nil
	}

	if len(chunk.Candidates) > 0 {
		candidate := chunk.Candidates[0]
		for _, part := range candidate.Content.Parts {
			if part.Text != "" {
				if !*contentStarted {
					*contentStarted = true
					startEvent := types.MessageEvent{
						Type:         "content_block_start",
						Index:        contentIndex,
						ContentBlock: &types.ContentBlock{Type: "text", Text: ""},
					}
					if err := writeSSEEvent(w, startEvent); err != nil {
						return ErrClientDisconnected
					}
				}

				delta := types.Delta{
					Type: "text_delta",
					Text: part.Text,
				}
				event := types.MessageEvent{
					Type:  "content_block_delta",
					Index: contentIndex,
					Delta: &delta,
				}
				if err := writeSSEEvent(w, event); err != nil {
					return ErrClientDisconnected
				}
				flusher.Flush()
			}
		}

		if candidate.FinishReason != "" && !*stopSent {
			if *contentStarted {
				stopEvent := types.MessageEvent{
					Type:  "content_block_stop",
					Index: contentIndex,
				}
				if err := writeSSEEvent(w, stopEvent); err != nil {
					return ErrClientDisconnected
				}
				*contentStarted = false
			}

			stopReason := "end_turn"
			if candidate.FinishReason == "MAX_TOKENS" {
				stopReason = "max_tokens"
			}

			msgDelta := types.MessageEvent{
				Type: "message_delta",
				Delta: &types.Delta{
					StopReason: stopReason,
				},
				Usage: usageInfoToAnthropic(nil),
			}
			if err := writeSSEEvent(w, msgDelta); err != nil {
				return ErrClientDisconnected
			}
			*stopSent = true
			flusher.Flush()
		}
	}

	return nil
}
