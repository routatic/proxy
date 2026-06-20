# How to Add a New Model

Adding a new model requires zero code changes. Everything is config-driven.

## Step 1: Identify the Provider and Endpoint

Determine which upstream provider the model uses and which endpoint format it accepts:

| Provider | Endpoint | Format |
|----------|----------|--------|
| `opencode-go` | `/v1/chat/completions` | OpenAI Chat Completions (default) |
| `opencode-go` | `/v1/messages` | Anthropic Messages (MiniMax, Qwen) |
| `opencode-zen` | `/v1/chat/completions` | OpenAI Chat Completions |
| `opencode-zen` | `/v1/messages` | Anthropic Messages (Claude, Qwen) |
| `opencode-zen` | `/v1/responses` | OpenAI Responses (GPT models) |
| `opencode-zen` | `/v1/models/{id}` | Gemini |

## Step 2: Add Model Metadata

Edit `internal/config/model_registry.go` and add the model to `modelMetadata`:

```go
"my-new-model": {
    ContextWindow:   256000,
    MaxOutputTokens: 8192,
    Vision:          false,
    SupportsTools:   true,
},
```

This metadata is used by `ResolveModelConfig` to fill in defaults when the model is referenced in config.

## Step 3: Add Endpoint Classification (Zen only)

If the model uses Zen, add it to the appropriate classifier in `internal/client/opencode.go`:

```go
// For Anthropic endpoint:
func isZenAnthropicModel(modelID string) bool {
    // ...
    if modelID == "my-new-model" {
        return true
    }
    // ...
}

// For Responses endpoint:
func isResponsesModel(modelID string) bool {
    // ...
    if modelID == "my-new-model" {
        return true
    }
    // ...
}

// For Gemini endpoint:
func isGeminiModel(modelID string) bool {
    // ...
    if modelID == "my-new-model" {
        return true
    }
    // ...
}
```

If the model uses Go provider and requires the Anthropic endpoint (not Chat Completions), add it to `IsAnthropicModel`:

```go
func IsAnthropicModel(modelID string) bool {
    switch modelID {
    // ...
    case "my-new-model":
        return true
    // ...
    }
}
```

## Step 4: Add to Config

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

## Step 5: Test

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

Set `"vision": true` in the model metadata to enable image routing:

```go
"my-vision-model": {
    ContextWindow:   256000,
    MaxOutputTokens: 8192,
    Vision:          true,
    SupportsTools:   true,
},
```

### Temperature constraints

Some models have hard temperature requirements (e.g., kimi-k2.7-code requires temperature=1). Add constraints in `constrainTemperature` in `internal/transformer/request.go`.
