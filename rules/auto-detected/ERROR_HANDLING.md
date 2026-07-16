# Structured Error Handling

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Use sentinel errors at package level for common failures, wrap errors with context using `fmt.Errorf`, and provide error classification functions for callers to make decisions.

## CORRECT

```go
// internal/core/errors.go
var (
    ErrModelNotFound      = errors.New("model not found")
    ErrProviderNotFound   = errors.New("provider not found")
    ErrRateLimited        = errors.New("rate limited by provider")
)

// Wrap with context
if err := json.Unmarshal(rawBody, &req); err != nil {
    return fmt.Errorf("failed to parse request body: %w", err)
}

// Classification function
func IsRetryableError(err error) bool {
    if err == nil {
        return false
    }
    var apiErr *client.APIError
    if errors.As(err, &apiErr) {
        return apiErr.StatusCode >= 500
    }
    return false
}
```

## WRONG

```go
// No sentinel errors, inline string comparisons
if err.Error() == "rate limit" {
    // Fragile - breaks if error message changes
}

// No error wrapping
if err != nil {
    return err // Loses context
}

// No classification helpers
if strings.Contains(err.Error(), "timeout") {
    // Every caller duplicates this logic
}
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Sentinel errors | `var Err... = errors.New("...")` at package level |
| Wrapping | `fmt.Errorf("context: %w", err)` |
| Classification | `func IsXError(err error) bool` functions |
| Nil check | Always check `err == nil` first in classifiers |
