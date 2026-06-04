package client

import (
	"sync"
	"time"
)

// KeyCircuitBreaker tracks per-api-key failure rates.
// Modeled after router.CircuitBreaker but scoped to a single upstream key.
type KeyCircuitBreaker struct {
	mu              sync.Mutex
	failureCount    int
	lastFailureTime time.Time
	threshold       int
	recoveryTimeout time.Duration
	open            bool
}

func NewKeyCircuitBreaker(threshold int, recoveryTimeout time.Duration) *KeyCircuitBreaker {
	return &KeyCircuitBreaker{
		threshold:       threshold,
		recoveryTimeout: recoveryTimeout,
	}
}

// Allow returns true if the key is allowed to be used.
func (cb *KeyCircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if !cb.open {
		return true
	}
	if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
		cb.open = false
		cb.failureCount = 0
		return true
	}
	return false
}

// RecordSuccess resets failure state.
func (cb *KeyCircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failureCount = 0
	cb.open = false
}

// RecordFailure increments failure count and opens circuit when threshold reached.
func (cb *KeyCircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()
	cb.failureCount++
	if cb.failureCount >= cb.threshold {
		cb.open = true
	}
}
