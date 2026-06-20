# Walkthrough - PR #80 Integration and Timeout Config Bug Fix

This integration completes 100% of the features, test cases, and documentation updates from PR #80 (`fix: stabilize Anthropic-native streaming, timeout handling, and fallback cancellation`), and addresses the issues surfaced during the RE-REVIEW pass.

## Changes and Integrations Applied

### 1. Fix for Timeout Config Overlap (BUG 1 & BUG 2)
- **Problem**: Previously, when a user configured `"streaming_timeout_ms": 600000` (total stream timeout) without setting `"stream_timeout_ms"` (idle timeout), the loader would silently assign `StreamTimeoutMs = TimeoutMs` (default 300000ms). As a result, the idle watchdog would fire at 300s and effectively neutralize the user's 600s total stream timeout whenever a stream stalled or went idle.
- **Fix**: Updated `applyDefaults` in [`loader.go`](internal/config/loader.go). When `StreamTimeoutMs` is unset, it now inherits the user-supplied `StreamingTimeoutMs` first (when present), and only falls back to `TimeoutMs` if neither is set.
- **Test coverage**: Added `TestDefaults_StreamingTimeoutFallback` to [`loader_test.go`](internal/config/loader_test.go) to exercise the real JSON file parsing path and assert the fallback behavior.

### 2. Confirmation on MiniMax and Qwen-plus Routing (BUG 3)
- The expansion of `IsAnthropicModel` (in [`opencode.go`](internal/client/opencode.go)) and `isAnthropicNativeGo` (in [`opencode_go.go`](internal/provider/opencode_go.go)) to return `true` for `minimax-m2.5/2.7/3` and the `qwen-plus` family is an intentional bug fix.
- These models reject OpenAI-format streaming with tools on OpenCode Go with `400: invalid params, function name or parameters is empty`. Routing them through the Anthropic-native `/v1/messages` branch resolves the failure mode at its root.
- Test cases in `internal/client/opencode_test.go` were updated in lockstep to reflect this new behavior.

### 3. Other Improvements and Integrations from PR #80
- **Heartbeat Suppression**: Pauses the SSE keepalive ticker while copying a raw Anthropic stream via the `heartbeatPaused` flag.
- **Early Cancellation**: Cancels fallback attempts as soon as the parent context is canceled in `ExecuteWithFallback` ([`fallback.go`](internal/router/fallback.go)).
- **`transformTools` Hardening**: Skips tools with missing names, validates that the schema `type` is `object`, and adds a guard for null/empty `input_schema`.
- **Documentation**: Added the **Streaming Scenario Routing** section to [`CONFIGURATION.md`](CONFIGURATION.md) and [`README.md`](README.md).

---

## Validation Results

All test suites on the current workspace pass at 100%:

```bash
go test ./...
```

Output:

```text
ok  	github.com/routatic/proxy/internal/client	0.007s
ok  	github.com/routatic/proxy/internal/config	1.224s
ok  	github.com/routatic/proxy/internal/daemon	(cached)
ok  	github.com/routatic/proxy/internal/handlers	1.503s
ok  	github.com/routatic/proxy/internal/router	(cached)
ok  	github.com/routatic/proxy/internal/token	(cached)
ok  	github.com/routatic/proxy/internal/transformer	(cached)
ok  	github.com/routatic/proxy/pkg/types	(cached)
```

All changes remain in the Unstaged state. No commits have been created.
