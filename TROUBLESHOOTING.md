# Troubleshooting

## Windows Scoop Background Mode

On Windows, `routatic-proxy serve -b` uses the native Windows process APIs and keeps
the Scoop shim path intact. This means background mode does not require `nohup`
or a Unix-like shell, and Scoop-provided environment variables continue to work.

## "invalid request body" Error

This means the proxy couldn't parse the request from Claude Code. Enable debug logging to see the raw request:

```json
{ "logging": { "level": "debug" } }
```

Or set the environment variable:

```bash
export ROUTATIC_PROXY_LOG_LEVEL=debug
```

## "all models failed" Error

All models in the fallback chain returned errors. Check:

1. Your API key is valid: `routatic-proxy validate`
2. You haven't exceeded your [usage limits](https://opencode.ai/auth)
3. The OpenCode Go service is reachable: `curl -H "Authorization: Bearer $ROUTATIC_PROXY_API_KEY" https://opencode.ai/zen/go/v1/models`

## Connection Refused

Make sure the proxy is running:

```bash
routatic-proxy status
```

And Claude Code is pointing to the right address:

```bash
echo $ANTHROPIC_BASE_URL  # Should be http://127.0.0.1:3456
```

## Streaming Not Working

The proxy transforms OpenAI SSE to Anthropic SSE in real-time. If streaming appears broken:

1. Set log level to `debug` to see the raw SSE chunks
2. Check that no proxy or firewall is buffering the connection
3. Try a non-streaming request first to verify the model works

## Debug Mode

For maximum logging, run with debug level:

```bash
ROUTATIC_PROXY_LOG_LEVEL=debug routatic-proxy serve
```

This logs:

- Raw Anthropic request body from Claude Code
- Transformed request sent to upstream (OpenCode Go/Zen)
- Upstream response received
- SSE stream events during streaming
