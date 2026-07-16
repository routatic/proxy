# Structured Logging with Slog

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Use `log/slog` with key-value pairs. Include contextual fields like `model_id`, `provider`, `attempt`. Log at appropriate levels: Debug for routine operations, Info for significant events, Warn for recoverable issues, Error for failures.

## CORRECT

```go
// Structured key-value logging
h.logger.Info("attempting model",
    "model", model.ModelID,
    "provider", model.Provider,
    "attempt", i+1,
    "total", totalModels,
)

// Debug for routine operations
h.logger.Debug("client disconnected during stream")

// Warn for recoverable issues
h.logger.Warn("model failed, trying fallback",
    "model", model.ModelID,
    "error", err,
    "remaining", totalModels-i-1,
)

// Error for failures
h.logger.Error("request error",
    "status", statusCode,
    "message", message,
    "error", err,
)

// Nil logger fallback
if logger == nil {
    logger = slog.Default()
}
```

## WRONG

```go
// Unstructured printf-style logging
log.Printf("attempting model %s (attempt %d/%d)", model.ModelID, i+1, total)

// Wrong level
h.logger.Error("model failed, trying fallback") // Should be Warn

// Missing context
h.logger.Info("request completed") // Which model? How long?
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Package | `log/slog` |
| Levels | Debug < Info < Warn < Error |
| Format | Key-value pairs after message |
| Nil safety | Default to `slog.Default()` |
