# Contributing

## Development

```bash
# Build (version auto-detected from git)
make build

# Run in development mode
make run

# Run tests with race detector
make test

# Run go vet
make vet

# Clean build artifacts
make clean

# Install to $GOPATH/bin
make install

# Build cross-platform release binaries
make dist
```

Run a single test: `go test ./internal/router/ -v`

## How It Works

```
┌─────────────┐     Anthropic API      ┌─────────────┐     OpenAI/Gemini/Responses  ┌─────────────┐
│  Claude Code ├──────────────────────►│  oc-go-cc    ├─────────────────────────────►│  OpenCode   │
│  (CLI)       │  POST /v1/messages   │  (Proxy)     │  Multiple endpoint formats   │  (Upstream) │
│              │◄──────────────────────┤              │◄─────────────────────────────┤              │
└─────────────┘   Anthropic SSE        └─────────────┘   Format-appropriate SSE      └─────────────┘
```

1. Claude Code sends a request in [Anthropic Messages API](https://docs.anthropic.com/en/api/messages) format
2. oc-go-cc parses the request, counts tokens, and selects a model via routing rules
3. Based on the model's provider and endpoint type, the request is transformed to the appropriate format:
   - **OpenAI Chat Completions** — for most OpenCode Go and Zen models
   - **Anthropic Messages** — for MiniMax models (sent directly without transformation)
   - **OpenAI Responses** — for GPT models on Zen
   - **Google Gemini** — for Gemini models on Zen
4. The transformed request is sent to the appropriate OpenCode endpoint
5. The response (streaming or non-streaming) is transformed back to Anthropic format
6. Claude Code receives the response as if it came from Anthropic directly

### What Gets Transformed

| Anthropic                                                    | OpenAI/Responses/Gemini                          |
| ------------------------------------------------------------ | ----------------------------------------------- |
| `system` (string or array)                                   | `messages[0]` with `role: "system"` (OpenAI) or `developer` role (Responses) |
| `content: [{"type":"text","text":"..."}]`                    | `content: "..."` (OpenAI) or `parts: [{text}]` (Gemini) |
| `tool_use` content blocks                                    | `tool_calls` array (OpenAI) or `function_call` (Responses) |
| `tool_result` content blocks                                 | `role: "tool"` messages (OpenAI)                |
| `thinking` content blocks                                    | `reasoning_content` (OpenAI)                    |
| `stop_reason: "end_turn"`                                    | `finish_reason: "stop"` (OpenAI) or `STOP` (Gemini) |
| `stop_reason: "tool_use"`                                    | `finish_reason: "tool_calls"` (OpenAI)          |
| SSE `message_start` / `content_block_delta` / `message_stop` | SSE format-appropriate events                   |

### DeepSeek V4 Thinking Mode

DeepSeek V4 Pro and Flash use the OpenAI-compatible `/chat/completions` endpoint through OpenCode Go. They support thinking mode and configurable reasoning effort.

For Claude Code and other agentic coding workflows, configure DeepSeek V4 models with:

```json
{
  "provider": "opencode-go",
  "model_id": "deepseek-v4-pro",
  "max_tokens": 8192,
  "reasoning_effort": "max",
  "thinking": {
    "type": "enabled"
  }
}
```

`oc-go-cc` forwards these fields to OpenCode Go as OpenAI Chat Completions parameters:

- `reasoning_effort`: controls DeepSeek V4 thinking effort (`high` or `max`)
- `thinking`: enables or disables DeepSeek V4 thinking mode

DeepSeek V4 thinking responses are returned as OpenAI `reasoning_content` and transformed back into Anthropic `thinking` blocks for Claude Code.

## Architecture

```
cmd/oc-go-cc/main.go           CLI entry point (cobra commands)
internal/
├── config/
│   ├── config.go               Config types (OpenCodeGoConfig, OpenCodeZenConfig)
│   ├── loader.go               JSON loading, env overrides, ${VAR} interpolation
│   ├── watcher.go              Hot reload file watcher (fsnotify)
│   └── atomic.go               Atomic config swap for concurrent access
├── router/
│   ├── model_router.go         Model selection based on scenario
│   ├── scenarios.go            Scenario detection (default/think/long_context/background)
│   └── fallback.go             Fallback handler with circuit breaker
├── server/
│   └── server.go               HTTP server setup, graceful shutdown, PID management
├── handlers/
│   ├── messages.go             POST /v1/messages handler (streaming + non-streaming)
│   └── health.go               Health check and token counting endpoints
├── transformer/
│   ├── request.go              Anthropic → OpenAI/Responses/Gemini request transformation
│   ├── response.go             OpenAI/Responses/Gemini → Anthropic response transformation
│   └── stream.go               Real-time SSE stream transformation for all formats
├── client/
│   └── opencode.go             OpenCode client with provider-aware routing
├── daemon/
│   ├── launchd.go              macOS launchd plist management
│   ├── background.go           Background daemon fork
│   └── process.go              PID file and process management
└── token/
    └── counter.go              Tiktoken token counter (cl100k_base)
pkg/types/
├── anthropic.go                Anthropic API types (polymorphic system/content fields)
├── openai.go                   OpenAI Chat Completions types
└── zen.go                      OpenAI Responses and Google Gemini types
configs/
└── config.example.json         Example configuration
```

### Key Design Decisions

- **Polymorphic field handling**: Anthropic's `system` and `content` fields accept both strings and arrays. We use `json.RawMessage` with accessor methods (`SystemText()`, `ContentBlocks()`) to handle both formats correctly.
- **Real-time stream proxying**: SSE events are transformed in-flight, not buffered. This means Claude Code sees responses as they arrive from upstream.
- **Circuit breaker per model**: Each model gets its own circuit breaker. After 3 consecutive failures, the model is skipped for 30 seconds, then tested again.
- **Environment variable interpolation**: Config values like `"${OC_GO_CC_API_KEY}"` are resolved at load time, so you never need to put secrets in the config file.
- **Provider-aware routing**: The `provider` field in model config determines which upstream service to use (Go or Zen). Zen models are further classified by endpoint type (Chat Completions, Anthropic, Responses, Gemini).

## API Endpoints

The proxy exposes these endpoints that Claude Code expects:

| Method | Path                        | Description                           |
| ------ | --------------------------- | ------------------------------------- |
| `POST` | `/v1/messages`              | Main chat endpoint (Anthropic format) |
| `POST` | `/v1/messages/count_tokens` | Token counting                        |
| `GET`  | `/health`                   | Health check                          |
