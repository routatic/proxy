# Sync.Mutex for Concurrent State

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Use `sync.Mutex` for protecting shared state. Prefer pointer mutex embedded in structs. Always lock/unlock in the same method. Use `defer mu.Unlock()` for safety.

## CORRECT

```go
// Pointer mutex in struct
type CircuitBreaker struct {
    mu               sync.Mutex
    state            CircuitState
    failureCount     int
    lastFailureTime  time.Time
}

// Lock with defer for safety
func (cb *CircuitBreaker) AllowRequest() bool {
    cb.mu.Lock()
    defer cb.mu.Unlock()
    
    switch cb.state {
    case CircuitClosed:
        return true
    case CircuitOpen:
        if time.Since(cb.lastFailureTime) > cb.recoveryTimeout {
            cb.state = CircuitHalfOpen
            return true
        }
        return false
    }
    return false
}

// Separate mutexes for separate concerns
type FallbackHandler struct {
    mu              sync.Mutex // For provider blocking
    circuitBreakers map[string]*CircuitBreaker
    cbMu            sync.Mutex // Separate lock for circuit breaker map
}
```

## WRONG

```go
// Value mutex (copies are useless)
type CircuitBreaker struct {
    mu sync.Mutex // Non-pointer, works but unusual
}

// Missing unlock on error path
func (cb *CircuitBreaker) Allow() bool {
    cb.mu.Lock()
    if cb.state == CircuitOpen {
        return false // Forgot to unlock!
    }
    cb.mu.Unlock()
    return true
}

// Single mutex for everything
type Handler struct {
    mu sync.Mutex // Protects everything - too coarse
}
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Declaration | `mu sync.Mutex` (pointer receiver methods work) |
| Lock pattern | `mu.Lock(); defer mu.Unlock()` |
| Granularity | Separate mutexes for independent state |
| RLock | Use `sync.RWMutex` when reads dominate |
