package client

import (
	"strconv"
	"sync"
	"time"
)

// CircuitState represents the state of a key-level circuit breaker.
type CircuitState int

const (
	CircuitClosed   CircuitState = iota // Normal operation
	CircuitHalfOpen                      // Testing if key recovered
	CircuitOpen                          // Failing fast
)

// KeyCircuitBreaker tracks per-api-key failure rates.
// Modeled after router.CircuitBreaker but scoped to a single upstream key.
type KeyCircuitBreaker struct {
	mu              sync.Mutex
	state           CircuitState
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	threshold       int
	recoveryTimeout time.Duration
	halfOpenMaxCalls int
	halfOpenCalls    int
}

func NewKeyCircuitBreaker(threshold int, recoveryTimeout time.Duration) *KeyCircuitBreaker {
	return &KeyCircuitBreaker{
		threshold:        threshold,
		recoveryTimeout:  recoveryTimeout,
		halfOpenMaxCalls: 3,
	}
}

// Allow returns true if the key is allowed to be used.
func (cb *KeyCircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitClosed:
		return true
	case CircuitOpen:
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

// RecordSuccess resets failure state (closed) or advances half-open probe.
func (cb *KeyCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case CircuitHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.halfOpenMaxCalls {
			cb.state = CircuitClosed
			cb.failureCount = 0
			cb.successCount = 0
			cb.halfOpenCalls = 0
		}
	case CircuitClosed:
		cb.failureCount = 0
	}
}

// RecordFailure increments failure count and opens circuit when threshold reached.
func (cb *KeyCircuitBreaker) RecordFailure() {
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
func (cb *KeyCircuitBreaker) State() CircuitState {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// parseRetryAfter parses a Retry-After header value into a duration.
// It handles integer seconds ("60") and falls back to a default on parse error.
func parseRetryAfter(raw string, fallback time.Duration) time.Duration {
	if raw == "" {
		return fallback
	}
	// Retry-After is typically seconds (RFC 7231)
	if sec, err := strconv.Atoi(raw); err == nil && sec >= 0 {
		return time.Duration(sec) * time.Second
	}
	// Could be an HTTP-date; for simplicity we fall back to default.
	return fallback
}
