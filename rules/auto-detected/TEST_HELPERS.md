# Test Helper Functions

> Auto-detected by /spartan:scan-rules — review and edit as needed.

Mark helper functions with `t.Helper()` for better error line numbers. Create factory functions for test fixtures. Use descriptive test names that explain the scenario.

## CORRECT

```go
// Helper function with t.Helper()
func newTestMessagesHandler(t *testing.T, cfg *config.Config) *MessagesHandler {
    t.Helper()
    return &MessagesHandler{
        modelRouter: router.NewModelRouter(config.NewAtomicConfig(cfg, "/tmp/test-config.json")),
        logger:      slog.Default(),
    }
}

// Factory for test fixtures
func newStreamingTestHandler(t *testing.T, upstreamURL string) *MessagesHandler {
    t.Helper()
    cfg := &config.Config{
        APIKey: "test-key",
        OpenCodeGo: config.OpenCodeGoConfig{
            AnthropicBaseURL: upstreamURL,
            BaseURL:          upstreamURL,
            TimeoutMs:        5000,
        },
    }
    // ...
}

// Descriptive test name
func TestBuildModelChain_Override_AppendsScenarioChainDeduped(t *testing.T) {
    // Test that override chain appends scenario chain with duplicates removed
}

// Helper for extracting test data
func chainIDs(chain []config.ModelConfig) []string {
    out := make([]string, len(chain))
    for i, m := range chain {
        out[i] = m.ModelID
    }
    return out
}
```

## WRONG

```go
// Missing t.Helper()
func newTestHandler(t *testing.T) *Handler {
    return &Handler{} // Error line points here, not test
}

// Vague test name
func TestBuildModelChain(t *testing.T) {
    // What aspect of BuildModelChain?
}

// Duplicated fixture setup in every test
func TestX(t *testing.T) {
    cfg := &config.Config{...} // Copied 20 times
}
```

## Quick Reference

| Aspect | Convention |
|--------|-----------|
| Marker | `t.Helper()` at start of helper |
| Factories | `newTest<Type>` functions |
| Naming | `Test<Unit>_<Scenario>_<ExpectedResult>` |
| Extraction | Small helpers like `chainIDs` for assertions |
