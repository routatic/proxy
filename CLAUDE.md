# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

```bash
make build   # Build binary to bin/oc-go-cc
make run     # Run without building
make test    # Run tests with race detector
make lint    # go vet + test
make clean   # Remove build artifacts
make install # Build and install to $GOPATH/bin
make dist    # Cross-compile for all platforms
```

Run a single test: `go test ./internal/router/ -v`

## Architecture

**Purpose:** oc-go-cc is a proxy server that sits between Claude Code and OpenCode Go. It intercepts Anthropic API requests, transforms them to OpenAI Chat Completions format, forwards them to OpenCode Go, and transforms responses back to Anthropic SSE.

**Model routing is config-driven, not code-driven.** All models are defined in `~/.config/oc-go-cc/config.json` — adding a new model requires no code changes. Go provider models are transformed to OpenAI Chat Completions format automatically. Zen models use endpoint classification via `ClassifyEndpoint()`. The router in `internal/router/` selects models by matching request content against scenario patterns defined in `scenarios.go`.

If a model's upstream doesn't support Anthropic tool format (`type: "custom"` server-tool shorthands), set `"anthropic_tools_disabled": true` in the model config to force it through the Chat Completions transform path instead of the raw Anthropic endpoint.

**Two API endpoints:**

- OpenAI endpoint (`/v1/chat/completions`) — used by most models (GLM, Kimi, MiMo, Qwen)
- Anthropic endpoint (`/v1/messages`) — used only by MiniMax models

`internal/client/opencode.go` routes Go provider models to Chat Completions; Zen models are classified by `ClassifyEndpoint()`. If a model's upstream doesn't support Anthropic tool format, set `anthropic_tools_disabled: true` in config.

**Scenario detection priority** (`internal/router/scenarios.go`):

1. Long Context (>80K tokens, configurable) → MiniMax (1M context)
2. Complex (architectural patterns, tool operations) → GLM-5.1
3. Think (reasoning keywords in system prompt) → GLM-5
4. Background (simple read-only ops, no tools) → Qwen3.5 Plus
5. Default → Kimi K2.6

For streaming, the router downgrades to fast models (Qwen3.6 Plus) for better TTFT.

**Polymorphic field handling:** Anthropic's `system` and `content` fields accept both strings and arrays. `pkg/types/` uses `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats.

**Long-running stream policy:** The proxy never kills a stream that is actively producing bytes. The server-level `WriteTimeout` is set to 0; instead each upstream read uses a per-`Read` deadline via `http.ResponseController.SetReadDeadline` that is renewed on every successful byte. If the gap between bytes exceeds `OpenCodeGo.stream_timeout_ms` (or `OpenCodeZen.stream_timeout_ms`), the connection is treated as stuck and the request is routed to the next fallback model. Defaults to `timeout_ms` when unset. Client disconnects during a stream are logged at `Debug` level — this is normal during Claude Code tool execution and is not a failure signal.

## Key Files

- `cmd/oc-go-cc/main.go` — CLI entry point (cobra). Default config template is generated here.
- `internal/config/` — Config types and JSON loader with `${VAR}` env interpolation.
- `internal/transformer/` — Request/response format conversion (Anthropic ↔ OpenAI).
- `internal/router/fallback.go` — Circuit breaker per model (3 failures = 30s skip).
- `configs/config.example.json` — Reference config with all options documented.
