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

## Invalid API Key

If requests are rejected before any model is tried:

1. Verify the key in the [OpenCode console](https://opencode.ai/auth)
2. Check the key is actually set — either `api_key` in the config file or the `ROUTATIC_PROXY_API_KEY` environment variable
3. Run `routatic-proxy validate` to confirm the config loads and the key is picked up

Provider-specific keys override the global one, so also check `ROUTATIC_PROXY_OPENCODE_GO_API_KEY` and `ROUTATIC_PROXY_OPENCODE_ZEN_API_KEY` if you've set them.

## Connection Refused

Make sure the proxy is running:

```bash
routatic-proxy status
```

And Claude Code is pointing to the right address:

```bash
echo $ANTHROPIC_BASE_URL  # Should be http://127.0.0.1:3456
```

If the proxy starts but Claude Code still can't reach it:

1. Confirm the port matches your config — the default is 3456
2. Check your firewall isn't blocking loopback connections
3. Run `routatic-proxy check`, which compares `ANTHROPIC_BASE_URL` against the configured host and port and flags conflicting Claude Code settings in `~/.claude/settings.json` and `~/.claude.json`

## Streaming Not Working

The proxy transforms OpenAI SSE to Anthropic SSE in real-time. If streaming appears broken:

1. Set log level to `debug` to see the raw SSE chunks
2. Check that no proxy or firewall is buffering the connection
3. Try a non-streaming request first to verify the model works

## Slow Model Responses

1. Check which model actually handled the request — some are far slower than others. Set the log level to `debug` to see the selected model
2. Make sure the `fast` scenario is configured with low-latency models. Streaming requests fall back to `fast` when the detected scenario has no model configured
3. Rule out plain network latency to the upstream

## Inaccurate Token Counts

The proxy counts tokens with tiktoken's `cl100k_base` encoding. If the numbers look off:

1. It's an estimate, not an exact count — the proxy adds a fixed per-message overhead on top of the encoded text
2. The upstream models don't all use `cl100k_base`, so their own accounting will differ
3. Long-context detection is driven by this estimate, so a request near the threshold (100K tokens by default) may route differently than you'd expect

## `routatic-proxy update` Fails

**"install directory ... is not writable by the current user"**

The updater replaces the binary where it lives, and that directory is owned by another user — typically root, for `/usr/local/bin`. Nothing was downloaded and the current binary is untouched. Re-run with `sudo` (Unix) or from an Administrator terminal (Windows), or update through the package manager that installed it (`brew upgrade`, `scoop update`, `sudo dnf upgrade routatic-proxy`).

**"no beta releases found"**

The beta channel is selected but no prerelease could be resolved. Check the channel with `routatic-proxy update-channel`, and confirm prereleases exist on the [Releases page](https://github.com/routatic/proxy/releases). Versions before v0.6.4 could not match the `v{VERSION}-beta.{N}` tag format at all and always failed this way — install a current build manually to get past it, see [Install a specific beta manually](INSTALLATION.md#install-a-specific-beta-manually).

**"You are already on the latest version" after switching to stable**

Expected: a beta is newer than the current stable release, so there is nothing to update *to*. Switching channels never downgrades. Reinstall a stable build explicitly — see [Going back to stable](INSTALLATION.md#going-back-to-stable).

**"GitHub API returned status 403"**

Unauthenticated GitHub API requests are rate limited per IP. Wait for the window to reset, or download the release asset directly from the [Releases page](https://github.com/routatic/proxy/releases).

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

## Getting Help

If none of the above resolves it:

1. Search the [GitHub issues](https://github.com/routatic/proxy/issues)
2. Ask on [Discord](https://discord.gg/pUrfwfTFxM)
3. Attach debug logs when you open a new issue
