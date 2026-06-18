package router

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"oc-go-cc/internal/config"
)

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

// TestExecuteWithFallback_RealModelFailurePenalizesCircuitBreaker verifies
// that a real upstream error (non-cancellation) DOES count as a model failure.
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

// TestExecuteWithFallback_ParentDeadlineExceededNotPenalized verifies
// context.DeadlineExceeded from parent context does not count as failure.
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

// TestExecuteWithFallback_AllModelsFailRecordsFailures verifies
// that multiple real model failures all record circuit breaker failures.
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
