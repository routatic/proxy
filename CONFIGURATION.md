# Configuration

## Config File

Location: `~/.config/routatic-proxy/config.json`

Override with `ROUTATIC_PROXY_CONFIG` environment variable.

For migration, `~/.config/oc-go-cc/config.json` is loaded when the new config file does not exist, and every `OC_GO_CC_*` environment variable is still accepted as a fallback for its `ROUTATIC_PROXY_*` replacement.

## Full Config Reference

```json
{
  "api_key": "${ROUTATIC_PROXY_API_KEY}",
  "host": "127.0.0.1",
  "port": 3456,
  "hot_reload": false,
  "anthropic_first": {
    "enabled": false,
    "base_url": "https://api.anthropic.com"
  },

  "enable_cost_based_routing": false,
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["opencode-go", "aws-bedrock"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.05
    }
  },

  "models": {
    "default": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "background": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.5,
      "max_tokens": 2048
    },
    "think": {
      "provider": "opencode-go",
      "model_id": "glm-5.2",
      "temperature": 0.7,
      "max_tokens": 8192
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    },
    "fast": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  },

  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "qwen3.7-plus" },
      { "provider": "opencode-go", "model_id": "qwen3.7-max" },
      { "provider": "opencode-zen", "model_id": "nemotron-3-ultra-free" },
      { "provider": "opencode-zen", "model_id": "mimo-v2.5-free" },
      { "provider": "opencode-zen", "model_id": "deepseek-v4-flash-free" }
    ],
    "think": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }],
    "complex": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }],
    "long_context": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }],
    "fast": [{ "provider": "opencode-go", "model_id": "qwen3.7-plus" }]
  },

  "model_overrides": {
    "claude-sonnet-4.5": {
      "provider": "opencode-zen",
      "model_id": "claude-sonnet-4.5",
      "temperature": 0.7,
      "max_tokens": 8192,
      "vision": true
    },
    "deepseek-v4-pro": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": {
        "type": "enabled"
      }
    },
    "deepseek-v4-flash-free": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-flash-free",
      "temperature": 0.7,
      "max_tokens": 4096
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

## Anthropic-First Failover

Enable this mode to keep Anthropic as Claude Code's primary API and use the configured OpenCode model chain only while Anthropic is unavailable:

```json
{
  "anthropic_first": {
    "enabled": true,
    "base_url": "https://api.anthropic.com"
  }
}
```

Configure Claude Code with only the proxy address:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
unset ANTHROPIC_AUTH_TOKEN ANTHROPIC_API_KEY
```

Leaving the credential variables unset preserves the saved Claude Pro, Max, Team, or Enterprise login. The proxy forwards the raw request, OAuth credential, `anthropic-version`, and complete `anthropic-beta` capability header to Anthropic.

Fallback occurs for HTTP 408, 429, 5xx, and transport failures before a response starts. HTTP 400, 401, 403, 404, and other request errors are returned unchanged. After a failure, the proxy honors `Retry-After`; otherwise it uses exponential backoff from 30 seconds to 15 minutes. One real user request probes recovery while concurrent requests continue through OpenCode. No synthetic health requests are sent.

Once response bytes have started, a failed stream cannot be restarted on another model without duplicating content. `/v1/messages/count_tokens` remains local and does not affect availability state.

When OpenCode Go returns `GoUsageLimitError`, remaining Go models are skipped for that request and the chain advances to Zen. The default chain uses Qwen3.7 Plus, Qwen3.7 Max, then the currently working Zen-free Nemotron 3 Ultra, MiMo V2.5, and DeepSeek V4 Flash models. Free Zen endpoints are time-limited and may retain data under [OpenCode's documented privacy terms](https://opencode.ai/docs/zen/#privacy).

## Providers

routatic-proxy supports three providers for upstream API calls:

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

### AWS Bedrock (`aws-bedrock`)

- Models hosted on AWS Bedrock Mantle
- Supports two wire formats:
  - **OpenAI Chat Completions** (`/v1/chat/completions`) — default, works with most models
  - **Anthropic Messages** (`/v1/messages`) — for Claude and other Anthropic-native models
- Supports the `OpenAI-Project` header for project-based routing
- Bedrock-specific API key falls back to the global key pool if unset
- Set `"provider": "aws-bedrock"` in your model config to use Bedrock

```json
{
  "aws_bedrock": {
    "base_url": "https://bedrock-mantle.us-east-1.api.aws/v1/chat/completions",
    "anthropic_base_url": "https://bedrock-mantle.us-east-1.api.aws/v1/messages",
    "api_key": "${BEDROCK_API_KEY}",
    "project_id": "proj_xxx",
    "wire_format": "openai",
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000,
    "streaming_timeout_ms": 600000
  }
}
```

Set `wire_format: "anthropic"` for models that need raw Anthropic Messages format (e.g., Claude on Bedrock). Requires `anthropic_base_url` to be configured.

### OpenRouter (`openrouter`)

- Unified API for accessing 200+ models from multiple providers (OpenAI, Anthropic, Google, Meta, Mistral, and more)
- Uses OpenAI Chat Completions API format
- Pay-as-you-go pricing with competitive rates
- Set `"provider": "openrouter"` in your model config to use OpenRouter

#### Configuration Schema

```json
{
  "openrouter": {
    "name": "openrouter",
    "base_url": "https://openrouter.ai/api/v1",
    "api_key": "${OPENROUTER_API_KEY}",
    "api_keys": ["${OPENROUTER_KEY_1}", "${OPENROUTER_KEY_2}"],
    "enabled": true,
    "timeout_ms": 300000,
    "stream_timeout_ms": 60000
  }
}
```

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | `string` | No | Provider display name (defaults to "openrouter") |
| `base_url` | `string` | No | API endpoint base URL. Default: `https://openrouter.ai/api/v1` |
| `api_key` | `string` | Yes* | Single API key for authentication. Required if `api_keys` not set |
| `api_keys` | `string[]` | Yes* | Multiple API keys for round-robin rotation. Required if `api_key` not set |
| `enabled` | `bool` | No | Whether this provider is active. Default: `true` |
| `timeout_ms` | `int` | No | Request timeout in milliseconds. Default: `300000` (5 minutes) |
| `stream_timeout_ms` | `int` | No | Per-chunk timeout during streaming. Default: `60000` (1 minute) |

*At least one of `api_key` or `api_keys` must be configured.

#### Environment Variable Overrides

| Variable | Description | Precedence |
|----------|-------------|------------|
| `ROUTATIC_PROXY_OPENROUTER_API_KEY` | Single API key override | Highest |
| `ROUTATIC_PROXY_OPENROUTER_API_KEYS` | Comma-separated keys for round-robin | Highest |
| `ROUTATIC_PROXY_OPENROUTER_BASE_URL` | Custom base URL override | Highest |

Environment variables take precedence over config file values. Config values support `${VAR}` interpolation.

Precedence order: `*_API_KEYS` → `*_API_KEY` → config file `api_keys` → config file `api_key`

#### Example Configurations

**Single-key setup:**

```json
{
  "openrouter": {
    "api_key": "sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxx"
  }
}
```

**Multi-key round-robin for load balancing:**

```json
{
  "openrouter": {
    "api_keys": [
      "sk-or-v1-key-1",
      "sk-or-v1-key-2",
      "sk-or-v1-key-3"
    ]
  }
}
```

**Custom base URL (for enterprise/self-hosted):**

```json
{
  "openrouter": {
    "base_url": "https://openrouter.mycompany.com/api/v1",
    "api_key": "${OPENROUTER_API_KEY}",
    "enabled": true
  }
}
```

#### Integration with Cost-Based Routing

OpenRouter works seamlessly with `cost_routing`. Use `penalty_per_provider` to adjust effective costs:

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["openrouter", "opencode-go"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.02,
      "opencode-go": 0.0,
      "aws-bedrock": 0.05
    }
  }
}
```

Penalties are additive to the raw model cost. Example: a model costing $0.10/1M tokens on OpenRouter with a 0.02 penalty has effective cost $0.12/1M tokens. Use this to bias routing preferences without excluding providers entirely.

#### Model Resolution via Catalog

Models are referenced using the `provider/model-name` pattern. OpenRouter models use the `openrouter/` prefix:

```json
{
  "model_overrides": {
    "claude-opus-4": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192,
      "vision": true
    },
    "gpt-4o": {
      "provider": "openrouter",
      "model_id": "openai/gpt-4o",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "gemini-2.5-pro": {
      "provider": "openrouter",
      "model_id": "google/gemini-2.5-pro-preview-07-11",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  }
}
```

**Discovering models:**

1. Visit [openrouter.ai/models](https://openrouter.ai/models) for the complete model list
2. Use the `routatic-proxy models` command to see cached catalog entries
3. Check the [OpenRouter API docs](https://openrouter.ai/docs) for pricing and context limits

The `model_id` in your config must match OpenRouter's model identifier exactly (e.g., `anthropic/claude-opus-4`, `openai/gpt-4o`, `google/gemini-2.5-pro-preview-07-11`).

#### Use Cases

**Accessing specific models:** Use OpenRouter when you need models not available on other providers:

```json
{
  "models": {
    "complex": {
      "provider": "openrouter",
      "model_id": "anthropic/claude-opus-4",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max"
    }
  }
}
```

**Fallback chains:** Include OpenRouter as a fallback when primary providers fail:

```json
{
  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "deepseek-v4-pro" },
      { "provider": "openrouter", "model_id": "anthropic/claude-sonnet-4.8" },
      { "provider": "openrouter", "model_id": "openai/gpt-4.1" }
    ]
  }
}
```

**Cost optimization:** Use `cost_routing` with provider penalties to automatically select the cheapest available model:

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["openrouter"],
    "penalty_per_provider": {
      "openrouter": -0.01
    }
  }
}
```

**Specialized models:** Access niche models for specific tasks:

```json
{
  "models": {
    "think": {
      "provider": "openrouter",
      "model_id": "deepseek/deepseek-r1-free",
      "temperature": 0.6,
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "openrouter",
      "model_id": "google/gemini-1.5-pro",
      "temperature": 0.7,
      "max_tokens": 16384,
      "context_threshold": 80000
    }
  }
}
```

## Environment Variables

Environment variables override config file values. Config values also support `${VAR}` interpolation.

| Variable                | Description                                 | Default                                          |
| ----------------------- | ------------------------------------------- | ------------------------------------------------ |
| `ROUTATIC_PROXY_API_KEY`      | OpenCode Go API key (**required**)          | —                                                |
| `ROUTATIC_PROXY_CONFIG`       | Custom config file path                     | `~/.config/routatic-proxy/config.json`                 |
| `ROUTATIC_PROXY_HOST`         | Proxy listen host                           | `127.0.0.1`                                      |
| `ROUTATIC_PROXY_PORT`         | Proxy listen port                           | `3456`                                           |
| `ROUTATIC_PROXY_OPENCODE_URL` | OpenCode Go API endpoint                    | `https://opencode.ai/zen/go/v1/chat/completions` |
| `ROUTATIC_PROXY_OPENCODE_ZEN_URL` | OpenCode Zen API endpoint              | `https://opencode.ai/zen/v1/chat/completions`    |
| `ROUTATIC_PROXY_OPENROUTER_API_KEY` | OpenRouter single API key           | —                                                |
| `ROUTATIC_PROXY_OPENROUTER_API_KEYS` | OpenRouter key pool (comma-separated) | —                                             |
| `ROUTATIC_PROXY_LOG_LEVEL`    | Log level: `debug`, `info`, `warn`, `error` | `info`                                           |

Legacy equivalents such as `OC_GO_CC_API_KEY`, `OC_GO_CC_CONFIG`, and `OC_GO_CC_PORT` continue to work. When both names are set, the `ROUTATIC_PROXY_*` value wins.

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

## Cost-Based Routing

When enabled, the proxy uses a catalog of model pricing data to automatically select the cheapest eligible model for each scenario, rather than always using the statically configured primary model.

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["opencode-go", "aws-bedrock"],
    "max_context_window": 1000000,
    "penalty_per_provider": {
      "openrouter": 0.05
    }
  }
}
```

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | `bool` | Activates cost-aware model selection. Can also be set via the legacy `enable_cost_based_routing` top-level flag. |
| `prefer_providers` | `string[]` | Restricts candidate providers globally. When set, only models on these providers are considered. Intersected with per-scenario `preferred_providers` when both are set. |
| `max_context_window` | `int64` | Hard cap on candidate model context window. Models exceeding this size are excluded. `0` (default) means no cap. |
| `penalty_per_provider` | `map[string]float64` | Per-provider cost penalty added to the effective cost during selection. Use this to bias away from providers without removing them entirely. |

When enabled, `SelectCheapest` resolves all eligible provider/model pairs for the matched scenario, applies the max context window cap, filters by the preferred providers set, and sorts by effective cost (raw cost + penalty). The cheapest candidate wins. This replaces the static `models.<scenario>` primary model.

```json
{
  "cost_routing": {
    "penalty_per_provider": {
      "opencode-go": 0.1,
      "openrouter": 0.05
    }
  }
}
```

Penalties are additive to the raw cost. A model on `opencode-go` with base cost 2.0 and a penalty of 0.1 has effective cost 2.1.

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
      "max_tokens": 8192,
      "vision": true
    },
    "deepseek-v4-pro": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": {
        "type": "enabled"
      }
    }
  }
}
```

Each entry accepts the same fields as a `ModelConfig` (`provider`, `model_id`, `temperature`, `max_tokens`, `reasoning_effort`, `thinking`, etc.). `model_id` is required; `provider` must be `"opencode-go"`, `"opencode-zen"`, or `"aws-bedrock"` (or omitted to inherit the default).

See `routatic-proxy models` for the complete list of available Zen models across all endpoint types (Claude, GPT, Gemini, and free-tier).

### Routing precedence

When a request arrives, the proxy selects a model chain using the following order:

1. **`model_overrides[<model>]`** — if the request's `model` field has an entry, use it as the primary and append the scenario chain as a safety net.
2. **`respect_requested_model`** — if enabled and `models[<model>]` is configured, use the requested model with default fallbacks.
3. **Scenario routing** — fall back to the scenario chain (`default`, `background`, `think`, `complex`, `long_context`, `fast`).

> **Trust model:** any client whose requests flow through the proxy can select from the configured `model_overrides` set without additional authentication. If you run the proxy as a shared service, treat `model_overrides` as a privileged allowlist.

### Streaming Scenario Routing

`enable_streaming_scenario_routing` controls whether streaming requests are evaluated by the full scenario router or routed directly to the `fast` scenario.

> **Note for Claude Code `/review-code`, `/ultracode`, and multi-agent workflows**
>
> If you use Claude Code workflows that dispatch many subagents or produce many parallel tool calls, enable streaming scenario routing:
>
> ```json
> {
>   "enable_streaming_scenario_routing": true
> }
> ```
>
> Without this option, streaming requests are routed through the `fast` scenario even when the request is actually tool-heavy. This can route complex Claude Code workloads, such as `/review-code` with many `Agent` tool calls, to a fast model that may not handle parallel tool-call orchestration reliably.
>
> When enabled, streaming requests are evaluated by the same scenario router as non-streaming requests, allowing large or tool-heavy workloads to use `complex` or `long_context` models instead of always using the `fast` model.

Recommended setup for Claude Code review workflows:

```json
{
  "enable_streaming_scenario_routing": true,
  "models": {
    "fast": {
      "provider": "opencode-go",
      "model_id": "deepseek-v4-flash",
      "max_tokens": 4096
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "max_tokens": 8192
    },
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m3",
      "max_tokens": 16384,
      "context_threshold": 80000
    }
  }
}
```

Use the `fast` scenario for short/simple requests. Use `complex` or `long_context` for code review, multi-agent dispatch, large diffs, many tools, or long-context Claude Code sessions.

## Claude Code Model Picker

You can select proxy models from Claude Code's `/model` picker in two ways.

### Type any model name (always works)

Claude Code's `/model` picker also accepts a free-form model name. Type any value the proxy understands — a scenario alias (`default`, `fast`, `complex`, …), a `model_overrides` key, or a catalog canonical name like `opencode-go/kimi-k2.6` — and the proxy routes it. No extra configuration is needed; this works regardless of Claude Code version.

### Gateway model discovery (opt-in, adds entries to the picker)

Recent Claude Code versions can auto-populate the picker by querying the proxy's [`GET /v1/models`](docs/reference-api.md#get-v1models) endpoint. When enabled, discovered models appear in `/model` labeled **"From gateway"** alongside the built-in entries (Sonnet, Opus, …).

Enable it by setting, alongside `ANTHROPIC_BASE_URL`:

```bash
export ANTHROPIC_BASE_URL=http://127.0.0.1:3456
export ANTHROPIC_AUTH_TOKEN=unused
export CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1
```

Discovery only runs when all of these hold: `ANTHROPIC_BASE_URL` is set, `CLAUDE_CODE_ENABLE_GATEWAY_MODEL_DISCOVERY=1`, no `CLAUDE_CODE_USE_*` provider variable is set, the base URL is not `api.anthropic.com`, and the Claude Code version supports it (≥ 2.1.129). Results are cached to `~/.claude/cache/gateway-models.json`.

> **Important — Claude Code filters discovered model IDs.** Claude Code only shows discovered models whose `id` begins with **`claude`** or **`anthropic`**. The proxy's scenario aliases (`default`, `fast`, …) and catalog names (`opencode-go/kimi-k2.6`) are therefore **filtered out of the picker**. To make a proxy model appear via discovery, give it a `claude-*` name — the natural fit is a [`model_overrides`](#model-overrides-model_overrides) key:
>
> ```json
> {
>   "model_overrides": {
>     "claude-glm-5.2": { "provider": "opencode-go", "model_id": "glm-5.2" }
>   }
> }
> ```
>
> `claude-glm-5.2` then appears in the picker (labeled "From gateway"), and selecting it routes to GLM-5.2. Models with non-`claude`/`anthropic` IDs remain fully usable — just type them into `/model` directly.

## Using with CC-Switch

[CC-Switch](https://github.com/farion1231/cc-switch) is a desktop app for managing and hot-switching Claude Code providers. routatic-proxy works with it out of the box — the proxy speaks the Anthropic API that Claude Code (and therefore CC-Switch) already expects, so you add it like any other custom provider.

### Add routatic-proxy as a custom provider

1. Start the proxy: `routatic-proxy serve` (default listen address `http://127.0.0.1:3456`).
2. In CC-Switch, click **Add Provider → Custom** and fill in:

   | CC-Switch field | Value |
   |-----------------|-------|
   | **Name** | `routatic-proxy` (any label) |
   | **Endpoint URL** | `http://127.0.0.1:3456` |
   | **API Key** | any non-empty value (e.g. `unused`) — see note below |

   CC-Switch writes these into Claude Code's config as:

   ```json
   {
     "env": {
       "ANTHROPIC_BASE_URL": "http://127.0.0.1:3456",
       "ANTHROPIC_AUTH_TOKEN": "unused"
     }
   }
   ```

   These are the exact two environment variables the proxy relies on — the same ones from the manual quickstart in the [README](README.md).
3. **Enable** the provider. Claude Code hot-reloads it, so no restart is needed.

> **About the API Key field:** the token in `ANTHROPIC_AUTH_TOKEN` is what Claude Code sends to the *proxy*, not what the proxy sends upstream. Your real upstream keys live in the proxy's own config (`opencode_go.api_key`, `openrouter.api_key`, etc.) or environment (`ROUTATIC_PROXY_*`). If you set `api_key` / `api_keys` in the proxy config, that value must match what CC-Switch sends; if you leave proxy auth unset, any non-empty token works.

### Configure specific models

You have two ways to control which model a CC-Switch-selected request runs on:

- **Let Claude Code pick, and honor it** — with `respect_requested_model: true` (the default), the proxy uses whatever model string Claude Code sends, resolving it against your `models` config and the catalog. Set it to `false` to force scenario-based routing regardless of the requested model.
- **Pin a model alias** — use [`model_overrides`](#model-overrides-model_overrides) to map a client-visible model name to a fixed upstream model. For example, requesting `claude-sonnet-4.5` can be routed to any provider/model you choose.

### CC-Switch "Fetch Models" button

CC-Switch's custom-provider form has a **Fetch Models** button that calls the OpenAI-style `GET /v1/models` endpoint to populate a model dropdown. The proxy implements this endpoint: it returns every model identifier you can request — config `models` aliases, `model_overrides` keys, and catalog canonical names (`provider/model`). See [docs/reference-api.md](docs/reference-api.md#get-v1models).

If the dropdown looks short, it usually means the model catalog has not synced into local storage yet; the scenario aliases (`default`, `fast`, `complex`, …) and any `model_overrides` keys always appear.

### Troubleshooting

- **CC-Switch reports the provider is unreachable** — confirm the proxy is running (`routatic-proxy status`) and the endpoint URL/port match `host`/`port` in your proxy config.
- **401 / auth errors from the proxy** — the token CC-Switch sends must satisfy the proxy's `api_key` / `api_keys` (or those must be unset). This is proxy-side auth, unrelated to your upstream provider keys.
- **Wrong model runs** — check routing precedence: `model_overrides` wins, then `respect_requested_model`, then scenario routing. See [Routing precedence](#routing-precedence).
