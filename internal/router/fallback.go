// Package router defines HTTP route registration and middleware chaining.
package router

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/routatic/proxy/internal/client"
	"github.com/routatic/proxy/internal/config"
)

// CircuitState represents the state of a circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation — requests flow freely
	CircuitHalfOpen                     // Recovery probe — allowing limited test requests
	CircuitOpen                         // Failing fast — blocking all requests until timeout
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

// NewCircuitBreaker creates a circuit breaker that opens after threshold
// consecutive failures and stays open for recoveryTimeout before allowing
// a probe. The defaults in NewFallbackHandler are 3 failures and 30s timeout,
// which balances quick recovery from transient issues with protection against
// sustained outages.
func NewCircuitBreaker(threshold int, recoveryTimeout time.Duration) *CircuitBreaker {
	return &CircuitBreaker{
		state:            CircuitClosed,
		threshold:        threshold,
		recoveryTimeout:  recoveryTimeout,
		halfOpenMaxCalls: 3,
	}
}

// AllowRequest returns whether the circuit should permit a request. In the
// closed state, all requests pass. In the open state, requests are blocked
// until recoveryTimeout elapses, at which point the circuit transitions to
// half-open and allows a limited number of probe requests. A successful probe
// closes the circuit; a failure reopens it.
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

// RecordSuccess transitions the circuit toward a healthy state. In half-open
// mode, accumulating enough successes (halfOpenMaxCalls, default 3) closes
// the circuit. In closed mode, this resets the failure counter so transient
// failures don't accumulate over time.
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

// RecordFailure transitions the circuit toward an unhealthy state. In half-open
// mode, any failure immediately reopens the circuit. In closed mode, once the
// failure count reaches the threshold, the circuit opens and blocks subsequent
// requests until the recovery timeout elapses.
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

// State returns the current circuit breaker state (CircuitClosed, CircuitHalfOpen,
// or CircuitOpen). Use this to inspect whether a model is being skipped due to
// recent failures, or to expose circuit state via metrics/health endpoints.
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
	atomicCfg       *config.AtomicConfig // Optional: for checking provider key counts
}

// NewFallbackHandler creates a handler that tries models in sequence until one
// succeeds, with per-model circuit breakers to skip failing models. Use this
// when you need resilient upstream calls with automatic backoff — the handler
// tracks failures per model and avoids hammering an already-failing endpoint.
// Default threshold is 3 failures; default timeout is 30 seconds.
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

// SetAtomicConfig sets the atomic config for the fallback handler.
// This is used to check provider key counts for auth error handling.
func (h *FallbackHandler) SetAtomicConfig(cfg *config.AtomicConfig) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.atomicCfg = cfg
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
	blockedProviders := make(map[string]bool)
	var usageLimitErr error
	var authErr error
	authAttempted := 0

	for i, model := range models {
		if err := ctx.Err(); err != nil {
			h.logger.Info("request context canceled, stopping fallback attempts",
				"error", err,
			)
			return nil, nil, err
		}

		provider := client.Provider(model)
		if blockedProviders[provider] {
			h.logger.Info("provider usage limit reached, skipping model", "provider", provider, "model", model.ModelID)
			continue
		}

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

		if errCtx := ctx.Err(); errCtx != nil {
			h.logger.Info("request context canceled after model attempt, stopping fallback",
				"model", model.ModelID,
				"error", errCtx,
			)
			return nil, nil, errCtx
		}

		// A provider-wide usage limit makes its remaining models pointless.
		// Skip them, but continue if the chain includes another provider.
		if IsUsageLimitError(err) {
			usageLimitErr = err
			blockedProviders[provider] = true
			h.logger.Warn("provider usage limit reached, trying another provider",
				"provider", provider,
				"model", model.ModelID,
				"error", err,
			)
			continue
		}

		// Auth errors (401/403) indicate invalid credentials.
		// If the provider has a single API key, block it so its remaining
		// models are skipped. If it has multiple keys, don't block the
		// round-robin — the next attempt will use a different key.
		if IsAuthError(err) {
			keyCount := client.ProviderKeyCount(h.atomicCfg, provider)
			if keyCount <= 1 {
				h.logger.Warn("authentication error, blocking provider",
					"provider", provider,
					"model", model.ModelID,
					"error", err,
				)
				blockedProviders[provider] = true
				authErr = err
				authAttempted = i + 1
				continue
			}
			h.logger.Warn("authentication error, but provider has multiple keys, trying next",
				"provider", provider,
				"model", model.ModelID,
				"key_count", keyCount,
				"error", err,
			)
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

	if authErr != nil {
		return &FallbackResult{
			ModelID:     models[0].ModelID,
			Success:     false,
			Attempted:   authAttempted,
			TotalModels: totalModels,
		}, nil, authErr
	}

	if usageLimitErr != nil {
		return &FallbackResult{
			ModelID:     models[0].ModelID,
			Success:     false,
			Attempted:   totalModels,
			TotalModels: totalModels,
		}, nil, usageLimitErr
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

	// APIError from the client carries the HTTP status code — use it directly
	// instead of string matching, so error format changes upstream can't
	// silently break the classification.
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		// 4xx client errors are not retryable — the request format itself is
		// invalid for that model, and retrying won't fix it. This includes 429
		// (rate limit) so the circuit breaker doesn't open for rate limits.
		return apiErr.StatusCode >= 500
	}

	// For non-API errors (network errors, timeouts, etc.), fall back to
	// pattern matching on the error string.
	errStr := err.Error()

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

// IsUsageLimitError returns true if the error is a GoUsageLimitError.
// Usage limit errors should be passed directly to the client instead of
// triggering a fallback, as fallback attempts will also encounter the
// same usage limit within a short period.
func IsUsageLimitError(err error) bool {
	if err == nil {
		return false
	}

	// Check for GoUsageLimitError in the error message
	// The error body contains: {"type":"error","error":{"type":"GoUsageLimitError",...}}
	errStr := err.Error()
	return strings.Contains(errStr, "GoUsageLimitError")
}

// IsAuthError returns true if the error is an authentication error (401 or 403).
// Auth errors are non-retryable and indicate invalid or expired credentials.
// Since all models from the same provider share the same API key, fallback
// attempts will fail identically, so we short-circuit the fallback chain.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 401 || apiErr.StatusCode == 403
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
