# How to Customize Model Routing

routatic-proxy routes requests to different models based on request content. You can customize this behavior through configuration.

## Understanding Scenarios

Each request is classified into a scenario, which maps to a model:

| Scenario | Trigger | Default Model |
|----------|---------|---------------|
| `default` | No special patterns detected | Kimi K2.6 |
| `complex` | Architectural keywords, tool operations | GLM-5.1 |
| `think` | Reasoning keywords in system prompt | GLM-5 |
| `background` | Simple read-only ops (ls, cat, "what is") | Qwen3.5 Plus |
| `long_context` | Token count > threshold (default 100K) | MiniMax M2.5 |
| `vision` | Request contains images | (must configure) |
| `fast` | Streaming requests (when scenario routing disabled) | Qwen3.6 Plus |

## Override Scenario Models

Change which model handles each scenario:

```json
{
  "models": {
    "default": {
      "provider": "opencode-go",
      "model_id": "kimi-k2.6",
      "temperature": 0.7,
      "max_tokens": 4096
    },
    "complex": {
      "provider": "opencode-go",
      "model_id": "glm-5.1",
      "temperature": 0.7,
      "max_tokens": 4096
    }
  }
}
```

## Add Model Overrides

Model overrides let specific model names bypass scenario routing:

```json
{
  "model_overrides": {
    "deepseek-v4-pro": {
      "provider": "opencode-zen",
      "model_id": "deepseek-v4-pro",
      "temperature": 0.7,
      "max_tokens": 8192,
      "reasoning_effort": "max",
      "thinking": { "type": "enabled" }
    }
  }
}
```

When Claude Code requests `deepseek-v4-pro`, it goes directly to that model regardless of scenario.

## Customize Fallback Chains

Define per-scenario fallback chains:

```json
{
  "fallbacks": {
    "default": [
      { "provider": "opencode-go", "model_id": "mimo-v2-pro" },
      { "provider": "opencode-go", "model_id": "qwen3.6-plus" }
    ],
    "complex": [
      { "provider": "opencode-go", "model_id": "glm-5" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ],
    "long_context": [
      { "provider": "opencode-go", "model_id": "minimax-m2.7" },
      { "provider": "opencode-go", "model_id": "kimi-k2.6" }
    ]
  }
}
```

If a model in the chain fails (5xx error, timeout), the next model is tried automatically.

## Adjust Context Threshold

The long-context threshold determines when the proxy switches to a 1M-context model:

```json
{
  "models": {
    "long_context": {
      "provider": "opencode-go",
      "model_id": "minimax-m2.5",
      "context_threshold": 80000
    }
  }
}
```

## Enable Streaming Scenario Routing

By default, streaming requests bypass scenario routing and use the `fast` model. Enable scenario-based routing for streaming:

```json
{
  "enable_streaming_scenario_routing": true
}
```

This is useful for multi-agent and review workflows where streaming requests need capability, not just speed.

## Disable Requested Model Routing

By default, the proxy respects the `model` field from Claude Code. Disable this to force scenario routing:

```json
{
  "respect_requested_model": false}
```

## Custom Scenario Detection

Scenario detection is keyword-based. To add custom patterns, edit `internal/router/scenarios.go`:

- `hasComplexPattern()` — keywords that trigger the `complex` scenario
- `hasThinkingPattern()` — keywords that trigger the `think` scenario
- `hasBackgroundPattern()` — keywords that trigger the `background` scenario
- `hasVisualIntent()` — keywords that suggest image-related requests

## Verify Routing

Check which scenario was selected in the logs:

```
INFO routing request scenario=complex model=glm-5.1 provider=opencode-go tokens=1500
```

Or use the validate command to check config:

```bash
routatic-proxy validate
```
