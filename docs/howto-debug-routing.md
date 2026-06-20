# How to Debug Routing Issues

When requests route to unexpected models or fail, here's how to diagnose.

## Enable Debug Logging

Set log level to debug in config:

```json
{
  "logging": {
    "level": "debug"
  }
}
```

Or temporarily via environment:

```bash
ROUTATIC_PROXY_LOG_LEVEL=debug routatic-proxy serve
```

Debug logs show:
- Request parsing and token counting
- Scenario detection with reasons
- Model selection and fallback attempts
- Upstream request/response details

## Check Scenario Detection

The log line `INFO routing request` shows the selected scenario and model:

```
INFO routing request scenario=complex model=glm-5.1 provider=opencode-go tokens=1500
```

If the scenario is wrong, check the keyword patterns in `internal/router/scenarios.go`.

## Check Circuit Breakers

If a model is being skipped, the circuit breaker may be open:

```
INFO circuit breaker open, skipping model model=kimi-k2.6 attempt=2 total=3
```

Circuit breakers open after 3 consecutive failures and recover after 30 seconds. Wait or restart the proxy to reset.

## Check Model Configuration

Validate your config:

```bash
routatic-proxy validate
```

Common issues:
- Model ID typo in `models` or `model_overrides`
- Missing provider field (defaults to `opencode-go`)
- Wrong endpoint format for the model

## Check Upstream Errors

5xx errors from upstream trigger fallback:

```
WARN model failed, trying fallback model=kimi-k2.6 error="API error 502: ..." remaining=2
```

4xx errors skip the circuit breaker (retrying won't help):

```
WARN non-retryable error (skipping circuit breaker), trying fallback model=kimi-k2.6 error="API error 400: ..."
```

## Check Token Counting

If requests route to `long_context` unexpectedly, check the token count:

```bash
curl -X POST http://localhost:3456/v1/messages/count_tokens \
  -H "Content-Type: application/json" \
  -d '{"system":"...","messages":[...]}'
```

## Check Streaming

Streaming issues show as idle timeouts or client disconnects:

```
WARN upstream openai stream idle, trying next model model=qwen3.6-plus idle_timeout=5m0s
```

```
DEBUG client disconnected during stream
```

The second is normal during Claude Code tool execution — the client pauses the stream while processing tool results.

## Common Routing Scenarios

**Request routes to default instead of complex:**
- Check if the keyword is in `hasComplexPattern()` — it only checks system and user messages
- Check if a tool keyword in `hasBackgroundPattern()` is blocking it

**Request routes to fast instead of complex (streaming):**
- Streaming routes to `fast` by default when `enable_streaming_scenario_routing` is false
- Enable scenario routing: `"enable_streaming_scenario_routing": true`

**Request routes to long_context unexpectedly:**
- Check token count — the default threshold is 100K tokens
- Image tokens add ~1500 per image — large images can push over the threshold
- Adjust threshold: `"context_threshold": 80000` in the long_context model config

**Vision request routes to non-vision model:**
- Check `"vision": true` in the model metadata (`internal/config/model_registry.go`)
- Check that the model is configured in the `vision` scenario in config
