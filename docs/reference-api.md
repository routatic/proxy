# HTTP API Reference

routatic-proxy exposes an Anthropic-compatible API. Claude Code connects to it as if it were the Anthropic API.

## Endpoints

### `POST /v1/messages`

The primary endpoint. Accepts Anthropic Messages API requests and returns responses in the same format.

**Request body** — standard Anthropic `MessageRequest`:

```json
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 4096,
  "system": "You are a helpful assistant.",
  "messages": [
    {
      "role": "user",
      "content": "Hello, world!"
    }
  ],
  "stream": true,
  "tools": []
}
```

**Response** — Anthropic `MessageResponse` (non-streaming) or SSE stream (streaming).

**Routing behavior:**

- If `model` matches an entry in `model_overrides`, that model is used as primary with a scenario-derived safety net
- Otherwise, scenario-based routing selects the model based on request content and token count
- Set `respect_requested_model: false` in config to force scenario routing regardless of the `model` field

**Headers:**

| Header | Value |
|--------|-------|
| `X-Request-ID` | Unique request identifier (generated or forwarded from client) |
| `Content-Type` | `application/json` (non-streaming) or `text/event-stream` (streaming) |

### `POST /v1/messages/count_tokens`

Counts tokens for a message array without generating a response.

**Request body:**

```json
{
  "system": "System prompt text",
  "messages": [
    { "role": "user", "content": "Hello" }
  ]
}
```

**Response:**

```json
{
  "input_tokens": 42
}
```

### `GET /health`

Returns server health status.

**Response:**

```json
{
  "status": "ok",
  "version": "1.2.3",
  "models_configured": 6,
  "uptime": "2h30m"
}
```

### `GET /statusline`

Returns compact status for TUI integration (statusline, tmux bar).

**Response:**

```json
{
  "status": "running",
  "version": "1.2.3",
  "uptime": "2h30m"
}
```

## Error Responses

Errors follow Anthropic's error format:

```json
{
  "type": "error",
  "error": {
    "type": "api_error",
    "message": "description of what went wrong"
  }
}
```

**HTTP status codes:**

| Code | Meaning |
|------|---------|
| 400 | Invalid request body |
| 405 | Method not allowed (non-POST on /v1/messages) |
| 413 | Request body too large (>100MB) |
| 429 | Rate limited |
| 500 | Internal error (routing failed, transform error) |
| 502 | All upstream models failed |

## Streaming

Streaming responses use Server-Sent Events (SSE) with Anthropic's event format:

```
event: message_start
data: {"type":"message_start","message":{"id":"msg_...","type":"message","role":"assistant","content":[],"model":"...","stop_reason":null,"stop_sequence":null,"usage":{"input_tokens":42,"output_tokens":0}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"Hello"}}

event: content_block_stop
data: {"type":"content_block_stop","index":0}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"end_turn","stop_sequence":null},"usage":{"output_tokens":15}}

event: message_stop
data: {"type":"message_stop"}
```

**Heartbeat**: keepalive comments (`:keepalive\n\n`) are sent every 3 seconds during streaming.

## Rate Limiting

The proxy applies per-IP rate limiting (default: 100 requests/minute). Rate-limited requests receive HTTP 429.

## Request Deduplication

Optional request deduplication (`request_dedup` in config) prevents processing identical concurrent requests. Deduplicated requests receive HTTP 200 with no body.
