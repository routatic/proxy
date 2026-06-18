// Package router defines HTTP route registration and middleware chaining.
package router

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"oc-go-cc/internal/config"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitHalfOpen                     // Testing if service recovered
	CircuitOpen                         // Failing fast, not attempting calls
)

// CircuitBreaker tracks failure rates and prevents calls to failing models.
type CircuitBreaker struct {
	mu               sync.Mutex
	state            CircuitState
	failureCount     int
	successCount     int
	lastFailureTime  time.Time
	threshold        int           // failures before opening circuit
	recoveryTimeout  time.Duration // how long to wait before half-open
	halfOpenMaxCalls int           // max test calls in half-open state
	halfOpenCalls    int
}

// NewCircuitBreaker creates a circuit breaker with default thresholds.
func NewCircuitBreaker(threshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		threshold:        threshold,
		recoveryTimeout:  recoveryTimeout,
		halfOpenMaxCalls: 3,
	}
}

// AllowRequest returns true if the circuit allows a request.
func (cb *CircuitBreaker) AllowRequest() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
		// Check if recovery timeout has elapsed
		if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
			cb.state = CircuitHalfOpen
			cb.halfOpenCalls = 0
			return true
		}
		return false
	case CircuitHalfOpen:
		if cb.halfOpenCalls < cb.halfOpenMaxCalls {
			cb.halfOpenCalls++
			return true
		}
		return false
	}
	return false
}

// RecordSuccess records a successful call.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMaxCalls {
			cb.state = CircuitClosed
			cb.failureCount = 0
			cb.successCount = 0
		}
	case CircuitClosed:
		cb.failureCount = 0
	}
}

// RecordFailure records a failed call.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()
	cb.failureCount++

	switch cb.state {
	case CircuitHalfOpen:
		cb.state = CircuitOpen
		cb.successCount = 0
	case CircuitClosed:
		if cb.failureCount >= cb.threshold {
			cb.state = CircuitOpen
		}
	}
}

// State returns the current circuit state.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// FallbackResult contains the result of a fallback attempt.
type FallbackResult struct {
	ModelID     string
	Success     bool
	Error       error
	Attempted   int
	TotalModels int
}

// FallbackHandler manages model fallback with circuit breaker protection.
type FallbackHandler struct {
	logger          *slog.Logger
	circuitBreakers map[string]*CircuitBreaker
	cbThreshold     int
	cbTimeout       time.Duration
	mu              sync.Mutex
}

// NewFallbackHandler creates a new fallback handler with circuit breakers.
func NewFallbackHandler(logger *slog.Logger, cbThreshold int, cbTimeout time.Duration) *FallbackHandler {
	if logger == nil {
		logger = slog.Default()
	}
	if cbThreshold <= 0 {
		cbThreshold = 3
	}
	if cbTimeout <= 0 {
		cbTimeout = 30 * time.Second
	}

	return &FallbackHandler{
		logger:          logger,
		circuitBreakers: make(map[string]*CircuitBreaker),
		cbThreshold:     cbThreshold,
		cbTimeout:       cbTimeout,
	}
}

// getCircuitBreaker returns or creates a circuit breaker for a model.
func (h *FallbackHandler) getCircuitBreaker(modelID string) *CircuitBreaker {
	h.mu.Lock()
	defer h.mu.Unlock()

	cb, exists := h.circuitBreakers[modelID]
	if !exists {
		cb = NewCircuitBreaker(h.cbThreshold, h.cbTimeout)
		h.circuitBreakers[modelID] = cb
	}
	return cb
}

// ExecuteWithFallback tries models in sequence until one succeeds.
// Respects circuit breaker state to skip models that are failing repeatedly.
func (h *FallbackHandler) ExecuteWithFallback(
	ctx context.Context,
	models []config.ModelConfig,
	executor func(context.Context, config.ModelConfig) ([]byte, error),
) (*FallbackResult, []byte, error) {
	totalModels := len(models)

	for i, model := range models {
		cb := h.getCircuitBreaker(model.ModelID)

		// Skip models with open circuit breakers
		if !cb.AllowRequest() {
			h.logger.Info("circuit breaker open, skipping model",
				"model", model.ModelID,
				"attempt", i+1,
				"total", totalModels,
			)
			continue
		}

		h.logger.Info("attempting model",
			"model", model.ModelID,
			"attempt", i+1,
			"total", totalModels,
		)

		body, err := executor(ctx, model)
		if err == nil {
			cb.RecordSuccess()
			h.logger.Info("model succeeded",
				"model", model.ModelID,
				"attempt", i+1,
			)
			return &FallbackResult{
				ModelID:     model.ModelID,
				Success:     true,
				Attempted:   i + 1,
				TotalModels: totalModels,
			}, body, nil
		}

		if IsRetryableError(err) {
			cb.RecordFailure()
			h.logger.Warn("model failed, trying fallback",
				"model", model.ModelID,
				"error", err,
				"remaining", totalModels-i-1,
				"circuit_state", cb.State(),
			)
		} else {
			h.logger.Warn("non-retryable error (skipping circuit breaker), trying fallback",
				"model", model.ModelID,
				"error", err,
				"remaining", totalModels-i-1,
			)
		}
	}

	return &FallbackResult{
		ModelID:     models[0].ModelID,
		Success:     false,
		Attempted:   totalModels,
		TotalModels: totalModels,
	}, nil, fmt.Errorf("all models failed (%d attempts)", totalModels)
}

// GetFallbackChain returns the fallback chain for a given primary model.
func GetFallbackChain(primary config.ModelConfig, fallbacks map[string][]config.ModelConfig) []config.ModelConfig {
	chain := []config.ModelConfig{primary}

	if fb, exists := fallbacks[primary.ModelID]; exists {
		chain = append(chain, fb...)
	}

	return chain
}

// IsRetryableError determines if an error is worth retrying with a fallback.
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := err.Error()

	// 4xx client errors are not retryable — the request format itself is invalid
	// for that model, and retrying won't fix it.
	if strings.Contains(errStr, "API error 4") {
		return false
	}

	// Retry on network errors, timeouts, rate limits (from non-4xx paths),
	// and server errors (5xx). 4xx client errors are already excluded by
	// the "API error 4" check above — 429 is correctly non-retryable, so
	// the circuit breaker doesn't open for rate limits.
	retryable := []string{
		"timeout",
		"connection refused",
		"connection reset",
		"rate limit",
		"503",
		"502",
		"500",
	}

	for _, sub := range retryable {
		if strings.Contains(errStr, sub) {
			return true
		}
	}
	return false
}

// GetCircuitStates returns the state of all circuit breakers.
func (h *FallbackHandler) GetCircuitStates() map[string]string {
	h.mu.Lock()
	defer h.mu.Unlock()

	states := make(map[string]string)
	for modelID, cb := range h.circuitBreakers {
		state := cb.State()
		switch state {
		case CircuitClosed:
			states[modelID] = "closed"
		case CircuitHalfOpen:
			states[modelID] = "half_open"
		case CircuitOpen:
			states[modelID] = "open"
		default:
			states[modelID] = "unknown"
		}
	}
	return states
}
