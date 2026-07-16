# Context Propagation

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Context is always the first parameter. Use per-attempt contexts with timeouts for operations that can fail independently. Check cancellation at loop boundaries.

## CORRECT

```go
// Context as first parameter
func (h *FallbackHandler) ExecuteWithFallback(
    ctx context.Context,
    models []config.ModelConfig,
    executor func(context.Context, config.ModelConfig) ([]byte, error),
) (*FallbackResult, []byte, error) {
    for i, model := range models {
        // Check cancellation at loop start
        if err := ctx.Err(); err != nil {
            return nil, nil, err
        }
        
        // Per-attempt context with timeout
        attemptCtx, cancel := context.WithTimeout(ctx, timeout)
        body, err := executor(attemptCtx, model)
        cancel() // Always cancel
        
        if err == nil {
            return result, body, nil
        }
    }
}

// Distinguish client disconnect from upstream timeout
if clientCtx.Err() != nil {
    return ErrClientDisconnected
}
return ErrStreamIdle
```

## WRONG

```go
// Context not first parameter
func Execute(models []Model, ctx context.Context) error

// No cancellation check in loops
for _, model := range models {
    body, err := executor(ctx, model) // Blocks forever on hung upstream
}

// Missing cancel call
attemptCtx, _ := context.WithTimeout(ctx, timeout)
// defer cancel() // Missing!
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Position | First parameter |
| Per-attempt | `context.WithTimeout` for each upstream call |
| Cleanup | Always `defer cancel()` or explicit `cancel()` |
| Loop check | `if ctx.Err() != nil` at loop boundary |
