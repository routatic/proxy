package router

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

func TestIsRetryableError_ClientsErrorsNotRetryable(t *testing.T) {
	tests := []struct {
		err  error
		want bool
	}{
		// 4xx errors should NOT be retryable
		{err: &client.APIError{StatusCode: 400, Body: "bad request"}, want: false},
		{err: &client.APIError{StatusCode: 401, Body: "unauthorized"}, want: false},
		{err: &client.APIError{StatusCode: 403, Body: "forbidden"}, want: false},
		{err: &client.APIError{StatusCode: 404, Body: "not found"}, want: false},
		{err: &client.APIError{StatusCode: 422, Body: "unprocessable"}, want: false},
		{err: &client.APIError{StatusCode: 429, Body: "rate limit"}, want: false},

		// 5xx errors should be retryable
		{err: &client.APIError{StatusCode: 500, Body: "internal error"}, want: true},
		{err: &client.APIError{StatusCode: 502, Body: "bad gateway"}, want: true},
		{err: &client.APIError{StatusCode: 503, Body: "service unavailable"}, want: true},

		// Non-API errors — fall back to string matching
		{err: errors.New("request timeout"), want: true},
		{err: errors.New("connection refused"), want: true},
		{err: errors.New("connection reset by peer"), want: true},
		{err: errors.New("rate limit exceeded"), want: true},

		// Edge cases
		{err: errors.New(""), want: false},
		{err: errors.New("random error"), want: false},
		{err: errors.New("API error 400"), want: false},
		{err: errors.New("API error 500"), want: true},
	}

	for _, tt := range tests {
		t.Run(tt.err.Error(), func(t *testing.T) {
			if got := IsRetryableError(tt.err); got != tt.want {
				t.Errorf("IsRetryableError(%q) = %v, want %v", tt.err.Error(), got, tt.want)
			}
		})
	}
}

func TestExecuteWithFallback_NonRetryableDoesNotOpenCircuit(t *testing.T) {
	h := NewFallbackHandler(nil, 1, 0) // 1 failure = open circuit

	models := []config.ModelConfig{
		{ModelID: "model-a"},
		{ModelID: "model-b"},
	}

	attempts := 0
	_, _, err := h.ExecuteWithFallback(
		context.Background(),
		models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			attempts++
			// Non-retryable 400 error — should NOT open circuit breaker
			return nil, &client.APIError{StatusCode: 400, Body: "bad request"}
		},
	)

	if err == nil {
		t.Fatal("expected all models to fail")
	}

	// Circuit breaker should still be closed since errors were non-retryable
	cb := h.getCircuitBreaker("model-a")
	if cb.State() != CircuitClosed {
		t.Errorf("model-a circuit should be closed after non-retryable errors, got %v", cb.State())
	}

	// All models were tried
	if attempts != 2 {
		t.Errorf("expected 2 attempts, got %d", attempts)
	}
}

func TestExecuteWithFallback_RetryableOpensCircuit(t *testing.T) {
	h := NewFallbackHandler(nil, 1, 0)

	models := []config.ModelConfig{
		{ModelID: "model-a"},
		{ModelID: "model-b"},
	}

	_, _, err := h.ExecuteWithFallback(
		context.Background(),
		models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			// Retryable 500 error — should open circuit breaker
			return nil, &client.APIError{StatusCode: 500, Body: "internal error"}
		},
	)

	if err == nil {
		t.Fatal("expected all models to fail")
	}

	// Circuit breaker should be OPEN after retryable failure
	cb := h.getCircuitBreaker("model-a")
	if cb.State() != CircuitOpen {
		t.Errorf("model-a circuit should be open after retryable error, got %v", cb.State())
	}
}

func TestExecuteWithFallback_NonRetryableThenRetryable(t *testing.T) {
	h := NewFallbackHandler(nil, 1, 0)
	callCount := 0

	models := []config.ModelConfig{
		{ModelID: "model-a"},
		{ModelID: "model-b"},
	}

	_, _, err := h.ExecuteWithFallback(
		context.Background(),
		models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			if callCount == 1 {
				// Non-retryable: model-a should NOT get circuit opened
				return nil, &client.APIError{StatusCode: 400, Body: "bad request"}
			}
			// Retryable: model-b should get circuit opened
			return nil, &client.APIError{StatusCode: 500, Body: "internal error"}
		},
	)

	if err == nil {
		t.Fatal("expected all models to fail")
	}

	// model-a circuit should be closed (non-retryable)
	cbA := h.getCircuitBreaker("model-a")
	if cbA.State() != CircuitClosed {
		t.Errorf("model-a circuit should be closed after non-retryable error, got %v", cbA.State())
	}

	// model-b circuit should be open (retryable)
	cbB := h.getCircuitBreaker("model-b")
	if cbB.State() != CircuitOpen {
		t.Errorf("model-b circuit should be open after retryable error, got %v", cbB.State())
	}
}

func TestExecuteWithFallback_StopsOnCanceledContext(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 3, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
		{Provider: "opencode-go", ModelID: "model-b"},
	}

	callCount := 0
	_, _, err := handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			return []byte("ok"), nil
		},
	)

	if callCount != 0 {
		t.Errorf("executor called %d times, want 0 (canceled context must stop immediately)", callCount)
	}
	if err == nil {
		t.Error("expected error for canceled context, got nil")
	}

	states := handler.GetCircuitStates()
	if len(states) > 0 {
		t.Errorf("expected no circuit breakers created, got %d", len(states))
	}
}

func TestExecuteWithFallback_StopsOnCanceledAfterFirstModel(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 3, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
		{Provider: "opencode-go", ModelID: "model-b"},
	}

	callCount := 0
	_, _, err := handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			if callCount == 1 {
				cancel()
				return nil, context.Canceled
			}
			return []byte("ok"), nil
		},
	)

	if callCount != 1 {
		t.Errorf("executor called %d times, want 1 (should stop after parent context canceled)", callCount)
	}
	if err == nil {
		t.Error("expected error for canceled context, got nil")
	}

	states := handler.GetCircuitStates()
	if _, ok := states["model-b"]; ok {
		t.Error("model-b should not have a circuit breaker entry")
	}
}

func TestExecuteWithFallback_PerModelTimeoutFallback(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 3, 30*time.Second)

	parentCtx, parentCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer parentCancel()

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
		{Provider: "opencode-go", ModelID: "model-b"},
	}

	callCount := 0
	result, body, err := handler.ExecuteWithFallback(parentCtx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			if callCount == 1 {
				return nil, context.DeadlineExceeded
			}
			return []byte("success-" + model.ModelID), nil
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("executor called %d times, want 2 (first timed out, second succeeds)", callCount)
	}
	if result.ModelID != "model-b" {
		t.Errorf("result model = %s, want model-b", result.ModelID)
	}
	if string(body) != "success-model-b" {
		t.Errorf("body = %s, want success-model-b", string(body))
	}
}

func TestExecuteWithFallback_RealPerModelTimeout(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 3, 30*time.Second)

	parentCtx, parentCancel := context.WithCancel(context.Background())
	defer parentCancel()

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
		{Provider: "opencode-go", ModelID: "model-b"},
	}

	callCount := 0
	result, body, err := handler.ExecuteWithFallback(parentCtx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			if callCount == 1 {
				attemptCtx, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
				<-attemptCtx.Done()
				return nil, attemptCtx.Err()
			}
			return []byte("fallback-success"), nil
		},
	)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if callCount != 2 {
		t.Errorf("executor called %d times, want 2", callCount)
	}
	if result.ModelID != "model-b" {
		t.Errorf("result model = %s, want model-b", result.ModelID)
	}
	if string(body) != "fallback-success" {
		t.Errorf("body = %s, want fallback-success", string(body))
	}
}

func TestExecuteWithFallback_CircuitBreakerDoesNotCountClientCancellation(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 1, 30*time.Second)

	ctx, cancel := context.WithCancel(context.Background())

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
	}

	callCount := 0
	_, _, err := handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			cancel()
			return nil, context.Canceled
		},
	)

	if callCount != 1 {
		t.Errorf("executor called %d times, want 1", callCount)
	}
	if err == nil {
		t.Error("expected error for canceled context")
	}

	states := handler.GetCircuitStates()
	if state, ok := states["model-a"]; ok {
		if state == "open" {
			t.Error("model-a circuit breaker should NOT be open for client cancellation")
		}
	}
}

func TestExecuteWithFallback_RealModelFailurePenalizesCircuitBreaker(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 1, 30*time.Second)

	ctx := context.Background()

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
	}

	_, _, _ = handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			return nil, errors.New("upstream 500 internal server error")
		},
	)

	// model-a's circuit breaker should be open because of real failure
	states := handler.GetCircuitStates()
	state, ok := states["model-a"]
	if !ok {
		t.Fatal("model-a should have circuit breaker entry")
	}
	if state != "open" {
		t.Errorf("model-a circuit breaker state = %s, want open", state)
	}
}

func TestExecuteWithFallback_ParentDeadlineExceededNotPenalized(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 1, 30*time.Second)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(10 * time.Millisecond) // let parent timeout expire

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
	}

	_, _, err := handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			return nil, nil
		},
	)

	if err == nil {
		t.Error("expected error for deadline exceeded context")
	}

	states := handler.GetCircuitStates()
	if state, ok := states["model-a"]; ok && state == "open" {
		t.Error("model-a circuit breaker should NOT be open for parent deadline exceeded")
	}
}

func TestExecuteWithFallback_AllModelsFailRecordsFailures(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 2, 30*time.Second)

	ctx := context.Background()

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
		{Provider: "opencode-go", ModelID: "model-b"},
	}

	_, _, err := handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			return nil, errors.New("upstream error")
		},
	)

	if err == nil {
		t.Error("expected error for all models failed")
	}

	states := handler.GetCircuitStates()
	if _, ok := states["model-a"]; !ok {
		t.Error("model-a should have circuit breaker entry")
	}
	if _, ok := states["model-b"]; !ok {
		t.Error("model-b should have circuit breaker entry")
	}
}

func TestIsUsageLimitError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "GoUsageLimitError from API",
			err:  &client.APIError{StatusCode: 429, Body: `{"type":"error","error":{"type":"GoUsageLimitError","message":"5-hour usage limit reached"}}`},
			want: true,
		},
		{
			name: "Regular rate limit error",
			err:  &client.APIError{StatusCode: 429, Body: `{"error": "rate limit exceeded"}`},
			want: false,
		},
		{
			name: "500 error with GoUsageLimitError in body",
			err:  &client.APIError{StatusCode: 500, Body: `{"type":"GoUsageLimitError"}`},
			want: true,
		},
		{
			name: "Non-API error with GoUsageLimitError",
			err:  errors.New(`API error 429: {"type":"GoUsageLimitError"}`),
			want: true,
		},
		{
			name: "Generic error",
			err:  errors.New("something went wrong"),
			want: false,
		},
		{
			name: "Nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsUsageLimitError(tt.err); got != tt.want {
				t.Errorf("IsUsageLimitError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestExecuteWithFallback_UsageLimitErrorStopsFallback(t *testing.T) {
	logger := slog.Default()
	handler := NewFallbackHandler(logger, 3, 30*time.Second)

	ctx := context.Background()

	models := []config.ModelConfig{
		{Provider: "opencode-go", ModelID: "model-a"},
		{Provider: "opencode-go", ModelID: "model-b"},
	}

	callCount := 0
	usageLimitErr := &client.APIError{
		StatusCode: 429,
		Body:       `{"type":"error","error":{"type":"GoUsageLimitError","message":"5-hour usage limit reached. Resets in 3hr 56min."}}`,
	}

	_, _, err := handler.ExecuteWithFallback(ctx, models,
		func(ctx context.Context, model config.ModelConfig) ([]byte, error) {
			callCount++
			return nil, usageLimitErr
		},
	)

	if callCount != 1 {
		t.Errorf("executor called %d times, want 1 (should stop on usage limit error)", callCount)
	}

	if err == nil {
		t.Fatal("expected error for usage limit")
	}

	// The error should be the original usage limit error, not "all models failed"
	if !IsUsageLimitError(err) {
		t.Errorf("expected usage limit error, got: %v", err)
	}

	// Circuit breaker should not be affected by usage limit errors
	states := handler.GetCircuitStates()
	if state, ok := states["model-a"]; ok && state == "open" {
		t.Error("model-a circuit breaker should NOT be open for usage limit error")
	}
}
