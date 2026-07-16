# OpenRouter Provider

[OpenRouter](https://openrouter.ai) is a unified API for 200+ LLMs from OpenAI, Anthropic, Google, Meta, Mistral, and other leading AI providers. It provides a single endpoint for accessing models from multiple vendors without managing separate API keys and integrations for each provider.

## Overview

### What is OpenRouter?

OpenRouter acts as a universal gateway to the AI model ecosystem. Instead of maintaining separate accounts and API keys for OpenAI, Anthropic, Google, and dozens of other providers, you use a single OpenRouter API key to access them all. OpenRouter handles the routing, normalization, and billing.

### Benefits

- **Unified API**: One endpoint, one authentication method for 200+ models
- **Automatic failover**: If a provider is down, requests can route to alternatives
- **Standardized pricing**: Clear per-token costs across all providers
- **Model exploration**: Easily experiment with new models without new integrations
- **OpenAI-compatible format**: Works with existing OpenAI SDKs and tools
- **No code changes**: Add new models via configuration only

## Getting Started

### 1. Sign Up and Get API Key

1. Sign up at [openrouter.ai](https://openrouter.ai)
2. Generate an API key at [https://openrouter.ai/keys](https://openrouter.ai/keys)
3. Add funds to your account (pay-as-you-go pricing)

### 2. Configure Environment Variables

Set the environment variable:

```bash
export ROUTATIC_PROXY_OPENROUTER_API_KEY=sk-or-v1-your-key-here
```

For key rotation or load balancing across multiple keys, use a comma-separated list:

```bash
export ROUTATIC_PROXY_OPENROUTER_API_KEYS=key-1,key-2,key-3
```

### 3. Enable in Config

Add the `openrouter` provider to your `~/.config/routatic-proxy/config.json`:

```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "api_key": "${ROUTATIC_PROXY_OPENROUTER_API_KEY}",
      "base_url": "https://openrouter.ai/api/v1"
    }
  }
}
```

## Configuration

### Configuration Schema

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

### Environment Variables

| Variable | Description | Precedence |
|----------|-------------|------------|
| `ROUTATIC_PROXY_OPENROUTER_API_KEY` | Single API key override | Highest |
| `ROUTATIC_PROXY_OPENROUTER_API_KEYS` | Comma-separated keys for round-robin | Highest |
| `ROUTATIC_PROXY_OPENROUTER_BASE_URL` | Custom base URL override | Highest |

Environment variables take precedence over config file values. Config values support `${VAR}` interpolation.

Precedence order: `*_API_KEYS` → `*_API_KEY` → config file `api_keys` → config file `api_key`

## Configuration Examples

### Single-Key Setup

```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "api_key": "sk-or-v1-xxxxxxxxxxxxxxxxxxxxxxxx"
    }
  }
}
```

### Multi-Key Round-Robin

For load balancing across multiple API keys:

```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "api_keys": [
        "sk-or-v1-key-1",
        "sk-or-v1-key-2",
        "sk-or-v1-key-3"
      ]
    }
  }
}
```

### Custom Base URL

For enterprise/self-hosted OpenRouter deployments:

```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "base_url": "https://openrouter.mycompany.com/api/v1",
      "api_key": "${OPENROUTER_API_KEY}"
    }
  }
}
```

### Complete Configuration with Models

```json
{
  "providers": {
    "openrouter": {
      "enabled": true,
      "api_key": "${ROUTATIC_PROXY_OPENROUTER_API_KEY}",
      "base_url": "https://openrouter.ai/api/v1",
      "timeout_ms": 300000,
      "stream_timeout_ms": 60000
    }
  },
  "models": {
    "openrouter/openai/gpt-4o": {
      "enabled": true,
      "display_name": "GPT-4o (via OpenRouter)"
    },
    "openrouter/anthropic/claude-3.5-sonnet": {
      "enabled": true,
      "display_name": "Claude 3.5 Sonnet (via OpenRouter)"
    },
    "openrouter/anthropic/claude-3-opus": {
      "enabled": true,
      "display_name": "Claude 3 Opus (via OpenRouter)"
    },
    "openrouter/anthropic/claude-3.5-haiku": {
      "enabled": true,
      "display_name": "Claude 3.5 Haiku (via OpenRouter)"
    },
    "openrouter/google/gemini-2.0-flash-exp": {
      "enabled": true,
      "display_name": "Gemini 2.0 Flash (via OpenRouter)"
    },
    "openrouter/google/gemini-pro-1.5": {
      "enabled": true,
      "display_name": "Gemini 1.5 Pro (via OpenRouter)"
    },
    "openrouter/meta-llama/llama-3.3-70b-instruct": {
      "enabled": true,
      "display_name": "Llama 3.3 70B (via OpenRouter)"
    },
    "openrouter/meta-llama/llama-3.1-405b": {
      "enabled": true,
      "display_name": "Llama 3.1 405B (via OpenRouter)"
    },
    "openrouter/mistralai/mistral-large": {
      "enabled": true,
      "display_name": "Mistral Large (via OpenRouter)"
    },
    "openrouter/deepseek/deepseek-chat": {
      "enabled": true,
      "display_name": "DeepSeek V3 (via OpenRouter)"
    }
  }
}
```

## Cost-Based Routing Integration

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

### Applying Cost Penalty to OpenRouter

When using `cost_routing`, you can apply a penalty to OpenRouter requests to account for routing overhead or prefer direct providers when costs are similar:

```json
{
  "cost_routing": {
    "enabled": true,
    "prefer_providers": ["opencode-go", "openrouter"],
    "penalty_per_provider": {
      "openrouter": 0.05
    }
  }
}
```

This adds a small cost penalty (e.g., 5 cents per million tokens) when selecting OpenRouter models, helping the router prefer direct providers when cost is comparable.

## Model Selection and Naming Convention

### Provider/Model Format

OpenRouter uses the `provider/model-name` format. Models are referenced using the `openrouter/` prefix followed by the provider and model name:

```
openrouter/{provider}/{model-name}
```

Examples:
- `openrouter/openai/gpt-4o`
- `openrouter/anthropic/claude-3.5-sonnet`
- `openrouter/google/gemini-2.0-flash-exp`
- `openrouter/meta-llama/llama-3.3-70b-instruct`

### Model Resolution via Catalog

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

The `model_id` in your config must match OpenRouter's model identifier exactly.

### Discovering Models

1. Visit [openrouter.ai/models](https://openrouter.ai/models) for the complete model list
2. Use the `routatic-proxy models` command to see cached catalog entries
3. Check the [OpenRouter API docs](https://openrouter.ai/docs) for pricing and context limits

## Model Examples

| Model Key | Provider | Description | Best For |
|-----------|----------|-------------|----------|
| `openai/gpt-4o` | OpenAI | Latest GPT-4o multimodal model | General purpose, vision tasks |
| `openai/o1` | OpenAI | Reasoning model (o1) | Complex reasoning, math, coding |
| `openai/gpt-4.5-preview` | OpenAI | GPT-4.5 preview | Advanced reasoning, research |
| `anthropic/claude-3.5-sonnet` | Anthropic | Claude 3.5 Sonnet | Coding, analysis, writing |
| `anthropic/claude-3-opus` | Anthropic | Claude 3 Opus | Most capable Anthropic model |
| `anthropic/claude-3.5-haiku` | Anthropic | Claude 3.5 Haiku | Fast, cost-effective tasks |
| `anthropic/claude-opus-4` | Anthropic | Claude Opus 4 | Deep reasoning, coding |
| `google/gemini-2.0-flash-exp` | Google | Gemini 2.0 Flash (experimental) | Low latency, high throughput |
| `google/gemini-pro-1.5` | Google | Gemini 1.5 Pro | Long context (up to 2M tokens) |
| `google/gemini-2.5-pro-preview-07-11` | Google | Gemini 2.5 Pro | Advanced multimodal tasks |
| `meta-llama/llama-3.3-70b-instruct` | Meta | Llama 3.3 70B | Open source, self-hostable |
| `meta-llama/llama-3.1-405b` | Meta | Llama 3.1 405B | Largest open source model |
| `mistralai/mistral-large` | Mistral | Mistral Large | Strong multilingual performance |
| `mistralai/mistral-medium` | Mistral | Mistral Medium | Balanced performance/cost |
| `mistralai/mistral-small` | Mistral | Mistral Small | Fast, efficient tasks |
| `deepseek/deepseek-chat` | DeepSeek | DeepSeek V3 | Strong reasoning, coding |
| `deepseek/deepseek-r1` | DeepSeek | DeepSeek R1 | Reasoning, step-by-step |
| `perplexity/sonar-reasoning` | Perplexity | Sonar Reasoning | Research, citations |

See the full catalog at [https://openrouter.ai/models](https://openrouter.ai/models).

## Use Cases

### Accessing Specific Models

Use OpenRouter when you need models not available on other providers:

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

### Fallback Chains

Include OpenRouter as a fallback when primary providers fail:

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

### Cost Optimization

Use `cost_routing` with provider penalties to automatically select the cheapest available model:

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

## Official Documentation

- **API Reference**: [https://openrouter.ai/docs](https://openrouter.ai/docs)
- **OpenAI Compatibility**: [https://openrouter.ai/docs#openai-compatibility](https://openrouter.ai/docs#openai-compatibility)
- **Provider Routing**: [https://openrouter.ai/docs#provider-routing](https://openrouter.ai/docs#provider-routing)
- **Models Catalog**: [https://openrouter.ai/models](https://openrouter.ai/models)

## Catalog Resolution Details

Resolution functions in `internal/catalog/resolve.go` extract the provider from the key prefix. For OpenRouter models:

- `ResolvedModel.ModelID` is the model name only (without provider prefix)
- `ResolvedModel.CanonicalName` is the full key (e.g., `openrouter/anthropic/claude-opus-4`)

The catalog schema for models includes:

| Field | Description |
|-------|-------------|
| `id` | Full key (matches the map key) |
| `name` | Display name |
| `limit.context` | Context window size |
| `rates.input` | Cost per million input tokens |
| `rates.output` | Cost per million output tokens |
| `tool_call` | Whether tools are supported |
| `modalities.input` | Input types (`["text"]`, `["text", "image"]`) |
| `modalities.output` | Output types (`["text"]`, `["text", "image"]`) |
| `reasoning` | Whether reasoning mode is supported |

For streaming, the router may downgrade to faster models for better TTFT (time to first token).

---

**Note**: OpenRouter models use the OpenAI Chat Completions API format. The proxy automatically handles request/response transformation between Anthropic and OpenAI formats.
