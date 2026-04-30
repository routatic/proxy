package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"oc-go-cc/internal/metrics"
	"oc-go-cc/internal/token"
)

func TestHandleCountTokensSupportsAnthropicContentBlocks(t *testing.T) {
	handler := newTestHealthHandler(t)

	body := []byte(`{
		"model":"deepseek-v4-pro",
		"messages":[{"role":"user","content":[{"type":"text","text":"hello world"}]}]
	}`)
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))

	handler.HandleCountTokens(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body: %s", got, want, recorder.Body.String())
	}

	var response map[string]int
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is invalid JSON: %v", err)
	}
	if response["input_tokens"] <= 0 {
		t.Fatalf("input_tokens = %d, want positive", response["input_tokens"])
	}
	if got, want := response["token_count"], response["input_tokens"]; got != want {
		t.Fatalf("token_count = %d, want %d", got, want)
	}
}

func TestHandleCountTokensIncludesSystemToolsAndThinking(t *testing.T) {
	handler := newTestHealthHandler(t)

	base := countTokensForTest(t, handler, []byte(`{
		"model":"deepseek-v4-pro",
		"messages":[{"role":"user","content":[{"type":"text","text":"hello"}]}]
	}`))

	withContext := countTokensForTest(t, handler, []byte(`{
		"model":"deepseek-v4-pro",
		"system":[{"type":"text","text":"You are helpful"}],
		"tools":[{"name":"read_file","description":"Read a file","input_schema":{"type":"object","properties":{"path":{"type":"string"}}}}],
		"messages":[
			{"role":"assistant","content":[{"type":"thinking","thinking":"Need to inspect files"},{"type":"tool_use","id":"toolu_1","name":"read_file","input":{"path":"README.md"}}]},
			{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"file contents"},{"type":"text","text":"continue"}]}
		]
	}`))

	if withContext <= base {
		t.Fatalf("context-rich count = %d, want greater than base %d", withContext, base)
	}
}

func countTokensForTest(t *testing.T, handler *HealthHandler, body []byte) int {
	t.Helper()

	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/messages/count_tokens", bytes.NewReader(body))
	handler.HandleCountTokens(recorder, req)

	if got, want := recorder.Code, http.StatusOK; got != want {
		t.Fatalf("status = %d, want %d; body: %s", got, want, recorder.Body.String())
	}

	var response map[string]int
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is invalid JSON: %v", err)
	}
	return response["input_tokens"]
}

func newTestHealthHandler(t *testing.T) *HealthHandler {
	t.Helper()

	counter, err := token.NewCounter()
	if err != nil {
		t.Fatalf("NewCounter() error = %v", err)
	}
	return NewHealthHandler(counter, nil, metrics.New())
}
