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
| `internal/client` | Upstream HTTP client, provider detection utilities |
| `internal/config` | JSON config loading, hot reload, atomic access |
| `internal/models` | Model classification utilities (endpoint type detection) |
| `internal/provider` | Provider-based dispatch (new path, replaces legacy client) |
| `internal/token` | Token counting via tiktoken |
| `internal/daemon` | Background mode, PID management, autostart |
| `internal/gui` | Embedded HTTP server for the webview dashboard (macOS only) |
| `internal/tray` | macOS system tray icon and menu (macOS only) |
| `internal/history` | In-memory ring buffer of recent proxy requests |
| `internal/metrics` | In-process request metrics collector |

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

**Endpoint classification** is handled by `internal/models.ClassifyEndpoint()`, which is shared between the client and provider packages to ensure consistent routing.

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

## macOS GUI Architecture

The macOS GUI (`routatic-proxy ui`) runs the proxy and a dashboard in a single process:

```
┌──────────────────────────────────────────────────┐
│  routatic-proxy (single process)                 │
│                                                   │
│  ┌──────────────┐   ┌──────────────────────────┐ │
│  │ Proxy Server  │   │  GUI Server              │ │
│  │ :3456         │   │  :random localhost port  │ │
│  │               │   │                          │ │
│  │ /v1/messages  │   │  / (static assets)       │ │
│  │ /health       │   │  /api/metrics            │ │
│  │ /statusline   │   │  /api/history            │ │
│  └──────┬───────┘   │  /api/config             │ │
│         │            │  /api/proxy/config       │ │
│         │            │  /api/proxy/start        │ │
│         │            │  /api/proxy/stop         │ │
│         │            └──────────┬───────────────┘ │
│         │                       │                  │
│         ▼                       ▼                  │
│  ┌──────────────┐   ┌──────────────────────────┐ │
│  │  History     │   │  Metrics                 │ │
│  │  (ring buf)  │   │  (counters)              │ │
│  └──────────────┘   └──────────────────────────┘ │
│         ▲                       ▲                 │
│         │   shared references   │                 │
│         └───────────────────────┘                 │
│                                                   │
│  ┌──────────────────────────────────────────┐    │
│  │  System Tray (systray)                    │    │
│  │  ● Open Console                           │    │
│  │  ● Start / Stop Proxy                     │    │
│  │  ● Start on Boot (checkbox)               │    │
│  │  ● Quit                                   │    │
│  └──────────────────────────────────────────┘    │
│                                                   │
│  ┌──────────────────────────────────────────┐    │
│  │  WebView (webview_go)                     │    │
│  │  Native macOS window with embedded HTML   │    │
│  │  Loads GUI server URL                     │    │
│  └──────────────────────────────────────────┘    │
└──────────────────────────────────────────────────┘
```

### Key Design Decisions

- **Single process**: The proxy and GUI share the same process, so History and Metrics instances are passed by reference — no IPC or shared memory needed.
- **Localhost-only GUI server**: The dashboard HTTP server binds to `127.0.0.1:0` (random available port), so only local processes can reach it.
- **Security headers**: All GUI server responses include `X-Content-Type-Options: nosniff` and `Content-Security-Policy` headers.
- **Partial config saves**: The Settings tab sends only changed fields as a JSON patch. The backend reads the current config from disk, merges the patch, and writes back — unchanged fields are preserved.
- **Nil-safe handlers**: The `/api/metrics` and `/api/history` endpoints handle nil dependencies gracefully (return zero values instead of panicking).

### GUI API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/metrics` | GET | Proxy running state, request counts, model distribution |
| `/api/history` | GET | Last 200 request records (newest first) |
| `/api/config` | GET/POST | GUI-level settings (autostart, notifications) |
| `/api/proxy/config` | GET/POST | Full proxy configuration (partial merge on POST) |
| `/api/proxy/start` | POST | Start the proxy server |
| `/api/proxy/stop` | POST | Stop the proxy server |

### Config Editing

The Settings tab exposes 28 config fields across 5 sections:

1. **Server** — listen host, listen port, global API key, hot reload toggle
2. **OpenCode Go** — base URL, Anthropic URL, API key, timeouts
3. **OpenCode Zen** — base URL, Anthropic URL, Responses URL, Gemini URL, API key, timeouts
4. **AWS Bedrock** — base URL, Anthropic URL, API key, project ID, timeouts
5. **Logging** — log level (debug/info/warn/error)

On save, only changed fields are sent to the server. The server merges them onto the current config from disk, writes the merged result, and reloads atomically — the running proxy picks up changes immediately.

### History Ring Buffer

The `internal/history` package maintains an in-memory ring buffer of the last 1000 request records. Each record includes:

- Request ID, model, provider, scenario
- Start time and duration
- Input/output token counts
- Streaming flag and success/failure status
- Error message (on failure)

The ring buffer uses head/tail indices for O(1) insert and returns a copy of each record on read (no shared reference to internal state).

### Scenario Tracking

Every history record now includes the routing scenario (e.g. "default", "complex", "think", "long_context", "background") that was selected for the request. This is threaded from the router's `RouteResult.Scenario` through the streaming and non-streaming handlers into the `history.RequestRecord`.

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

**Why not use Anthropic format everywhere?** Most upstream models only support OpenAI Chat Completions format. The proxy handles the translation so Claude Code doesn't need to know which provider it's talking to. Bedrock Mantle models also use Chat Completions format via the `aws-bedrock` provider.

**Why per-read idle timeout instead of WriteTimeout?** Claude Code's tool execution can pause streams for minutes. A server-level timeout would kill active streams. The per-byte watchdog only triggers when the upstream is truly stuck.

**Why rewrite DeepSeek system messages?** DeepSeek internally reorders all system-role messages to the front. Claude Code injects system reminders mid-conversation, which would shift the message history and invalidate the prefix cache. Wrapping them in `<system-reminder>` tags prevents reordering.
