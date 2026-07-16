# Constructor Pattern

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Use `New<TypeName>` functions to create instances. Return pointer types. Apply sensible defaults for nil/zero parameters.

## CORRECT

```go
// Constructor with defaults
func NewFallbackHandler(logger *slog.Logger, threshold int, timeout time.Duration) *FallbackHandler {
    if logger == nil {
        logger = slog.Default()
    }
    if threshold <= 0 {
        threshold = 3 // sensible default
    }
    if timeout <= 0 {
        timeout = 30 * time.Second
    }
    return &FallbackHandler{
        logger:          logger,
        threshold:        threshold,
        timeout:          timeout,
    }
}

// Simple constructor
func NewRequestTransformer() *RequestTransformer {
    return &RequestTransformer{}
}
```

## WRONG

```go
// No defaults, caller must know magic values
func NewFallbackHandler(threshold int, timeout time.Duration) *FallbackHandler {
    return &FallbackHandler{threshold: threshold, timeout: timeout}
    // Panics or misbehaves if threshold=0
}

// Inconsistent naming
func CreateFallbackHandler(...) *FallbackHandler
func MakeFallbackHandler(...) *FallbackHandler
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Naming | `New<TypeName>` |
| Return | Pointer to struct |
| Nil params | Apply sensible defaults |
| Zero values | Apply sensible defaults |
