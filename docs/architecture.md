# Architecture

routatic-proxy sits between Claude Code and upstream model providers, intercepting Anthropic API requests and routing them to the optimal model. The design prioritizes cost efficiency, reliability, and zero-config operation.

## Request Flow

```
Claude Code
    │
    ▼
┌─────────────────────────────────────────────┐
│  routatic-proxy                             │
│                                             │
│  1. Parse Anthropic MessageRequest          │
│  2. Count tokens (tiktoken cl100k_base)     │
│  3. Analyze request facts                   │
│     - Has images? New images?               │
│     - Complex patterns? Thinking patterns?  │
│  4. Detect scenario                         │
│  5. Select model + fallback chain           │
│  6. Transform to provider format            │
│  7. Forward to upstream                     │
│  8. Transform response back to Anthropic    │
│  9. Stream SSE to Claude Code               │
└─────────────────────────────────────────────┘
    │
    ▼
OpenCode Go / OpenCode Zen
```

## Core Modules

| Module | Purpose |
|--------|---------|
| `internal/handlers` | HTTP request handling, message parsing, orchestration |
| `internal/router` | Scenario detection, model selection, fallback chains |
| `internal/transformer` | Anthropic ↔ OpenAI/Responses/Gemini format conversion |
| `internal/client` | Upstream HTTP client, endpoint classification |
| `internal/config` | JSON config loading, hot reload, atomic access |
| `internal/provider` | Provider-based dispatch (new path, replaces legacy client) |
| `internal/token` | Token counting via tiktoken |
| `internal/daemon` | Background mode, PID management, autostart |

## Scenario-Based Routing

The router analyzes each request and assigns it a scenario. Each scenario maps to a primary model and a fallback chain, all config-driven.

**Detection priority** (highest to lowest):

1. **Long Context** — token count exceeds `context_threshold` (default 100K) → MiniMax (1M context)
2. **Vision** — request contains images → vision-capable model
3. **Complex** — architectural patterns, tool operations → GLM-5.1
4. **Think** — reasoning keywords in system prompt → GLM-5
5. **Background** — simple read-only ops, no tools → Qwen3.5 Plus
6. **Default** → Kimi K2.6

**Streaming override**: when `enable_streaming_scenario_routing` is false (default), streaming requests always route to the `fast` model (Qwen3.6 Plus) for better TTFT.

## Request Transformation

Claude Code sends Anthropic Messages API format. The proxy transforms to the provider's native format:

| Provider | Format | Endpoint |
|----------|--------|----------|
| OpenCode Go (most models) | OpenAI Chat Completions | `/v1/chat/completions` |
| OpenCode Go (MiniMax, Qwen) | Anthropic Messages | `/v1/messages` |
| OpenCode Zen (Claude, Qwen) | Anthropic Messages | `/v1/messages` |
| OpenCode Zen (GPT models) | OpenAI Responses | `/v1/responses` |
| OpenCode Zen (Gemini) | Gemini | `/v1/models/{id}` |

**Key transformation details:**

- Anthropic `tool_use` ↔ OpenAI `function_calling` bidirectional translation
- Anthropic `thinking` blocks ↔ OpenAI `reasoning_content` preservation
- DeepSeek system message rewriting to prevent prefix cache invalidation
- Image blocks: base64 data URL for vision models, `[Image]` placeholder for non-vision
- `cache_control` stripping for non-DeepSeek models

## Fallback & Circuit Breaker

When a model fails, the proxy tries the next model in the chain. The circuit breaker prevents repeated calls to failing models:

```
Closed (normal) → 3 failures → Open (skip) → 30s timeout → Half-Open (test) → success → Closed
                                                                            → failure → Open
```

- Only 5xx errors and network failures trigger the circuit breaker
- 4xx errors (bad request, rate limit) skip the breaker — retrying won't help
- Per-model tracking: each model has its own circuit breaker

## Streaming Architecture

Streaming uses a per-byte idle watchdog instead of a server-level write timeout:

1. Server `WriteTimeout` is 0 (disabled) — long SSE streams must not be killed mid-flight
2. Each upstream read uses `http.ResponseController.SetReadDeadline` that resets on every successful byte
3. If no byte arrives within `stream_timeout_ms`, the connection is treated as stuck
4. Heartbeat comments (`:keepalive\n\n`) are sent every 3s to keep the connection alive
5. Client disconnects during stream are logged at Debug level — normal during tool execution

## Configuration Hot Reload

When `hot_reload: true`, a filesystem watcher monitors the config file. Changes are applied atomically via `AtomicConfig` — all readers see a consistent snapshot without locks. The HTTP server, model router, and provider registry all read from the same atomic pointer.

## Polymorphic Field Handling

Anthropic's `system` and `content` fields accept both strings and arrays. The `pkg/types` package uses `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats transparently.

## Design Decisions

**Why config-driven routing?** Adding a new model requires zero code changes — just add an entry to `config.json`. The scenario detector, fallback chains, and model metadata are all declarative.

**Why not use Anthropic format everywhere?** Most upstream models only support OpenAI Chat Completions format. The proxy handles the translation so Claude Code doesn't need to know which provider it's talking to.

**Why per-read idle timeout instead of WriteTimeout?** Claude Code's tool execution can pause streams for minutes. A server-level timeout would kill active streams. The per-byte watchdog only triggers when the upstream is truly stuck.

**Why rewrite DeepSeek system messages?** DeepSeek internally reorders all system-role messages to the front. Claude Code injects system reminders mid-conversation, which would shift the message history and invalidate the prefix cache. Wrapping them in `<system-reminder>` tags prevents reordering.
