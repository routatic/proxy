package transformer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/routatic/proxy/pkg/types"
)

// mockResponseWriter implements http.ResponseWriter and http.Flusher for testing.
type mockResponseWriter struct {
	buf    bytes.Buffer
	header http.Header
	status int
}

func newMockResponseWriter() *mockResponseWriter {
	return &mockResponseWriter{
		header: make(http.Header),
	}
}

func (m *mockResponseWriter) Header() http.Header         { return m.header }
func (m *mockResponseWriter) Write(p []byte) (int, error) { return m.buf.Write(p) }
func (m *mockResponseWriter) WriteHeader(statusCode int)  { m.status = statusCode }
func (m *mockResponseWriter) Flush()                      {}

// sseLines builds raw SSE body from a list of data payloads.
func sseLines(lines ...string) io.ReadCloser {
	var b strings.Builder
	for _, line := range lines {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteString("\n\n")
	}
	return io.NopCloser(strings.NewReader(b.String()))
}

// parseSSEEvents parses the raw response buffer into a slice of MessageEvent.
func parseSSEEvents(t *testing.T, raw string) []types.MessageEvent {
	t.Helper()
	var events []types.MessageEvent
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data: ") {
			data := strings.TrimPrefix(line, "data: ")
			if data == "" || data == "[DONE]" {
				continue
			}
			var ev types.MessageEvent
			if err := json.Unmarshal([]byte(data), &ev); err != nil {
				t.Fatalf("unmarshal SSE event: %v (data: %s)", err, data)
			}
			events = append(events, ev)
		}
	}
	return events
}

func TestEmitMessageResponse_SynthesizesAnthropicSSE(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	resp := &types.MessageResponse{
		ID:         "msg_test",
		Type:       "message",
		Role:       "assistant",
		Model:      "qwen3.6-plus",
		StopReason: "end_turn",
		Content: []types.ContentBlock{
			{Type: "text", Text: "Vedo uno screenshot."},
		},
		Usage: types.Usage{InputTokens: 10, OutputTokens: 4},
	}

	if err := handler.EmitMessageResponse(w, resp); err != nil {
		t.Fatalf("EmitMessageResponse error: %v", err)
	}
	events := parseSSEEvents(t, w.buf.String())
	if len(events) != 6 {
		t.Fatalf("events = %d, want 6: %+v", len(events), events)
	}
	if events[0].Type != "message_start" {
		t.Fatalf("event[0] = %s, want message_start", events[0].Type)
	}
	if events[2].Type != "content_block_delta" || events[2].Delta.Type != "text_delta" {
		t.Fatalf("event[2] = %+v, want text_delta", events[2])
	}
	if got, want := events[2].Delta.Text, "Vedo uno screenshot."; got != want {
		t.Fatalf("text delta = %q, want %q", got, want)
	}
	if events[4].Type != "message_delta" || events[5].Type != "message_stop" {
		t.Fatalf("tail events = %+v %+v, want message_delta/message_stop", events[4], events[5])
	}
}

func TestProxyStream_ReasoningContentFastPath(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"reasoning_content":"Let me think"}}]}`,
		`{"choices":[{"delta":{"reasoning_content":" step by step"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected: message_start, content_block_start, 2x thinking_delta,
	// signature_delta, content_block_stop, message_delta, message_stop.
	if len(events) != 8 {
		t.Fatalf("expected 8 events, got %d: %+v", len(events), events)
	}

	if events[0].Type != "message_start" {
		t.Errorf("event[0].Type = %q, want message_start", events[0].Type)
	}
	if events[1].Type != "content_block_start" {
		t.Errorf("event[1].Type = %q, want content_block_start", events[1].Type)
	}
	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "thinking" {
		t.Errorf("event[1].ContentBlock = %+v, want thinking block", events[1].ContentBlock)
	}
	if events[2].Type != "content_block_delta" {
		t.Errorf("event[2].Type = %q, want content_block_delta", events[2].Type)
	}
	if got := events[2].Delta.Type; got != "thinking_delta" {
		t.Errorf("event[2].Delta.Type = %q, want thinking_delta", got)
	}
	if got := events[2].Delta.Thinking; got != "Let me think" {
		t.Errorf("event[2].Delta.Thinking = %q, want %q", got, "Let me think")
	}
	if events[3].Type != "content_block_delta" {
		t.Errorf("event[3].Type = %q, want content_block_delta", events[3].Type)
	}
	if got := events[3].Delta.Thinking; got != " step by step" {
		t.Errorf("event[3].Delta.Thinking = %q, want %q", got, " step by step")
	}
	// The start block opens unsigned; the signature arrives as signature_delta
	// (event[4]) and accumulates onto the block.
	if got := events[1].ContentBlock.Signature; got != "" {
		t.Errorf("event[1].ContentBlock.Signature = %q, want empty", got)
	}
	if events[4].Type != "content_block_delta" || events[4].Delta == nil || events[4].Delta.Type != "signature_delta" {
		t.Errorf("event[4] = %+v, want signature_delta", events[4])
	}
	if got := events[4].Delta.Signature; got != thinkingSignaturePlaceholder {
		t.Errorf("event[4].Delta.Signature = %q, want %q", got, thinkingSignaturePlaceholder)
	}
	if events[5].Type != "content_block_stop" {
		t.Errorf("event[5].Type = %q, want content_block_stop", events[5].Type)
	}
	if events[6].Type != "message_delta" {
		t.Errorf("event[6].Type = %q, want message_delta", events[6].Type)
	}
	if events[7].Type != "message_stop" {
		t.Errorf("event[7].Type = %q, want message_stop", events[7].Type)
	}
}

func TestProxyStream_ReasoningSignatureAndMergedMessageDelta(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"reasoning_content":"Thinking..."}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d: %+v", len(events), events)
	}
	if events[3].Delta == nil || events[3].Delta.Type != "signature_delta" || events[3].Delta.Signature == "" {
		t.Fatalf("event[3] = %+v, want non-empty signature_delta", events[3])
	}
	if events[4].Type != "content_block_stop" {
		t.Fatalf("event[4] = %+v, want content_block_stop", events[4])
	}
	if events[5].Type != "message_delta" || events[5].Delta == nil || events[5].Delta.StopReason != "end_turn" {
		t.Fatalf("event[5] = %+v, want terminal message_delta", events[5])
	}
	if events[5].Usage == nil || events[5].Usage.InputTokens != 100 || events[5].Usage.OutputTokens != 20 {
		t.Fatalf("event[5].Usage = %+v, want input=100 output=20", events[5].Usage)
	}
}

func TestProxyStream_ReasoningThenText(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"reasoning_content":"Thinking..."}}]}`,
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected: message_start, content_block_start(thinking, idx=0), thinking_delta,
	// signature_delta, content_block_stop(idx=0), content_block_start(text, idx=1),
	// text_delta x2, content_block_stop(idx=1), message_delta, message_stop.
	if len(events) != 11 {
		t.Fatalf("expected 11 events, got %d: %+v", len(events), events)
	}

	// Verify indexes
	if got := *events[1].Index; got != 0 {
		t.Errorf("thinking start index = %d, want 0", got)
	}
	if got := *events[4].Index; got != 0 {
		t.Errorf("thinking stop index = %d, want 0", got)
	}
	if got := *events[5].Index; got != 1 {
		t.Errorf("text start index = %d, want 1", got)
	}
	if got := *events[8].Index; got != 1 {
		t.Errorf("text stop index = %d, want 1", got)
	}

	// Verify types
	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "thinking" {
		t.Errorf("event[1].ContentBlock = %+v, want thinking block", events[1].ContentBlock)
	}
	if got := events[2].Delta.Type; got != "thinking_delta" {
		t.Errorf("event[2].Delta.Type = %q, want thinking_delta", got)
	}
	if events[3].Delta == nil || events[3].Delta.Type != "signature_delta" {
		t.Errorf("event[3] = %+v, want signature_delta", events[3])
	}
	if events[5].ContentBlock == nil || events[5].ContentBlock.Type != "text" {
		t.Errorf("event[5].ContentBlock = %+v, want text block", events[5].ContentBlock)
	}
	if got := events[6].Delta.Type; got != "text_delta" {
		t.Errorf("event[6].Delta.Type = %q, want text_delta", got)
	}
}

func TestProxyStream_TextOnlyStillWorks(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected: message_start, content_block_start, 2x content_block_delta, content_block_stop, message_delta, message_stop
	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "text" {
		t.Errorf("event[1] = %+v, want content_block_start(text)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta.Type != "text_delta" {
		t.Errorf("event[2] = %+v, want content_block_delta(text_delta)", events[2])
	}
	if events[2].Delta.Text != "Hello" {
		t.Errorf("event[2].Delta.Text = %q, want Hello", events[2].Delta.Text)
	}
}

func TestProxyStream_ContentArrayTextDelta(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":[{"type":"text","text":"Vedo uno screenshot."}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "qwen3.6-plus", ctx, 5*time.Second, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}
	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "text" {
		t.Errorf("event[1] = %+v, want content_block_start(text)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta.Type != "text_delta" {
		t.Errorf("event[2] = %+v, want content_block_delta(text_delta)", events[2])
	}
	if got, want := events[2].Delta.Text, "Vedo uno screenshot."; got != want {
		t.Errorf("event[2].Delta.Text = %q, want %q", got, want)
	}
}

func TestProxyStream_UsageOnlyChunk(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":123,"completion_tokens":45,"total_tokens":168,"prompt_cache_hit_tokens":100,"prompt_cache_miss_tokens":23}}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "deepseek-v4-pro", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	var usage *types.Usage
	for _, event := range events {
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if usage == nil {
		t.Fatalf("no usage event found in stream: %+v", events)
		return
	}
	// Per Anthropic spec, input_tokens excludes cache reads AND cache
	// creations. Upstream prompt_tokens=123 split as 100 hit + 23 miss
	// means everything was accounted for by the cache → input_tokens = 0.
	if got, want := usage.InputTokens, 0; got != want {
		t.Fatalf("InputTokens = %d, want %d", got, want)
	}
	if got, want := usage.OutputTokens, 45; got != want {
		t.Fatalf("OutputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheReadInputTokens, 100; got != want {
		t.Fatalf("CacheReadInputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheCreationInputTokens, 23; got != want {
		t.Fatalf("CacheCreationInputTokens = %d, want %d", got, want)
	}
}

// TestProxyStream_PartialCacheTokensStreaming covers the case where
// hit + miss < prompt_tokens in a streaming context. The leftover tokens
// should map to input_tokens.
func TestProxyStream_PartialCacheTokensStreaming(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Partial cache"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":5,"total_tokens":105,"prompt_cache_hit_tokens":60,"prompt_cache_miss_tokens":30}}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "deepseek-v4-pro", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	var usage *types.Usage
	for _, event := range events {
		if event.Usage != nil {
			usage = event.Usage
		}
	}
	if usage == nil {
		t.Fatalf("no usage event found in stream: %+v", events)
		return
	}
	// 100 - 60 - 30 = 10 tokens are neither cached nor newly cached.
	if got, want := usage.InputTokens, 10; got != want {
		t.Errorf("InputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheReadInputTokens, 60; got != want {
		t.Errorf("CacheReadInputTokens = %d, want %d", got, want)
	}
	if got, want := usage.CacheCreationInputTokens, 30; got != want {
		t.Errorf("CacheCreationInputTokens = %d, want %d", got, want)
	}
}

// TestProxyStream_NoDuplicateMessageDelta verifies that finish_reason and usage
// arriving in separate chunks are merged into exactly one message_delta.
func TestProxyStream_NoDuplicateMessageDelta(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":20,"total_tokens":120}}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "deepseek-v4-pro", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	var messageDeltas []types.MessageEvent
	for _, ev := range events {
		if ev.Type == "message_delta" {
			messageDeltas = append(messageDeltas, ev)
		}
	}

	if len(messageDeltas) != 1 {
		t.Fatalf("expected exactly 1 message_delta, got %d: %+v", len(messageDeltas), messageDeltas)
	}
	if messageDeltas[0].Delta == nil || messageDeltas[0].Delta.StopReason != "end_turn" {
		t.Fatalf("message_delta = %+v, want stop_reason=end_turn", messageDeltas[0])
	}

	totalUsage := messageDeltas[0].Usage
	if totalUsage == nil {
		t.Fatalf("no usage found in stream: %+v", events)
		return
	}
	if got, want := totalUsage.InputTokens, 100; got != want {
		t.Errorf("InputTokens = %d, want %d", got, want)
	}
}

func TestProxyStream_ReasoningJSONFallback(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	// This payload does NOT match the fast-path string pattern because of extra whitespace,
	// forcing the JSON fallback path.
	body := sseLines(
		fmt.Sprintf(`{"choices":[{"delta":%s}]}`, mustJSON(t, types.ChatMessage{ReasoningContent: strPtr("Reasoning via JSON")})),
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected: message_start, thinking start/delta/signature/stop, message_delta, message_stop.
	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "thinking" {
		t.Errorf("event[1] = %+v, want content_block_start(thinking)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta.Type != "thinking_delta" {
		t.Errorf("event[2] = %+v, want content_block_delta(thinking_delta)", events[2])
	}
	if events[2].Delta.Thinking != "Reasoning via JSON" {
		t.Errorf("event[2].Delta.Thinking = %q, want %q", events[2].Delta.Thinking, "Reasoning via JSON")
	}
	if events[3].Delta == nil || events[3].Delta.Type != "signature_delta" {
		t.Errorf("event[3] = %+v, want signature_delta", events[3])
	}
}

func TestProxyStream_EmptyReasoningContentSkipped(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		fmt.Sprintf(`{"choices":[{"delta":%s}]}`, mustJSON(t, types.ChatMessage{ReasoningContent: strPtr("")})),
		`{"choices":[{"delta":{"content":"Only text"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Empty reasoning should be skipped; only one text chunk -> 6 events total
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "text" {
		t.Errorf("event[1] = %+v, want content_block_start(text)", events[1])
	}
	if *events[1].Index != 0 {
		t.Errorf("text start index = %d, want 0", *events[1].Index)
	}
}

func TestProxyStream_ReasoningAndContentInSameChunk(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		fmt.Sprintf(`{"choices":[{"delta":%s}]}`, mustJSON(t, types.ChatMessage{
			ReasoningContent: strPtr("Thinking..."),
			Content:          contentText("Hello"),
		})),
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// message_start + thinking_start + thinking_delta + signature_delta + thinking_stop +
	// text_start + text_delta("Hello") + text_delta(" world") + text_stop +
	// message_delta + message_stop = 11
	if len(events) != 11 {
		t.Fatalf("expected 11 events, got %d: %+v", len(events), events)
	}

	// Block 0: thinking
	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "thinking" {
		t.Errorf("event[1] = %+v, want content_block_start(thinking)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta.Type != "thinking_delta" {
		t.Errorf("event[2] = %+v, want content_block_delta(thinking_delta)", events[2])
	}
	if events[2].Delta.Thinking != "Thinking..." {
		t.Errorf("event[2].Delta.Thinking = %q, want %q", events[2].Delta.Thinking, "Thinking...")
	}
	if events[3].Delta == nil || events[3].Delta.Type != "signature_delta" {
		t.Errorf("event[3] = %+v, want signature_delta", events[3])
	}
	if events[4].Type != "content_block_stop" {
		t.Errorf("event[4].Type = %q, want content_block_stop", events[4].Type)
	}

	// Block 1: text
	if events[5].Type != "content_block_start" || events[5].ContentBlock == nil || events[5].ContentBlock.Type != "text" {
		t.Errorf("event[5] = %+v, want content_block_start(text)", events[5])
	}
	if events[6].Type != "content_block_delta" || events[6].Delta.Type != "text_delta" {
		t.Errorf("event[6] = %+v, want content_block_delta(text_delta)", events[6])
	}
	if events[6].Delta.Text != "Hello" {
		t.Errorf("event[6].Delta.Text = %q, want Hello", events[6].Delta.Text)
	}
	if events[7].Type != "content_block_delta" || events[7].Delta.Type != "text_delta" {
		t.Errorf("event[7] = %+v, want content_block_delta(text_delta)", events[7])
	}
	if events[7].Delta.Text != " world" {
		t.Errorf("event[7].Delta.Text = %q, want \" world\"", events[7].Delta.Text)
	}
	if events[8].Type != "content_block_stop" {
		t.Errorf("event[8].Type = %q, want content_block_stop", events[8].Type)
	}
}

// TestProxyStream_ReasoningBeforeContentFastPathRegression ensures that when
// a provider sends reasoning_content BEFORE content in the same delta (with no
// role field), the fast path for content is skipped and reasoning_content is
// not silently dropped. If it were dropped, the next turn would fail on
// DeepSeek with "reasoning_content must be passed back".
func TestProxyStream_ReasoningBeforeContentFastPathRegression(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	// Hand-crafted JSON: reasoning_content appears before content, no role field.
	// Before the fix, the fast path matched "delta":{"content":" and returned
	// early, discarding reasoning_content entirely.
	body := sseLines(
		`{"choices":[{"delta":{"reasoning_content":"Thinking...","content":"Hello"}}]}`,
		`{"choices":[{"delta":{"content":" world"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "deepseek-v4-flash", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// message_start + thinking_start + thinking_delta + signature_delta + thinking_stop +
	// text_start + text_delta("Hello") + text_delta(" world") + text_stop +
	// message_delta + message_stop = 11
	if len(events) != 11 {
		t.Fatalf("expected 11 events, got %d: %+v", len(events), events)
	}

	// Block 0: thinking (must NOT be lost)
	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "thinking" {
		t.Errorf("event[1] = %+v, want content_block_start(thinking)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta.Type != "thinking_delta" {
		t.Errorf("event[2] = %+v, want content_block_delta(thinking_delta)", events[2])
	}
	if events[2].Delta.Thinking != "Thinking..." {
		t.Errorf("event[2].Delta.Thinking = %q, want %q", events[2].Delta.Thinking, "Thinking...")
	}
	if events[3].Delta == nil || events[3].Delta.Type != "signature_delta" {
		t.Errorf("event[3] = %+v, want signature_delta", events[3])
	}

	// Block 1: text
	if events[5].Type != "content_block_start" || events[5].ContentBlock == nil || events[5].ContentBlock.Type != "text" {
		t.Errorf("event[5] = %+v, want content_block_start(text)", events[5])
	}
	if events[6].Delta.Text != "Hello" {
		t.Errorf("event[6].Delta.Text = %q, want Hello", events[6].Delta.Text)
	}
}

// TestProxyStream_ToolCallFinishReasonWithUsage verifies that when finish_reason
// arrives (fast path) followed by a usage-only chunk, tool blocks are closed
// exactly once — no duplicate content_block_stop from EOF cleanup.
func TestProxyStream_ToolCallFinishReasonWithUsage(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_a","type":"function","function":{"name":"fn_a","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"toolu_b","type":"function","function":{"name":"fn_b","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"y\":2}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
		`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Count content_block_stop events — should be exactly 2 (one per tool)
	var stopCount int
	for _, ev := range events {
		if ev.Type == "content_block_stop" {
			stopCount++
		}
	}
	if stopCount != 2 {
		t.Fatalf("expected 2 content_block_stop events, got %d: %+v", stopCount, events)
	}

	// Verify usage is present
	var hasUsage bool
	for _, ev := range events {
		if ev.Usage != nil {
			hasUsage = true
		}
	}
	if !hasUsage {
		t.Error("expected usage in stream, found none")
	}
}

// TestProxyStream_SingleToolCall verifies a single tool call streamed
// incrementally produces exactly one content_block_start, argument deltas,
// and a content_block_stop.
func TestProxyStream_SingleToolCall(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_abc","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"ation\":\"NYC\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected: message_start, tool_start(idx=0), 2x input_json_delta,
	// tool_stop(idx=0), message_delta, message_stop = 7
	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d: %+v", len(events), events)
	}

	// Verify tool_use block start
	if events[1].Type != "content_block_start" {
		t.Errorf("event[1].Type = %q, want content_block_start", events[1].Type)
	}
	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "tool_use" {
		t.Errorf("event[1].ContentBlock = %+v, want tool_use", events[1].ContentBlock)
	}
	if events[1].ContentBlock.ID != "toolu_abc" {
		t.Errorf("event[1].ContentBlock.ID = %q, want toolu_abc", events[1].ContentBlock.ID)
	}
	if events[1].ContentBlock.Name != "get_weather" {
		t.Errorf("event[1].ContentBlock.Name = %q, want get_weather", events[1].ContentBlock.Name)
	}

	// Verify argument deltas
	if events[2].Delta == nil || events[2].Delta.Type != "input_json_delta" {
		t.Errorf("event[2] = %+v, want input_json_delta", events[2])
	}
	if events[2].Delta.PartialJSON != `{"loc` {
		t.Errorf("event[2].Delta.PartialJSON = %q, want %q", events[2].Delta.PartialJSON, `{"loc`)
	}
	if events[3].Delta == nil || events[3].Delta.Type != "input_json_delta" {
		t.Errorf("event[3] = %+v, want input_json_delta", events[3])
	}

	// Verify tool block stop
	if events[4].Type != "content_block_stop" {
		t.Errorf("event[4].Type = %q, want content_block_stop", events[4].Type)
	}

	// Verify stop reason
	if events[5].Type != "message_delta" {
		t.Errorf("event[5].Type = %q, want message_delta", events[5].Type)
	}
	if events[5].Delta == nil || events[5].Delta.StopReason != "tool_use" {
		t.Errorf("event[5].Delta.StopReason = %q, want tool_use", events[5].Delta.StopReason)
	}
	if events[6].Type != "message_stop" {
		t.Errorf("event[6].Type = %q, want message_stop", events[6].Type)
	}
}

// TestProxyStream_MultipleParallelToolCalls verifies that two concurrent tool
// calls produce two content_block_start events, each with their own argument
// deltas, and that content_block_stop events are emitted in ascending index
// order (not random map iteration order).
func TestProxyStream_MultipleParallelToolCalls(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	// Two tool calls: index 0 and index 1, interleaved as OpenAI sends them
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_1","type":"function","function":{"name":"search","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"id":"toolu_2","type":"function","function":{"name":"lookup","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"q"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"{\"id"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"uery\":\"go\"}"}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":1,"function":{"arguments":"\":\"42\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Count content_block_start events (should be exactly 2)
	var startEvents []types.MessageEvent
	for _, ev := range events {
		if ev.Type == "content_block_start" {
			startEvents = append(startEvents, ev)
		}
	}
	if len(startEvents) != 2 {
		t.Fatalf("expected 2 content_block_start events, got %d", len(startEvents))
	}

	// Both should be tool_use blocks
	for i, se := range startEvents {
		if se.ContentBlock == nil || se.ContentBlock.Type != "tool_use" {
			t.Errorf("start event[%d].ContentBlock = %+v, want tool_use", i, se.ContentBlock)
		}
	}
	if startEvents[0].ContentBlock.Name != "search" {
		t.Errorf("first tool name = %q, want search", startEvents[0].ContentBlock.Name)
	}
	if startEvents[1].ContentBlock.Name != "lookup" {
		t.Errorf("second tool name = %q, want lookup", startEvents[1].ContentBlock.Name)
	}

	// Count content_block_stop events (should be exactly 2)
	var stopIndices []int
	for _, ev := range events {
		if ev.Type == "content_block_stop" && ev.Index != nil {
			stopIndices = append(stopIndices, *ev.Index)
		}
	}
	if len(stopIndices) != 2 {
		t.Fatalf("expected 2 content_block_stop events, got %d", len(stopIndices))
	}
	// Verify ascending order
	if stopIndices[0] >= stopIndices[1] {
		t.Errorf("stop indices not ascending: %v", stopIndices)
	}
}

// TestProxyStream_ToolCallGhostChunk verifies that a ghost chunk (tool call
// index with empty name) is ignored and does not produce a content_block_start.
func TestProxyStream_ToolCallGhostChunk(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_a","type":"function","function":{"name":"real_func","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"x\":1}"}}]}}]}`,
		// Ghost chunk: index 0 recycled but no name
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":""}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Should have exactly 1 content_block_start for the real tool call
	var startEvents []types.MessageEvent
	for _, ev := range events {
		if ev.Type == "content_block_start" {
			startEvents = append(startEvents, ev)
		}
	}
	if len(startEvents) != 1 {
		t.Fatalf("expected 1 content_block_start, got %d: %+v", len(startEvents), startEvents)
	}
}

// TestProxyStream_MixedTextAndToolCall verifies a response that starts with
// text content and then transitions to a tool call.
func TestProxyStream_MixedTextAndToolCall(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Let me check that for you."}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_x","type":"function","function":{"name":"get_data","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"id\":1}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Verify text block at index 0
	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "text" {
		t.Errorf("event[1] = %+v, want content_block_start(text)", events[1])
	}
	if *events[1].Index != 0 {
		t.Errorf("text start index = %d, want 0", *events[1].Index)
	}

	// Verify tool_use block at index 1
	if events[4].Type != "content_block_start" || events[4].ContentBlock == nil || events[4].ContentBlock.Type != "tool_use" {
		t.Errorf("event[4] = %+v, want content_block_start(tool_use)", events[4])
	}
	if *events[4].Index != 1 {
		t.Errorf("tool start index = %d, want 1", *events[4].Index)
	}
	if events[4].ContentBlock.Name != "get_data" {
		t.Errorf("tool name = %q, want get_data", events[4].ContentBlock.Name)
	}

	var stopIndices []int
	for _, ev := range events {
		if ev.Type == "content_block_stop" && ev.Index != nil {
			stopIndices = append(stopIndices, *ev.Index)
		}
	}
	if len(stopIndices) != 2 {
		t.Fatalf("expected text and tool blocks to be stopped exactly once, got %v", stopIndices)
	}
	if stopIndices[0] != 0 || stopIndices[1] != 1 {
		t.Fatalf("stop indices = %v, want [0 1]", stopIndices)
	}
}

// TestProxyStream_MixedReasoningAndToolCall verifies that a reasoning block is
// closed before the stream starts a tool_use block.
func TestProxyStream_MixedReasoningAndToolCall(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		fmt.Sprintf(`{"choices":[{"delta":%s}]}`, mustJSON(t, types.ChatMessage{ReasoningContent: strPtr("Need a tool.")})),
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_x","type":"function","function":{"name":"get_data","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"id\":1}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "thinking" {
		t.Errorf("event[1] = %+v, want content_block_start(thinking)", events[1])
	}
	if events[3].Delta == nil || events[3].Delta.Type != "signature_delta" {
		t.Errorf("event[3] = %+v, want signature_delta", events[3])
	}
	if events[4].Type != "content_block_stop" || events[4].Index == nil || *events[4].Index != 0 {
		t.Errorf("event[4] = %+v, want content_block_stop(index=0)", events[4])
	}
	if events[5].Type != "content_block_start" || events[5].ContentBlock == nil || events[5].ContentBlock.Type != "tool_use" {
		t.Errorf("event[5] = %+v, want content_block_start(tool_use)", events[5])
	}

	var stopIndices []int
	for _, ev := range events {
		if ev.Type == "content_block_stop" && ev.Index != nil {
			stopIndices = append(stopIndices, *ev.Index)
		}
	}
	if len(stopIndices) != 2 {
		t.Fatalf("expected reasoning and tool blocks to be stopped exactly once, got %v", stopIndices)
	}
	if stopIndices[0] != 0 || stopIndices[1] != 1 {
		t.Fatalf("stop indices = %v, want [0 1]", stopIndices)
	}
}

// TestProxyStream_ToolCallFinishReasonFastPath verifies that when a tool call
// finish reason arrives in a chunk matching the fast path, the stop reason
// is correctly set to "tool_use".
func TestProxyStream_ToolCallFinishReasonFastPath(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_xyz","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected events: message_start, content_block_start, content_block_stop, message_delta, message_stop = 5
	if len(events) != 5 {
		t.Fatalf("expected 5 events, got %d: %+v", len(events), events)
	}

	// Verify message_delta has StopReason set to tool_use
	msgDelta := events[3]
	if msgDelta.Type != "message_delta" {
		t.Errorf("expected event[3] to be message_delta, got %q", msgDelta.Type)
	}
	if msgDelta.Delta == nil || msgDelta.Delta.StopReason != "tool_use" {
		t.Errorf("stop reason = %q, want tool_use", msgDelta.Delta.StopReason)
	}
}

// TestProxyStream_ContentAndFinishReasonInSameChunk verifies that when a chunk
// contains both a text content delta and a finish reason, both are handled.
func TestProxyStream_ContentAndFinishReasonInSameChunk(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Hello"},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected events:
	// 0: message_start
	// 1: content_block_start (index 0, type text)
	// 2: content_block_delta (index 0, text "Hello")
	// 3: content_block_stop (index 0)
	// 4: message_delta (stop_reason: end_turn)
	// 5: message_stop
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "text" {
		t.Errorf("event[1] = %+v, want content_block_start(text)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta == nil || events[2].Delta.Text != "Hello" {
		t.Errorf("event[2] = %+v, want content_block_delta(Hello)", events[2])
	}
	if events[3].Type != "content_block_stop" || events[3].Index == nil || *events[3].Index != 0 {
		t.Errorf("event[3] = %+v, want content_block_stop(0)", events[3])
	}
	if events[4].Type != "message_delta" || events[4].Delta == nil || events[4].Delta.StopReason != "end_turn" {
		t.Errorf("event[4] = %+v, want message_delta(end_turn)", events[4])
	}
}

// TestProxyStream_ToolCallAndFinishReasonInSameChunk verifies that when a chunk
// contains both a tool call arguments delta and a finish reason, both are handled.
func TestProxyStream_ToolCallAndFinishReasonInSameChunk(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_xyz","type":"function","function":{"name":"get_weather","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"loc\":\"Beijing\"}"}}]},"finish_reason":"tool_calls"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// Expected events:
	// 0: message_start
	// 1: content_block_start (index 1, type tool_use)
	// 2: content_block_delta (index 0, partial_json "{\"loc\":\"Beijing\"}")
	// 3: content_block_stop (index 0)
	// 4: message_delta (stop_reason: tool_use)
	// 5: message_stop
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil || events[1].ContentBlock.Type != "tool_use" {
		t.Errorf("event[1] = %+v, want content_block_start(tool_use)", events[1])
	}
	if events[2].Type != "content_block_delta" || events[2].Delta == nil || events[2].Delta.PartialJSON != `{"loc":"Beijing"}` {
		t.Errorf("event[2] = %+v, want content_block_delta", events[2])
	}
	if events[3].Type != "content_block_stop" || events[3].Index == nil || *events[3].Index != 0 {
		t.Errorf("event[3] = %+v, want content_block_stop(0)", events[3])
	}
	if events[4].Type != "message_delta" || events[4].Delta == nil || events[4].Delta.StopReason != "tool_use" {
		t.Errorf("event[4] = %+v, want message_delta(tool_use)", events[4])
	}
}

func TestProxyStream_NoUsageFallback(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "qwen3.6-plus", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	var messageDeltaEvent *types.MessageEvent
	for _, event := range events {
		if event.Type == "message_delta" {
			messageDeltaEvent = &event
			break
		}
	}

	if messageDeltaEvent == nil {
		t.Fatalf("expected message_delta event, got none: %+v", events)
		return
	}

	if messageDeltaEvent.Usage == nil {
		t.Fatal("expected message_delta event to have non-nil Usage, but it was nil")
		return
	}

	if messageDeltaEvent.Usage.InputTokens != 0 || messageDeltaEvent.Usage.OutputTokens != 0 {
		t.Errorf("Usage = %+v, want InputTokens: 0, OutputTokens: 0", messageDeltaEvent.Usage)
	}
}

func TestProxyStream_NoFinishReasonFallback(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"content":"Hello"}}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "qwen3.6-plus", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	// Expected events:
	// 0: message_start
	// 1: content_block_start
	// 2: content_block_delta
	// 3: content_block_stop
	// 4: message_delta (fallback stop_reason: end_turn)
	// 5: message_stop
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	if events[4].Type != "message_delta" || events[4].Delta == nil || events[4].Delta.StopReason != "end_turn" {
		t.Errorf("event[4] = %+v, want message_delta(end_turn)", events[4])
	}
}

// TestProxyStream_EOFFallbackStopReasonToolUse verifies that when the stream
// ends mid-tool-call (no finish_reason), the EOF fallback sets stop_reason
// to "tool_use" rather than "end_turn".
func TestProxyStream_EOFFallbackStopReasonToolUse(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_abc","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/test\"}"}}]}}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	var msgDelta *types.MessageEvent
	for i := range events {
		if events[i].Type == "message_delta" {
			msgDelta = &events[i]
			break
		}
	}
	if msgDelta == nil {
		t.Fatalf("expected message_delta event, got none: %+v", events)
		return
	}
	if msgDelta.Delta == nil || msgDelta.Delta.StopReason != "tool_use" {
		t.Errorf("stop_reason = %q, want tool_use (stream ended mid-tool-call)", msgDelta.Delta.StopReason)
	}
}

// TestProxyStream_ToolUseFirstContentBlock verifies that when the first
// assistant output is a direct tool call (no preceding text or reasoning),
// the tool_use block is emitted at index 0 per Anthropic SSE spec.
func TestProxyStream_ToolUseFirstContentBlock(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"toolu_abc","type":"function","function":{"name":"read_file","arguments":""}}]}}]}`,
		`{"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{\"path\":\"/tmp/x\"}"}}]}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"tool_use"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// 0: message_start
	// 1: content_block_start (index 0, type tool_use) — first content block
	// 2: content_block_delta (index 0)
	// 3: content_block_stop (index 0)
	// 4: message_delta
	// 5: message_stop
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" {
		t.Fatalf("event[1].Type = %q, want content_block_start", events[1].Type)
	}
	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "tool_use" {
		t.Fatalf("event[1].ContentBlock = %+v, want tool_use", events[1].ContentBlock)
	}
	if events[1].Index == nil || *events[1].Index != 0 {
		t.Fatalf("tool_use content_block_start index = %v, want 0", events[1].Index)
	}

	if events[3].Type != "content_block_stop" || events[3].Index == nil || *events[3].Index != 0 {
		t.Fatalf("tool_use content_block_stop index = %v, want 0", events[3].Index)
	}
	if events[4].Type != "message_delta" || events[4].Delta == nil || events[4].Delta.StopReason != "tool_use" {
		t.Errorf("event[4] = %+v, want message_delta(tool_use)", events[4])
	}
}

// helpers

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func strPtr(s string) *string { return &s }

// bodyThenErrReader yields body on the first Read, then fails with err. It
// simulates an upstream that stalls or drops after delivering a complete
// response (finish_reason already sent).
type bodyThenErrReader struct {
	body []byte
	err  error
	sent bool
}

func (r *bodyThenErrReader) Read(p []byte) (int, error) {
	if !r.sent {
		r.sent = true
		return copy(p, r.body), nil
	}
	return 0, r.err
}

func (r *bodyThenErrReader) Close() error { return nil }

func TestProxyStream_TerminalEventsSurviveUpstreamErrorAfterFinishReason(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "read error", err: io.ErrUnexpectedEOF},
		{name: "idle cancel", err: context.Canceled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := NewStreamHandler()
			w := newMockResponseWriter()
			body := "data: " + `{"choices":[{"delta":{"reasoning_content":"Thinking..."}}]}` + "\n\n" +
				"data: " + `{"choices":[{"delta":{"content":"42"},"finish_reason":"stop"}]}` + "\n\n"

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := handler.ProxyStream(w, &bodyThenErrReader{body: []byte(body), err: tt.err}, "kimi-k2.6", ctx, 0, cancel)
			if err != nil {
				t.Fatalf("ProxyStream() error = %v, want nil (response already complete)", err)
			}

			events := parseSSEEvents(t, w.buf.String())
			if len(events) < 2 {
				t.Fatalf("expected terminal events, got %+v", events)
			}
			last := events[len(events)-1]
			penultimate := events[len(events)-2]
			if last.Type != "message_stop" {
				t.Errorf("last event = %q, want message_stop", last.Type)
			}
			if penultimate.Type != "message_delta" {
				t.Fatalf("penultimate event = %q, want message_delta", penultimate.Type)
			}
			if penultimate.Delta == nil || penultimate.Delta.StopReason != "end_turn" {
				t.Errorf("message_delta.Delta = %+v, want stop_reason end_turn", penultimate.Delta)
			}
		})
	}
}

func TestProxyStream_UpstreamErrorBeforeFinishReasonStillFails(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := "data: " + `{"choices":[{"delta":{"content":"partial"}}]}` + "\n\n"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := handler.ProxyStream(w, &bodyThenErrReader{body: []byte(body), err: io.ErrUnexpectedEOF}, "kimi-k2.6", ctx, 0, cancel)
	if err == nil {
		t.Fatal("ProxyStream() error = nil, want error so the handler can fall back")
	}
}

func TestProxyStream_ThinkingBlockStartHasEmptySignature(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"choices":[{"delta":{"reasoning_content":"Thinking..."}}]}`,
		`{"choices":[{"delta":{},"finish_reason":"stop"}]}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, body, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	if events[1].Type != "content_block_start" || events[1].ContentBlock == nil {
		t.Fatalf("event[1] = %+v, want content_block_start", events[1])
	}
	// Signature deltas accumulate onto the start block, so the start block must
	// not pre-seed the placeholder or the client sees it twice.
	if got := events[1].ContentBlock.Signature; got != "" {
		t.Errorf("content_block_start signature = %q, want empty", got)
	}
}

func TestEmitMessageResponse_ThinkingSignatureOnlyInDelta(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	resp := &types.MessageResponse{
		ID:   "msg_1",
		Type: "message",
		Role: "assistant",
		Content: []types.ContentBlock{
			{Type: "thinking", Thinking: "hmm", Signature: "upstream-sig"},
		},
		Model: "kimi-k2.6",
	}

	if err := handler.EmitMessageResponse(w, resp); err != nil {
		t.Fatalf("EmitMessageResponse error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	var start, sigDelta *types.MessageEvent
	for i := range events {
		switch {
		case events[i].Type == "content_block_start":
			start = &events[i]
		case events[i].Type == "content_block_delta" && events[i].Delta != nil && events[i].Delta.Type == "signature_delta":
			sigDelta = &events[i]
		}
	}
	if start == nil || start.ContentBlock == nil {
		t.Fatalf("no content_block_start in %+v", events)
	}
	if got := start.ContentBlock.Signature; got != "" {
		t.Errorf("content_block_start signature = %q, want empty", got)
	}
	if sigDelta == nil {
		t.Fatalf("no signature_delta in %+v", events)
	}
	if got := sigDelta.Delta.Signature; got != "upstream-sig" {
		t.Errorf("signature_delta = %q, want the upstream signature", got)
	}
}

func TestProxyStream_NoDuplicateToolStopsOnErrorAfterFinishReason(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := "data: " + `{"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"read","arguments":"{}"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyStream(w, &bodyThenErrReader{body: []byte(body), err: io.ErrUnexpectedEOF}, "kimi-k2.6", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyStream() error = %v, want nil", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	stops := 0
	for _, ev := range events {
		if ev.Type == "content_block_stop" {
			stops++
		}
	}
	if stops != 1 {
		t.Errorf("content_block_stop count = %d, want 1: %+v", stops, events)
	}
	last := events[len(events)-1]
	if last.Type != "message_stop" {
		t.Errorf("last event = %q, want message_stop", last.Type)
	}
	if got := events[len(events)-2]; got.Type != "message_delta" || got.Delta == nil || got.Delta.StopReason != "tool_use" {
		t.Errorf("penultimate event = %+v, want message_delta with stop_reason tool_use", got)
	}
}

// TestProxyResponsesStream_ToolCall verifies that a function_call output item in
// a Responses stream becomes an Anthropic tool_use block with incremental
// input_json_delta arguments and stop_reason tool_use.
func TestProxyResponsesStream_ToolCall(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"type":"response.output_item.added","output":[{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"get_weather","arguments":"","status":"in_progress"}]}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"city\":"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"\"Paris\"}"}`,
		`{"type":"response.output_item.done","output":[{"type":"function_call","id":"fc_1","call_id":"call_abc","name":"get_weather","arguments":"{\"city\":\"Paris\"}","status":"completed"}]}`,
		`{"type":"response.completed"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyResponsesStream(w, body, "muse-spark-1.2-contributor", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyResponsesStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// message_start, tool_start(idx=0), 2x input_json_delta, tool_stop(idx=0),
	// message_delta, message_stop = 7
	if len(events) != 7 {
		t.Fatalf("expected 7 events, got %d: %+v", len(events), events)
	}

	if events[1].Type != "content_block_start" {
		t.Errorf("event[1].Type = %q, want content_block_start", events[1].Type)
	}
	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "tool_use" {
		t.Errorf("event[1].ContentBlock = %+v, want tool_use", events[1].ContentBlock)
	}
	if events[1].ContentBlock.ID != "call_abc" {
		t.Errorf("event[1].ContentBlock.ID = %q, want call_abc", events[1].ContentBlock.ID)
	}
	if events[1].ContentBlock.Name != "get_weather" {
		t.Errorf("event[1].ContentBlock.Name = %q, want get_weather", events[1].ContentBlock.Name)
	}
	if got := string(events[1].ContentBlock.Input); got != `{}` {
		t.Errorf("event[1].ContentBlock.Input = %s, want {}", got)
	}
	if events[1].Index == nil || *events[1].Index != 0 {
		t.Errorf("event[1].Index = %v, want 0", events[1].Index)
	}

	if events[2].Delta == nil || events[2].Delta.Type != "input_json_delta" || events[2].Delta.PartialJSON != `{"city":` {
		t.Errorf("event[2] = %+v, want input_json_delta `{\"city\":`", events[2])
	}
	if events[3].Delta == nil || events[3].Delta.Type != "input_json_delta" || events[3].Delta.PartialJSON != `"Paris"}` {
		t.Errorf("event[3] = %+v, want input_json_delta `\"Paris\"}`", events[3])
	}

	if events[4].Type != "content_block_stop" {
		t.Errorf("event[4].Type = %q, want content_block_stop", events[4].Type)
	}

	if events[5].Type != "message_delta" {
		t.Errorf("event[5].Type = %q, want message_delta", events[5].Type)
	}
	if events[5].Delta == nil || events[5].Delta.StopReason != "tool_use" {
		t.Errorf("event[5].Delta.StopReason = %q, want tool_use", events[5].Delta.StopReason)
	}
	if events[6].Type != "message_stop" {
		t.Errorf("event[6].Type = %q, want message_stop", events[6].Type)
	}
}

func TestProxyResponsesStream_OverlappingToolCallsUseDistinctIndices(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"first"}}`,
		`{"type":"response.output_item.added","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"second"}}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_1","delta":"{\"a\":1}"}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_2","delta":"{\"b\":2}"}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_1","call_id":"call_1","name":"first"}}`,
		`{"type":"response.output_item.done","item":{"type":"function_call","id":"fc_2","call_id":"call_2","name":"second"}}`,
		`{"type":"response.completed"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyResponsesStream(w, body, "gpt-5", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyResponsesStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())
	if len(events) != 9 {
		t.Fatalf("expected 9 events, got %d: %+v", len(events), events)
	}

	wantEvents := []struct {
		eventType string
		index     int
		deltaType string
	}{
		{eventType: "content_block_start", index: 0},
		{eventType: "content_block_start", index: 1},
		{eventType: "content_block_delta", index: 0, deltaType: "input_json_delta"},
		{eventType: "content_block_delta", index: 1, deltaType: "input_json_delta"},
		{eventType: "content_block_stop", index: 0},
		{eventType: "content_block_stop", index: 1},
	}
	for i, want := range wantEvents {
		event := events[i+1]
		if event.Type != want.eventType {
			t.Errorf("event[%d].Type = %q, want %q", i+1, event.Type, want.eventType)
		}
		if event.Index == nil || *event.Index != want.index {
			t.Errorf("event[%d].Index = %v, want %d", i+1, event.Index, want.index)
		}
		if want.eventType == "content_block_start" &&
			(event.ContentBlock == nil || event.ContentBlock.Type != "tool_use") {
			t.Errorf("event[%d].ContentBlock = %+v, want tool_use", i+1, event.ContentBlock)
		}
		if want.deltaType != "" &&
			(event.Delta == nil || event.Delta.Type != want.deltaType) {
			t.Errorf("event[%d].Delta = %+v, want %s", i+1, event.Delta, want.deltaType)
		}
	}

	if events[7].Type != "message_delta" ||
		events[7].Delta == nil ||
		events[7].Delta.StopReason != "tool_use" {
		t.Errorf("event[7] = %+v, want message_delta with tool_use stop reason", events[7])
	}
	if events[8].Type != "message_stop" {
		t.Errorf("event[8].Type = %q, want message_stop", events[8].Type)
	}
}

// TestProxyResponsesStream_TextThenToolCall verifies that a text block followed
// by a function_call produces contiguous indices (text=0, tool=1) and that both
// blocks are closed before the terminal message_delta.
func TestProxyResponsesStream_TextThenToolCall(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"type":"response.output_text.delta","delta":"I'll fetch it now."}`,
		`{"type":"response.output_item.added","output":[{"type":"function_call","id":"fc_9","call_id":"call_xyz","name":"search","arguments":"","status":"in_progress"}]}`,
		`{"type":"response.function_call_arguments.delta","item_id":"fc_9","delta":"{\"q\":\"go\"}"}`,
		`{"type":"response.output_item.done","output":[{"type":"function_call","id":"fc_9","call_id":"call_xyz","name":"search","arguments":"{\"q\":\"go\"}","status":"completed"}]}`,
		`{"type":"response.completed"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyResponsesStream(w, body, "muse-spark-1.2-contributor", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyResponsesStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// message_start, text_start(0), text_delta, text_stop(0), tool_start(1),
	// args_delta(1), tool_stop(1), message_delta, message_stop = 9
	if len(events) != 9 {
		t.Fatalf("expected 9 events, got %d: %+v", len(events), events)
	}

	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "text" {
		t.Errorf("event[1] = %+v, want text block start", events[1])
	}
	if events[1].Index == nil || *events[1].Index != 0 {
		t.Errorf("event[1].Index = %v, want 0", events[1].Index)
	}

	if events[3].Type != "content_block_stop" {
		t.Errorf("event[3].Type = %q, want content_block_stop (text close)", events[3].Type)
	}

	if events[4].ContentBlock == nil || events[4].ContentBlock.Type != "tool_use" {
		t.Errorf("event[4] = %+v, want tool_use block start", events[4])
	}
	if events[4].Index == nil || *events[4].Index != 1 {
		t.Errorf("event[4].Index = %v, want 1", events[4].Index)
	}

	if events[5].Delta == nil || events[5].Delta.Type != "input_json_delta" {
		t.Errorf("event[5] = %+v, want input_json_delta", events[5])
	}

	if events[7].Type != "message_delta" || events[7].Delta == nil || events[7].Delta.StopReason != "tool_use" {
		t.Errorf("event[7] = %+v, want message_delta stop_reason tool_use", events[7])
	}
}

// TestProxyResponsesStream_TextOnlyStillWorks guards the pre-existing text-only
// behavior: stop_reason stays end_turn and no tool blocks are emitted.
func TestProxyResponsesStream_TextOnlyStillWorks(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"type":"response.output_text.delta","delta":"Four"}`,
		`{"type":"response.completed"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyResponsesStream(w, body, "muse-spark-1.2-contributor", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyResponsesStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// message_start, text_start, text_delta, text_stop, message_delta, message_stop = 6
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}
	if events[4].Type != "message_delta" || events[4].Delta == nil || events[4].Delta.StopReason != "end_turn" {
		t.Errorf("event[4] = %+v, want message_delta stop_reason end_turn", events[4])
	}
}

// TestProxyResponsesStream_ToolCall_ItemField matches the event shape opencode.ai's
// Responses endpoint actually sends: output_item events carry the item in the
// "item" field (not an "output" array), and arguments arrive via
// function_call_arguments.delta keyed by item_id.
func TestProxyResponsesStream_ToolCall_ItemField(t *testing.T) {
	handler := NewStreamHandler()
	w := newMockResponseWriter()
	body := sseLines(
		`{"type":"response.output_item.added","sequence_number":4,"output_index":2,"item":{"id":"fc_abc","type":"function_call","status":"in_progress","name":"get_weather","call_id":"call_xyz"}}`,
		`{"type":"response.function_call_arguments.delta","sequence_number":11,"output_index":2,"item_id":"fc_abc","delta":"{\"city\":\"Paris\"}"}`,
		`{"type":"response.output_item.done","sequence_number":13,"output_index":2,"item":{"id":"fc_abc","type":"function_call","status":"completed","name":"get_weather","call_id":"call_xyz","arguments":"{\"city\":\"Paris\"}"}}`,
		`{"type":"response.completed"}`,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := handler.ProxyResponsesStream(w, body, "muse-spark-1.2-contributor", ctx, 0, cancel); err != nil {
		t.Fatalf("ProxyResponsesStream error: %v", err)
	}

	events := parseSSEEvents(t, w.buf.String())

	// message_start, tool_start(idx=0), args_delta, tool_stop(idx=0),
	// message_delta, message_stop = 6
	if len(events) != 6 {
		t.Fatalf("expected 6 events, got %d: %+v", len(events), events)
	}
	if events[1].ContentBlock == nil || events[1].ContentBlock.Type != "tool_use" {
		t.Fatalf("event[1] = %+v, want tool_use block start", events[1])
	}
	if events[1].ContentBlock.ID != "call_xyz" || events[1].ContentBlock.Name != "get_weather" {
		t.Errorf("event[1] tool_use = %+v, want id call_xyz name get_weather", events[1].ContentBlock)
	}
	if events[2].Delta == nil || events[2].Delta.Type != "input_json_delta" || events[2].Delta.PartialJSON != `{"city":"Paris"}` {
		t.Errorf("event[2] = %+v, want input_json_delta `{\"city\":\"Paris\"}`", events[2])
	}
	if events[3].Type != "content_block_stop" {
		t.Errorf("event[3].Type = %q, want content_block_stop", events[3].Type)
	}
	if events[4].Delta == nil || events[4].Delta.StopReason != "tool_use" {
		t.Errorf("event[4] = %+v, want message_delta stop_reason tool_use", events[4])
	}
}
