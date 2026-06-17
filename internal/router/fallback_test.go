package router

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"oc-go-cc/internal/config"
)

func TestIsRetryableError_ClientsErrorsNotRetryable(t *testing.T) {
	tests := []struct {
		err  string
		want bool
	}{
		// 4xx errors should NOT be retryable
		{err: "API error 400: bad request", want: false},
		{err: "API error 401: unauthorized", want: false},
		{err: "API error 403: forbidden", want: false},
		{err: "API error 404: not found", want: false},
		{err: "API error 422: unprocessable", want: false},
		{err: "API error 429: rate limit", want: false},

		// 5xx and network errors should be retryable (existing behavior)
		{err: "API error 500: internal error", want: true},
		{err: "API error 502: bad gateway", want: true},
		{err: "API error 503: service unavailable", want: true},
		{err: "request timeout", want: true},
		{err: "connection refused", want: true},
		{err: "connection reset by peer", want: true},
		{err: "rate limit exceeded", want: true},

		// Edge cases
		{err: "", want: false},
		{err: "random error", want: false},
		{err: "API error 400", want: false},
		{err: "API error 500", want: true},
	}

	for _, tt := range tests {
		t.Run(tt.err, func(t *testing.T) {
			var err error
			if tt.err != "" {
				err = errors.New(tt.err)
			}
			if got := IsRetryableError(err); got != tt.want {
				t.Errorf("IsRetryableError(%q) = %v, want %v", tt.err, got, tt.want)
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
			return nil, fmt.Errorf("API error 400: bad request")
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
			return nil, fmt.Errorf("API error 500: internal error")
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
				return nil, fmt.Errorf("API error 400: bad request")
			}
			// Retryable: model-b should get circuit opened
			return nil, fmt.Errorf("API error 500: internal error")
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
