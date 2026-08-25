# How to Add a New Model

Adding a model from an existing family requires only config changes. A new Zen
model family that uses a non-default endpoint (Responses, Gemini, or Messages)
may require updating `internal/models/classifier.go`.

## Step 1: Identify the Provider and Endpoint

Determine which upstream provider the model uses and which endpoint format it accepts:

| Provider | Endpoint | Format |
|----------|----------|--------|
| `opencode-go` | `/v1/chat/completions` | OpenAI Chat Completions (default) |
| `opencode-go` | `/v1/messages` | Anthropic Messages (MiniMax, Qwen) |
| `opencode-zen` | `/v1/chat/completions` | OpenAI Chat Completions |
| `opencode-zen` | `/v1/messages` | Anthropic Messages (Claude, Qwen) |
| `opencode-zen` | `/v1/responses` | OpenAI Responses (GPT, Grok, Muse Spark) |
| `opencode-zen` | `/v1/models/{id}` | Gemini |
| `aws-bedrock` | `/v1/chat/completions` | OpenAI Chat Completions (Bedrock Mantle) |
| `aws-bedrock` | `/v1/messages` | Anthropic Messages (Bedrock Mantle, requires `wire_format: "anthropic"`) |

## Step 2: Check Endpoint Classification

Zen endpoint classification is prefix-based. Models in these existing families
need no classifier change:

| Endpoint | Recognized model prefixes | Classifier |
|----------|---------------------------|------------|
| Anthropic Messages | `claude-*`, `qwen*` | `IsZenAnthropicModel` |
| OpenAI Responses | `gpt-*`, `grok-*`, `muse-spark-*` | `IsResponsesModel` |
| Gemini | `gemini-*` | `IsGeminiModel` |

Update `internal/models/classifier.go` only when Zen introduces a new model
family that uses a non-default endpoint. Add the family prefix to the
appropriate classifier and add unit-test coverage. For example, a new
Responses family would be added alongside the existing prefixes:

```go
func IsResponsesModel(modelID string) bool {
    return strings.HasPrefix(modelID, "gpt-") ||
        strings.HasPrefix(modelID, "grok-") ||
        strings.HasPrefix(modelID, "muse-spark-") ||
        strings.HasPrefix(modelID, "my-responses-family-")
}
```

The Go provider is config-driven. If a Go model requires a non-default wire
format, set `wire_format` on its model configuration instead of changing the
Zen classifier:

```json
{
  "provider": "opencode-go",
  "model_id": "my-new-model",
  "wire_format": "anthropic"
}
```

Supported Go-provider overrides are `openai`, `anthropic`, and `responses`.
Zen classification functions are shared between `internal/client` and
`internal/provider` so both paths route models consistently.

## Step 3: Add to Config

Add the model to your `config.json`:

**As a scenario model:**

```json
{
  "models": {
    "default": {
      "provider": "opencode-go",
      "model_id": "my-new-model",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  }
}
```

**As a model override (for direct requests):**

```json
{
  "model_overrides": {
    "my-new-model": {
      "provider": "opencode-go",
      "model_id": "my-new-model",
      "temperature": 0.7,
      "max_tokens": 8192
    }
  }
}
```

**As a fallback:**

```json
{
  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "my-new-model" }
    ]
  }
}
```

## Step 4: Test

```bash
# Validate config
routatic-proxy validate

# Test with a request
curl -X POST http://localhost:3456/v1/messages \
  -H "Content-Type: application/json" \
  -d '{
    "model": "my-new-model",
    "max_tokens": 100,
    "messages": [{"role": "user", "content": "Hello"}]
  }'
```

## Model-Specific Considerations

### Models requiring Anthropic endpoint

Some models (MiniMax, Qwen on Go provider) only accept Anthropic Messages format, not OpenAI Chat Completions. These need `IsAnthropicModel` to return true.

### Models with thinking/reasoning

If the model supports thinking mode (DeepSeek, OpenAI o-series), configure:

```json
{
  "thinking": { "type": "enabled" },
  "reasoning_effort": "high"
}
```

The proxy handles the Anthropic `thinking` ↔ OpenAI `reasoning_content` translation automatically.

### Models with tool format issues

If the model doesn't support Anthropic's `type: "custom"` tool shorthands, set:

```json
{
  "anthropic_tools_disabled": true
}
```

This forces the request through the Chat Completions transform path.

### Models with vision support

Set `"vision": true` in the model config to enable image routing:

```json
{
  "my-vision-model": {
    "provider": "opencode-go",
    "model_id": "my-vision-model",
    "vision": true
  }
}
```

### Temperature constraints

Some models have hard temperature requirements (e.g., kimi-k2.7-code requires temperature=1). Add constraints in `constrainTemperature` in `internal/transformer/request.go`.

## Cost-Based Routing

When `cost_routing.enabled` is true, the proxy uses a catalog of model pricing data to automatically select the cheapest eligible model for each scenario.

The catalog is downloaded from `models.dev` and cached locally in `~/.config/routatic-proxy/catalog/`. The catalog schema uses provider-prefixed model keys:

```json
{
  "providers": {
    "opencode-go": {
      "name": "opencode-go",
      "base_url": "https://opencode.ai/zen/go/v1/chat/completions",
      "enabled": true
    }
  },
  "models": {
    "opencode-go/my-new-model": {
      "id": "opencode-go/my-new-model",
      "name": "My New Model",
      "limit": { "context": 128000 },
      "rates": { "input": 1.0, "output": 2.0 },
      "tool_call": true,
      "modalities": { "input": ["text"], "output": ["text"] }
    }
  }
}
```

### Key catalog fields:

| Field | Description |
|-------|-------------|
| `id` | Full model key (`provider/model-name`) |
| `name` | Display name |
| `limit.context` | Context window size (tokens) |
| `rates.input` | Cost per million input tokens |
| `rates.output` | Cost per million output tokens |
| `tool_call` | Whether the model supports tools |
| `modalities.input` | Input types: `["text"]` or `["text", "image"]` for vision |
| `modalities.output` | Output types: usually `["text"]` |
| `reasoning` | Whether the model supports reasoning mode |

To add a model to the cost-based routing catalog, submit a PR to the models.dev repository or run:

```bash
routatic-proxy catalog sync --force
```
