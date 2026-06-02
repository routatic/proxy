# Configuration

## Config File

Location: `~/.config/oc-go-cc/config.json`

Override with `OC_GO_CC_CONFIG` environment variable.

## Full Config Reference

```json
{
  "api_key": "${OC_GO_CC_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,

  "models": {
    "default": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "background": {
      "provider": "opencode-go",
      "model_id": "qwen3.5-plus",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m2.7",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "qwen3.6-plus",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "glm-5" },
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" }
    ],
    "think": [{ "provider": "opencode-go", "model_id": "glm-5" }],
    "complex": [{ "provider": "opencode-go", "model_id": "glm-5" }],
    "long_context": [{ "provider": "opencode-go", "model_id": "minimax-m2.5" }],
    "fast": [{ "provider": "opencode-go", "model_id": "qwen3.5-plus" }]
  },

  "model_overrides": {
    "claude-sonnet-4.5": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  },

  "opencode_go": {
    "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/go/v1/messages",
    "timeout_ms": 300000
  },

  "opencode_zen": {
    "base_url": "https://opencode.ai/zen/v1/chat/completions",
    "anthropic_base_url": "https://opencode.ai/zen/v1/messages",
    "responses_base_url": "https://opencode.ai/zen/v1/responses",
    "gemini_base_url": "https://opencode.ai/zen/v1/models",
    "timeout_ms": 300000
  },

  "logging": {
    "level": "info",
    "requests": true
  }
}
```

## Providers

oc-go-cc supports two providers for upstream API calls:

### OpenCode Go (`opencode-go`)

- Default provider for most models
- Uses OpenAI Chat Completions and Anthropic Messages endpoints
- Pricing: $5/month subscription + usage-based

### OpenCode Zen (`opencode-zen`)

- Curated, tested models with pay-as-you-go pricing
- Supports additional endpoint formats:
  - **Chat Completions** (`/v1/chat/completions`) — OpenAI-compatible models
  - **Anthropic Messages** (`/v1/messages`) — Claude, Qwen models
  - **OpenAI Responses** (`/v1/responses`) — GPT models
  - **Google Gemini** (`/v1/models/{id}`) — Gemini models
- Set `"provider": "opencode-zen"` in your model config to use Zen

## Environment Variables

Environment variables override config file values. Config values also support `${VAR}` interpolation.

| Variable                | Description                                 | Default                                          |
| ----------------------- | ------------------------------------------- | ------------------------------------------------ |
| `OC_GO_CC_API_KEY`      | OpenCode Go API key (**required**)          | —                                                |
| `OC_GO_CC_CONFIG`       | Custom config file path                     | `~/.config/oc-go-cc/config.json`                 |
| `OC_GO_CC_HOST`         | Proxy listen host                           | `127.0.0.1`                                      |
| `OC_GO_CC_PORT`         | Proxy listen port                           | `3456`                                           |
| `OC_GO_CC_OPENCODE_URL` | OpenCode Go API endpoint                    | `https://opencode.ai/zen/go/v1/chat/completions` |
| `OC_GO_CC_OPENCODE_ZEN_URL` | OpenCode Zen API endpoint              | `https://opencode.ai/zen/v1/chat/completions`    |
| `OC_GO_CC_LOG_LEVEL`    | Log level: `debug`, `info`, `warn`, `error` | `info`                                           |

## Hot Reload

By default, config changes require a server restart. Enable hot reload to watch for config file changes and apply them automatically:

```json
{
  "hot_reload": true
}
```

When enabled, the proxy watches the config directory for changes (handling editors that save via rename/create) and reloads the config automatically. You can also trigger a manual reload by sending `SIGHUP` to the process:

```bash
kill -HUP <PID>
```

## Model Routing

The proxy automatically detects the type of request and routes to the appropriate model based on context size and content analysis:

| Scenario         | Trigger                                             | Model        | Why                                             |
| ---------------- | --------------------------------------------------- | ------------ | ----------------------------------------------- |
| **Long Context** | >80K tokens (configurable)                          | MiniMax M2.7 | 1M context window vs 128-256K for others        |
| **Complex**      | "architect", "refactor", "complex" in system prompt | GLM-5.1      | Best reasoning & architectural understanding    |
| **Think**        | "think", "plan", "reason" in system prompt          | GLM-5        | Good reasoning, cheaper than GLM-5.1            |
| **Background**   | "read file", "grep", "list directory"               | Qwen3.5 Plus | Cheapest (~10K req/5hr), perfect for simple ops |
| **Default**      | Everything else                                     | Kimi K2.6    | Best balance of quality & cost (~1.8K req/5hr)  |

**See [MODELS.md](MODELS.md) for detailed model capabilities, costs, and routing recommendations.**

DeepSeek V4 users can set any scenario model to `deepseek-v4-pro` or `deepseek-v4-flash`. For deterministic max thinking, add `reasoning_effort: "max"` and `thinking: {"type":"enabled"}` to that scenario's model config and fallback entries.

### Routing in Detail

| Scenario         | Trigger                                                                      | Config Key            | Default Model  |
| ---------------- | ---------------------------------------------------------------------------- | --------------------- | -------------- |
| **Default**      | Standard chat                                                                | `models.default`      | `kimi-k2.6`    |
| **Think**        | System prompt contains "think", "plan", "reason"; or thinking content blocks | `models.think`        | `glm-5.1`      |
| **Long Context** | Token count exceeds `context_threshold`                                      | `models.long_context` | `minimax-m2.7` |
| **Background**   | File read, directory list, grep patterns                                     | `models.background`   | `qwen3.5-plus` |

Routing priority: **Long Context** > **Think** > **Background** > **Default**

## Fallback Chains

When a model request fails (network error, rate limit, server error), the proxy tries the next model in the fallback chain:

```
Primary model -> Fallback 1 -> Fallback 2 -> ... -> Error (all failed)
```

Each model also has a **circuit breaker** that tracks consecutive failures. After 3 failures, the circuit opens and that model is skipped for 30 seconds, then tested again (half-open state).

## Model Overrides (`model_overrides`)

`model_overrides` lets you map a specific client-requested model name (the value of the `model` field in `/v1/messages`) to a fixed `ModelConfig`. This is useful when you want clients to be able to request a particular model (e.g. `claude-sonnet-4.5`) without that model going through the scenario router.

When a request arrives, the proxy checks `model_overrides[<model>]` **first**. If the requested model has an entry, the override is used as the primary. The fallback chain is `fallbacks[<model>]`, falling back to `fallbacks["default"]` if no override-specific entry exists. The scenario-routed chain is then appended as a **safety-net fallback** (deduplicated by `model_id`).

```json
{
  "model_overrides": {
    "claude-sonnet-4.5": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  }
}
```

Each entry accepts the same fields as a `ModelConfig` (`provider`, `model_id`, `temperature`, `max_tokens`, `reasoning_effort`, `thinking`, etc.). `model_id` is required; `provider` must be `"opencode-go"` or `"opencode-zen"` (or omitted to inherit the default).

### Routing precedence

When a request arrives, the proxy selects a model chain using the following order:

1. **`model_overrides[<model>]`** — if the request's `model` field has an entry, use it as the primary and append the scenario chain as a safety net.
2. **`respect_requested_model`** — if enabled and `models[<model>]` is configured, use the requested model with default fallbacks.
3. **Scenario routing** — fall back to the scenario chain (`default`, `background`, `think`, `complex`, `long_context`, `fast`).

> **Trust model:** any client whose requests flow through the proxy can select from the configured `model_overrides` set without additional authentication. If you run the proxy as a shared service, treat `model_overrides` as a privileged allowlist.
